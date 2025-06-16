package handlers

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/measure"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	MeasurementReadyHandlerName                         = "MEASUREMENT_READY_HANDLER"
	arm                         instrument.PropertyName = "ARM"
)

// SubscriptionConfig defines a subscription configuration
type SubscriptionConfig struct {
	Subject     string
	HandlerFunc func(*nats.Msg)
	SubField    **nats.Subscription // pointer to the subscription field in the handler
	Name        string              // for logging
}

var (
	ArmedMessage            = api.GetCommandName(api.Armed{})
	ExecutingMessage        = api.GetCommandName(api.Executing{})
	MeasurementReadyMessage = api.GetCommandName(api.MeasurementReady{})
	ProcessDataMessage      = api.GetCommandName(api.ProcessData{})
	TriggerMessage          = api.GetCommandName(api.Trigger{})
	ReturnDataMessage       = api.GetCommandName(api.ReturnData{})
	UploadDataMessage       = api.GetCommandName(api.UploadData{})
	GetMessage              = api.GetCommandName(api.Get{})
)

// MeasurementScheduler tracks measurements waiting for RETURN_DATA
type MeasurementScheduler struct {
	ID                       instrument.MeasurementID    // Combined ProcessId and ChunkId
	GetterPorts              []instrument.JsonPort       // Original getter ports
	SetterPorts              []instrument.JsonPort       // Original setter ports
	GetterInstruments        []instrument.Name           // Instruments that need to be armed
	SetterInstruments        []instrument.Name           // Instruments that need to be armed
	MasterTriggerInstruments []instrument.Name           // Master instruments for hardware trigger
	ReceivedReturns          int                         // Number of RETURN_DATA messages received
	ExpectedReturns          int                         // Expected number of RETURN_DATA messages
	Results                  map[instrument.JsonPort]any // Port -> Data mapping
	ReadyChecklist           map[instrument.Name]bool    // Setter instrument -> ready status
	TriggeredGetterChecklist map[instrument.Name]bool    // Getter instrument -> triggered status
}

func (ms *MeasurementScheduler) registerReadySetter(
	name instrument.Name,
) error {
	if !slices.Contains(ms.SetterInstruments, name) {
		return fmt.Errorf(
			"instrument %s not found in setter instruments. Available setter instruments: %v",
			name,
			ms.SetterInstruments,
		)
	}
	ms.ReadyChecklist[name] = true
	return nil
}

func (ms *MeasurementScheduler) settersAreReady() bool {
	for _, name := range ms.SetterInstruments {
		if !ms.ReadyChecklist[name] {
			return false
		}
	}
	return true
}

func (ms *MeasurementScheduler) resetSettersReadiness() {
	for name := range ms.ReadyChecklist {
		ms.ReadyChecklist[name] = false
	}
}

func (ms *MeasurementScheduler) gettersAreTriggered() bool {
	for _, name := range ms.GetterInstruments {
		if !ms.TriggeredGetterChecklist[name] {
			return false
		}
	}
	return true
}

func (ms *MeasurementScheduler) resetGettersTriggered() {
	for name := range ms.TriggeredGetterChecklist {
		ms.TriggeredGetterChecklist[name] = false
	}
}

func (ms *MeasurementScheduler) containsGetter(port instrument.JsonPort) bool {
	return slices.Contains(ms.GetterPorts, port)
}

func (ms *MeasurementScheduler) storeData(port instrument.JsonPort, data any) {
	ms.Results[port] = data
	ms.ReceivedReturns++
}

func (ms *MeasurementScheduler) allDataHere() bool {
	return ms.ReceivedReturns == ms.ExpectedReturns
}

type Instructions struct {
	Setter   instrument.JsonPort       `json:"setter"`
	Property []instrument.PropertyName `json:"property"`
	Values   []any                     `json:"values"`
}

// separate converts the Instructions into a slice of SetInstruction
func (in *Instructions) separate() []instrument.SetInstruction {
	var instructions []instrument.SetInstruction
	for i, property := range in.Property {
		instructions = append(instructions, instrument.SetInstruction{
			Name:     in.Setter,
			Property: property,
			Value:    in.Values[i],
		})
	}
	return instructions
}

// fromJson loads instructions from a JSON string
func (in *Instructions) fromJson(jsonStr string) error {
	err1 := json.Unmarshal([]byte(jsonStr), &in)
	// marshal cycling the Setter to ensure it is a valid JsonPort
	fixed_bytes, err2 := json.Marshal(in.Setter)
	err3 := json.Unmarshal(fixed_bytes, &in.Setter)
	if err1 == nil && err2 == nil && err3 == nil {
		return nil
	}
	var errorMsgs []string
	if err1 != nil {
		errorMsgs = append(
			errorMsgs,
			fmt.Sprintf("unmarshal error: %v", err1),
		)
	}
	if err2 != nil {
		errorMsgs = append(
			errorMsgs,
			fmt.Sprintf("marshal error: %v", err2),
		)
	}
	if err3 != nil {
		errorMsgs = append(
			errorMsgs,
			fmt.Sprintf("remarshal error: %v", err3),
		)
	}
	return fmt.Errorf(
		"failed to process instruction: %s",
		strings.Join(errorMsgs, "; "),
	)
}

type InstrumentInstructions struct {
	Name            instrument.Name
	SetInstructions []instrument.SetInstruction
}

// append adds a new instruction to the list
func (ii *InstrumentInstructions) append(in Instructions) {
	ii.SetInstructions = append(ii.SetInstructions, in.separate()...)
}

// peek returns the first instruction without removing it
func (ii *InstrumentInstructions) peek() *instrument.SetInstruction {
	return &ii.SetInstructions[0]
}

// arm will add an arm instruction to the end of the lists
func (ii *InstrumentInstructions) arm() {
	// any Instructions for the instrument will work as a surrogate
	newii := Instructions{
		Setter:   ii.peek().Name,
		Property: []instrument.PropertyName{arm},
		Values:   []any{true},
	}
	// TODO: seperate the arm instruction from the get go

	ii.append(newii)
}

// MeasurementReadyHandler handles MEASUREMENT_READY requests
type MeasurementReadyHandler struct {
	logger              *logging.Logger
	log                 *instrument.LogWrapper
	nc                  *nats.Conn
	subscription        *nats.Subscription
	armedSub            *nats.Subscription
	executingSub        *nats.Subscription
	returnDataSub       *nats.Subscription
	instrumentHandler   *instrument.Handler
	config              *config.Config
	measurementStack    *measure.MeasurementStack
	currentMeasurement  *measure.MeasurementStackItem
	isProcessing        bool
	getResults          map[instrument.ID]map[instrument.JsonPort]any
	schedulers          map[instrument.ID]map[instrument.ID]*MeasurementScheduler // ProcessId -> ChunkId -> Scheduler
	pendingMeasurements map[instrument.ID]*MeasurementScheduler
	pendingGets         map[instrument.ID]any
	nextChunkId         int64 // Unique identifier for the next chunk
	mutex               sync.RWMutex
}

// NewMeasurementReadyHandler creates a new handler
func NewMeasurementReadyHandler(
	logger *logging.Logger,
	instrumentHandler *instrument.Handler,
	cfg *config.Config,
) *MeasurementReadyHandler {
	return &MeasurementReadyHandler{
		logger: logger,
		log: instrument.NewLogWrapper(
			logger,
			MeasurementReadyHandlerName,
		),
		instrumentHandler: instrumentHandler,
		config:            cfg,
		measurementStack:  &measure.MeasurementStack{},
		isProcessing:      false,
		getResults:        make(map[instrument.ID]map[instrument.JsonPort]any),
		schedulers: make(
			map[instrument.ID]map[instrument.ID]*MeasurementScheduler,
		),
		pendingMeasurements: make(
			map[instrument.ID]*MeasurementScheduler,
		),
		pendingGets: make(map[instrument.ID]any),
		nextChunkId: 1,
	}
}

// Subscribe starts listening for MEASUREMENT_READY requests
func (h *MeasurementReadyHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc

	// Define all subscriptions
	subscriptions := []SubscriptionConfig{
		{
			Subject:     MeasurementReadyMessage,
			HandlerFunc: h.handleMeasurementReady,
			SubField:    &h.subscription,
			Name:        MeasurementReadyMessage,
		},
		{
			Subject:     ArmedMessage + ".>",
			HandlerFunc: h.handleArmed,
			SubField:    &h.armedSub,
			Name:        ArmedMessage,
		},
		{
			Subject:     ExecutingMessage + ".>",
			HandlerFunc: h.handleExecuting,
			SubField:    &h.executingSub,
			Name:        ExecutingMessage,
		},
		{
			Subject:     ReturnDataMessage + ".>",
			HandlerFunc: h.handleReturnData,
			SubField:    &h.returnDataSub,
			Name:        ReturnDataMessage,
		},
	}

	// Subscribe to all subjects
	var subjects []string
	for _, config := range subscriptions {
		sub, err := nc.Subscribe(config.Subject, config.HandlerFunc)
		if err != nil {
			return fmt.Errorf(
				"failed to subscribe to %s: %w",
				config.Name,
				err,
			)
		}
		*config.SubField = sub
		subjects = append(subjects, config.Name)
	}

	h.log.Info("Subscribed to %s", strings.Join(subjects, ", "))
	return nil
}

// Unsubscribe stops listening for commands
func (h *MeasurementReadyHandler) Unsubscribe() error {
	// Define all subscriptions to unsubscribe from
	subscriptions := []*nats.Subscription{
		h.subscription,
		h.returnDataSub,
		h.executingSub,
		h.armedSub,
	}

	var errs []error
	for _, sub := range subscriptions {
		if sub != nil {
			if err := sub.Unsubscribe(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	// Clear all subscription references
	h.subscription = nil
	h.returnDataSub = nil
	h.executingSub = nil
	h.armedSub = nil

	if len(errs) > 0 {
		return fmt.Errorf("failed to unsubscribe: %v", errs)
	}

	h.log.Info(
		"Unsubscribed from %s, %s, %s, and %s",
		MeasurementReadyMessage,
		ArmedMessage,
		ExecutingMessage,
		ReturnDataMessage,
	)
	return nil
}

// handleMeasurementReady processes incoming MEASUREMENT_READY requests
func (h *MeasurementReadyHandler) handleMeasurementReady(msg *nats.Msg) {
	var measurementReady api.MeasurementReady
	h.log.Debug("Received  %s", MeasurementReadyMessage)
	if err := json.Unmarshal(msg.Data, &measurementReady); err != nil {
		h.log.Error("Failed to unmarshal %s: %v", MeasurementReadyMessage, err)
		return
	}
	// Create stack item and add to queue with assigned ChunkId
	h.mutex.Lock()
	chunkId := h.nextChunkId
	h.nextChunkId++
	h.mutex.Unlock()

	stackItem := measure.MeasurementStackItem{
		MeasurementReady: measurementReady,
		Timestamp:        time.Now(),
		ChunkId:          chunkId,
	}

	h.measurementStack.Push(stackItem)
	h.log.Info(
		"Queued measurement for ProcessId %d with ChunkId %d. Queue size: %d",
		measurementReady.ProcessId,
		chunkId,
		h.measurementStack.Size(),
	)

	h.processMeasurementSets(stackItem)
	h.tryProcessNextMeasurement()
}

// tryProcessNextMeasurement attempts to start processing the next queued
// measurement
func (h *MeasurementReadyHandler) tryProcessNextMeasurement() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.isProcessing {
		h.log.Debug("Already processing a measurement, skipping")
		return
	}
	stackItem, hasNext := h.measurementStack.Pop()
	if !hasNext {
		h.log.Debug("No measurements in queue")
		return
	}
	h.isProcessing = true
	h.currentMeasurement = &stackItem

	h.log.Info(
		"Starting processing of measurement ProcessId %d, ChunkId %d. Remaining in queue: %d",
		stackItem.MeasurementReady.ProcessId,
		stackItem.ChunkId,
		h.measurementStack.Size(),
	)
}

// markMeasurementComplete marks the current measurement as complete and tries
// to process the next one
func (h *MeasurementReadyHandler) markMeasurementComplete() {
	h.mutex.Lock()
	h.isProcessing = false
	h.currentMeasurement = nil
	h.mutex.Unlock()

	h.log.Debug(
		"Measurement processing complete, checking for next measurement",
	)
	h.tryProcessNextMeasurement()
}

// processMeasurementSets sends SET commands immediately for pipelining
func (h *MeasurementReadyHandler) processMeasurementSets(
	stackItem measure.MeasurementStackItem,
) {
	msg := stackItem.MeasurementReady
	chunkId := stackItem.ChunkId

	h.log.Info(
		"Processing SET commands for ProcessId %d, ChunkId %d (Setters: %d)",
		msg.ProcessId,
		chunkId,
		len(msg.Setters),
	)

	totalInstructions, err := collectAllSetInstructions(msg.Setters)
	if err != nil {
		h.log.Error(
			"Failed to collect all set instructions: %s",
			err,
		)
	}

	// Begin sorting the instructions by instrument
	sortedInstructions := make(
		[]*InstrumentInstructions,
		0,
		len(totalInstructions),
	)

	for _, instruction := range totalInstructions {
		options, err := h.instrumentHandler.GetPortOptions(instruction.Setter)
		if err != nil {
			h.log.Error(
				"Failed to get port configuration for setter %s: %v",
				instruction.Setter,
				err,
			)
			continue
		}

		// Find existing InstrumentInstructions or create new one
		var targetInstructions *InstrumentInstructions
		for _, existing := range sortedInstructions {
			if existing.Name == options.Instrument {
				targetInstructions = existing
				break
			}
		}

		if targetInstructions == nil {
			targetInstructions = &InstrumentInstructions{
				Name: options.Instrument,
			}
			sortedInstructions = append(sortedInstructions, targetInstructions)
		}
		targetInstructions.append(instruction)
	}

	measurementID := instrument.MeasurementID{
		ProcessId: instrument.ID(msg.ProcessId),
		ChunkId:   instrument.ID(chunkId),
	}

	// Create scheduler BEFORE sending SET commands to avoid race condition
	h.createSchedulerForMeasurement(msg, chunkId)

	for _, instructions := range sortedInstructions {
		instructions.arm()
		h.instrumentHandler.SetProperties(
			instructions.SetInstructions,
			measurementID,
		)
	}
}

// createSchedulerForMeasurement creates the scheduler before sending SET
// commands
func (h *MeasurementReadyHandler) createSchedulerForMeasurement(
	msg api.MeasurementReady,
	chunkId int64,
) {
	if len(msg.Getters) == 0 {
		h.log.Error("No getters specified for measurement")
		return
	}

	totalInstructions, err := collectAllSetInstructions(msg.Setters)
	if err != nil {
		h.log.Error("Failed to collect all set instructions: %s", err)
		return
	}

	setterPorts := make([]instrument.JsonPort, 0, len(totalInstructions))
	for _, instruction := range totalInstructions {
		setterPorts = append(setterPorts, instruction.Setter)
	}

	getterPorts, err := convertToJsonPorts(msg.Getters)
	if err != nil {
		h.log.Error("Failed to convert getters to JsonPorts: %s", err)
		return
	}

	// Get unique instruments from getters and setters
	getterInstruments := h.getUniqueInstruments(getterPorts)
	setterInstruments := h.getUniqueInstruments(setterPorts)
	masterInstruments := setterInstruments // default for unbuffered

	if msg.Buffered && len(setterInstruments) > 1 {
		masterInstrument, err := h.instrumentHandler.FindMasterInstrument(
			setterInstruments,
		)
		if err != nil {
			h.log.Error("Failed to find master setter instruments: %v", err)
			return
		}
		masterInstruments = []instrument.Name{masterInstrument}

		if len(masterInstruments) == 0 {
			h.log.Error(
				"No master setter instruments found for buffered measurement",
			)
			return
		}

		if len(masterInstruments) > 1 {
			h.log.Error(
				"Multiple master setter instruments found for buffered measurement: %v, expected 1",
				masterInstruments,
			)
			return
		}

		h.log.Info(
			"Using master trigger instrument %s for buffered measurement",
			masterInstrument,
		)
	}

	readyChecklist := make(map[instrument.Name]bool)
	for _, instrumentName := range setterInstruments {
		readyChecklist[instrumentName] = false
	}

	triggerGetterChecklist := make(map[instrument.Name]bool)
	for _, instrumentName := range getterInstruments {
		triggerGetterChecklist[instrumentName] = false
	}

	// Initialize scheduler for this specific chunk
	h.mutex.Lock()
	if h.schedulers[instrument.ID(msg.ProcessId)] == nil {
		h.schedulers[instrument.ID(msg.ProcessId)] = make(
			map[instrument.ID]*MeasurementScheduler,
		)
	}

	scheduler := &MeasurementScheduler{
		ID: instrument.MeasurementID{
			ProcessId: instrument.ID(msg.ProcessId),
			ChunkId:   instrument.ID(chunkId),
		},
		GetterPorts:              getterPorts,
		GetterInstruments:        getterInstruments,
		SetterInstruments:        setterInstruments,
		MasterTriggerInstruments: masterInstruments,
		SetterPorts:              setterPorts,
		ReceivedReturns:          0,
		ExpectedReturns:          len(getterPorts),
		ReadyChecklist:           readyChecklist,
		TriggeredGetterChecklist: triggerGetterChecklist,
		Results:                  make(map[instrument.JsonPort]any),
	}

	h.log.Debug(
		"Created scheduler for %+v with setter instruments: %v, getter instruments: %v",
		scheduler.ID,
		setterInstruments,
		getterInstruments,
	)
	h.schedulers[instrument.ID(msg.ProcessId)][scheduler.ID.ChunkId] = scheduler
	h.mutex.Unlock()
}

func collectAllSetInstructions(
	setters []string,
) ([]Instructions, error) {
	var allInstructions []Instructions
	var errorMsgs []string

	for _, setter := range setters {
		var instructions Instructions
		err := instructions.fromJson(setter)
		if err == nil {
			allInstructions = append(allInstructions, instructions)
			continue
		}
		errorMsgs = append(
			errorMsgs,
			fmt.Sprintf("setter %q: %v", setter, err),
		)
	}

	if len(errorMsgs) > 0 {
		return allInstructions, fmt.Errorf(
			"failed to process some setters: %s",
			strings.Join(errorMsgs, "; "),
		)
	}

	return allInstructions, nil
}

// handleArmed processes ARMED messages from instruments
func (h *MeasurementReadyHandler) handleArmed(msg *nats.Msg) {
	var armed api.Armed
	h.log.Debug("Received %s", ArmedMessage)
	if err := json.Unmarshal(msg.Data, &armed); err != nil {
		h.log.Error("Failed to unmarshal %s: %v", ArmedMessage, err)
		return
	}

	measurementID := instrument.MeasurementID{
		ProcessId: instrument.ID(armed.ProcessId),
		ChunkId:   instrument.ID(armed.ChunkId),
	}
	if measurementID.ProcessId == 0 {
		h.log.Error("ProcessId not found in %s message", ArmedMessage)
		return
	}

	if measurementID.ChunkId == 0 {
		h.log.Error("ChunkId not found in %s message", ArmedMessage)
		return
	}

	// Subject format: ARMED.<instrument_name>
	subjectParts := strings.Split(msg.Subject, ".")
	if len(subjectParts) < 2 {
		h.log.Error("Invalid %s subject format: %s", ArmedMessage, msg.Subject)
		return
	}
	instrumentName := instrument.Name(subjectParts[1])
	h.log.Debug(
		"Processing %s from instrument: %s for %+v",
		ArmedMessage,
		instrumentName,
		measurementID,
	)

	// Update ready checklist for the specific scheduler
	h.mutex.Lock()
	defer h.mutex.Unlock()

	schedulerMap, exists := h.schedulers[measurementID.ProcessId]
	if !exists {
		h.log.Error(
			"No scheduler map found for ProcessId %d",
			measurementID.ProcessId,
		)
		return
	}

	scheduler, exists := schedulerMap[measurementID.ChunkId]
	if !exists {
		h.log.Error(
			"No scheduler found for %+v", measurementID,
		)
		return
	}
	err := scheduler.registerReadySetter(instrumentName)
	if err != nil {
		h.log.Error("error registering ready setter: %v",
			err.Error(),
		)
	}
	h.log.Debug(
		"Marked instrument %s as ready for %+v",
		instrumentName,
		measurementID,
	)
	if !scheduler.settersAreReady() {
		return
	}
	h.log.Info("All setter instruments ready for %+v", measurementID)
	scheduler.resetSettersReadiness()
	h.log.Debug("Reset ready checklist for %+v", measurementID)
	go h.triggerGetterInstruments(measurementID, scheduler.GetterInstruments)
}

// getUniqueInstruments extracts unique instrument names from a list of ports
func (h *MeasurementReadyHandler) getUniqueInstruments(
	ports []instrument.JsonPort,
) []instrument.Name {
	instrumentSet := make(map[instrument.Name]bool)
	var uniqueInstruments []instrument.Name

	for _, port := range ports {
		portConfig, err := h.instrumentHandler.GetPortOptions(port)
		if err != nil {
			h.log.Error(
				"Failed to get configuration for port %s: %v",
				port,
				err,
			)
			continue
		}

		if !instrumentSet[portConfig.Instrument] {
			instrumentSet[portConfig.Instrument] = true
			uniqueInstruments = append(uniqueInstruments, portConfig.Instrument)
		}
	}

	return uniqueInstruments
}

// triggerGetterInstruments sends TRIGGER commands to arm all getter instruments
func (h *MeasurementReadyHandler) triggerGetterInstruments(
	measurementID instrument.MeasurementID,
	getterInstruments []instrument.Name,
) {
	for _, instrumentName := range getterInstruments {
		if err := h.sendTriggerCommand(instrumentName, measurementID); err != nil {
			h.log.Error(
				"Failed to send %s command to arm instrument %s: %v",
				TriggerMessage,
				instrumentName,
				err,
			)
		}
	}
}

// sendTriggerCommand sends a TRIGGER command to an instrument
func (h *MeasurementReadyHandler) sendTriggerCommand(
	instrumentName instrument.Name,
	measurementID instrument.MeasurementID,
) error {
	triggerCommand := api.Trigger{
		Timestamp: time.Now().UnixMicro(),
		ProcessId: int64(measurementID.ProcessId),
		ChunkId:   int64(measurementID.ChunkId),
	}
	triggerCommandData, err := json.Marshal(triggerCommand)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal %s command: %w",
			TriggerMessage,
			err,
		)
	}
	subject := fmt.Sprintf("%s.%s", TriggerMessage, instrumentName)
	if err := h.nc.Publish(subject, triggerCommandData); err != nil {
		return fmt.Errorf(
			"failed to publish "+TriggerMessage+" command: %w",
			err,
		)
	}
	h.log.Debug(
		"Sent %s command to %s instrument for %+v",
		TriggerMessage,
		instrumentName,
		measurementID,
	)
	return nil
}

// handleExecuting processes EXECUTING messages from instruments
func (h *MeasurementReadyHandler) handleExecuting(msg *nats.Msg) {
	var executing api.Executing
	h.log.Debug(
		"Received "+ExecutingMessage+": %s", string(msg.Data),
	)
	if err := json.Unmarshal(msg.Data, &executing); err != nil {
		h.log.Error(
			"Failed to unmarshal "+ExecutingMessage+": %v", err,
		)
		return
	}

	measurementID := instrument.MeasurementID{
		ProcessId: instrument.ID(executing.ProcessId),
		ChunkId:   instrument.ID(executing.ChunkId),
	}
	if measurementID.ProcessId == 0 {
		h.log.Error(
			"ProcessId not found in EXECUTING message",
		)
		return
	}

	if measurementID.ChunkId == 0 {
		h.log.Error(
			"ChunkId not found in EXECUTING message",
		)
		return
	}

	// Subject format: "EXECUTING.<instrument_name>"
	subjectParts := strings.Split(msg.Subject, ".")
	if len(subjectParts) < 2 {
		h.log.Error(
			"Invalid EXECUTING subject format: %s", msg.Subject,
		)
		return
	}
	instrumentName := instrument.Name(subjectParts[1])

	h.log.Debug(
		"Processing EXECUTING from instrument: %s for %+v",
		instrumentName,
		measurementID,
	)

	h.mutex.Lock()
	defer h.mutex.Unlock()

	schedulerMap, exists := h.schedulers[measurementID.ProcessId]
	if !exists {
		h.log.Error(
			"No scheduler map found for ProcessId %d",
			measurementID.ProcessId,
		)
		return
	}

	scheduler, exists := schedulerMap[measurementID.ChunkId]
	if !exists {
		h.log.Error(
			"No scheduler found for %+v", measurementID,
		)
		return
	}

	if !slices.Contains(scheduler.GetterInstruments, instrumentName) {
		h.log.Error(
			"Instrument %s not found in getter instruments for %+v",
			instrumentName,
			measurementID,
		)
		return
	}
	scheduler.TriggeredGetterChecklist[instrumentName] = true
	h.log.Debug(
		"Marked getter instrument %s as triggered for %+v",
		instrumentName,
		measurementID,
	)
	if !scheduler.gettersAreTriggered() {
		return
	}
	scheduler.resetGettersTriggered()
	h.log.Debug("Reset triggered getter checklist for %+v", measurementID)
	go h.handleAllGettersTriggered(
		measurementID,
		scheduler.MasterTriggerInstruments,
	)
}

// handleAllGettersTriggered handles when all getter instruments are triggered
func (h *MeasurementReadyHandler) handleAllGettersTriggered(
	measurementID instrument.MeasurementID,
	triggerNames []instrument.Name,
) {
	h.log.Info(
		"All getter instruments triggered for %+v, triggering %d setter instruments: %v",
		measurementID,
		len(triggerNames),
		triggerNames,
	)

	for _, instrumentName := range triggerNames {
		if err := h.sendTriggerCommand(instrumentName, measurementID); err != nil {
			h.log.Error(
				"Failed to send %s command to register triggers instrument %s: %v",
				TriggerMessage,
				instrumentName,
				err,
			)
		}
	}
}

// handleReturnData processes RETURN_DATA responses from buffered measurements
func (h *MeasurementReadyHandler) handleReturnData(msg *nats.Msg) {
	var returnData api.ReturnData
	h.log.Debug(
		"Received "+ReturnDataMessage+": %s", string(msg.Data),
	)
	if err := json.Unmarshal(msg.Data, &returnData); err != nil {
		h.log.Error(
			"Failed to unmarshal "+ReturnDataMessage+": %v", err,
		)
		return
	}
	// Extract instrument name from subject (RETURN_DATA.<instrument_name>)
	subjectParts := strings.Split(msg.Subject, ".")
	if len(subjectParts) < 2 {
		h.log.Error(
			"Invalid %s subject format: %s",
			ReturnDataMessage,
			msg.Subject,
		)
		return
	}
	instrumentName := instrument.Name(subjectParts[1])
	measurementID := instrument.MeasurementID{
		ProcessId: instrument.ID(returnData.ProcessId),
		ChunkId:   instrument.ID(returnData.ChunkId),
	}
	if measurementID.ProcessId == 0 {
		h.log.Error(
			"ProcessId not found in %s message",
			ReturnDataMessage,
		)
		return
	}

	if measurementID.ChunkId == 0 {
		h.log.Error(
			"ChunkId not found in %s message",
			ReturnDataMessage,
		)
		return
	}

	h.log.Debug(
		"Processing %s: property=%s, index=%d, data=%v, measurementID=%+v",
		ReturnDataMessage,
		returnData.Property,
		returnData.Index,
		returnData.Data,
		measurementID,
	)
	port, exists := h.instrumentHandler.Instruments[instrumentName].Ports[instrument.PropertyName(returnData.Property)][instrument.Index(strconv.FormatInt(returnData.Index, 10))]
	if !exists {
		h.log.Error(
			"Failed to find port for %s (property: %s, index: %d, measurementID: %+v)",
			ReturnDataMessage,
			returnData.Property,
			returnData.Index,
			measurementID,
		)
		return
	}

	h.log.Debug(
		"Found port '%s' for %s (property: %s, index: %d, measurementID: %+v)",
		port,
		ReturnDataMessage,
		returnData.Property,
		returnData.Index,
		measurementID,
	)

	// Find the scheduler for this measurement
	h.mutex.Lock()
	defer h.mutex.Unlock()

	schedulerMap, exists := h.schedulers[measurementID.ProcessId]
	if !exists {
		h.log.Error(
			"No scheduler map found for ProcessId %d",
			measurementID.ProcessId,
		)
		return
	}

	scheduler, exists := schedulerMap[measurementID.ChunkId]
	if !exists {
		h.log.Error(
			"No scheduler found for %+v", measurementID,
		)
		return
	}
	if !scheduler.containsGetter(port) {
		h.log.Error(
			"Port %s not found in getters for %+v",
			port,
			measurementID,
		)
		return
	}
	scheduler.storeData(port, returnData.Data)
	h.log.Debug(
		"Stored result for port %s, %+v (%d/%d received): %v",
		port,
		measurementID,
		scheduler.ReceivedReturns,
		scheduler.ExpectedReturns,
		returnData.Data,
	)
	if !scheduler.allDataHere() {
		return
	}
	h.sendProcessDataForBuffered(measurementID, scheduler.Results)
}

// sendProcessDataForBuffered sends the collected buffered data as PROCESS_DATA
func (h *MeasurementReadyHandler) sendProcessDataForBuffered(
	measurementID instrument.MeasurementID,
	results map[instrument.JsonPort]any,
) {
	schedulerMap, exists := h.schedulers[measurementID.ProcessId]
	if !exists {
		h.log.Error(
			"No scheduler map found for ProcessId %d",
			measurementID.ProcessId,
		)
		return
	}

	dataBytes, err := json.Marshal(results)
	if err != nil {
		h.log.Error(
			"Failed to marshal buffered results for %+v: %v",
			measurementID,
			err,
		)
		return
	}
	processData := api.ProcessData{
		Data:      string(dataBytes),
		ProcessId: int64(measurementID.ProcessId),
		Timestamp: time.Now().UnixMicro(),
	}
	processDataBytes, err := json.Marshal(processData)
	if err != nil {
		h.log.Error(
			"Failed to marshal "+ProcessDataMessage+" for %+v: %v",
			measurementID,
			err,
		)
		return
	}

	if err := h.nc.Publish(ProcessDataMessage, processDataBytes); err != nil {
		h.log.Error(
			"Failed to publish %s for %+v: %v",
			ProcessDataMessage,
			measurementID,
			err,
		)
		return
	}
	h.log.Info(
		"Sent %s for buffered measurement %+v with %d results",
		ProcessDataMessage,
		measurementID,
		len(results),
	)

	// Clean up the scheduler
	delete(schedulerMap, measurementID.ChunkId)
	if len(schedulerMap) == 0 {
		delete(h.schedulers, measurementID.ProcessId)
	}
	h.markMeasurementComplete()
}

func convertToJsonPorts(strs []string) ([]instrument.JsonPort, error) {
	result := make([]instrument.JsonPort, len(strs))
	var errorMsgs []string

	for i, s := range strs {
		fixed_bytes, err1 := json.Marshal(s)
		if err1 != nil {
			errorMsgs = append(
				errorMsgs,
				fmt.Sprintf("marshal error for string %d (%q): %v", i, s, err1),
			)
			continue
		}

		err2 := json.Unmarshal(fixed_bytes, &result[i])
		if err2 != nil {
			errorMsgs = append(
				errorMsgs,
				fmt.Sprintf(
					"unmarshal error for string %d (%q): %v",
					i,
					s,
					err2,
				),
			)
		}
	}

	if len(errorMsgs) > 0 {
		return result, fmt.Errorf(
			"failed to convert some strings to JsonPorts: %s",
			strings.Join(errorMsgs, "; "),
		)
	}

	return result, nil
}
