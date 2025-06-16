package measure

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

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
		measurementStack:  &MeasurementStack{},
		isProcessing:      false,
		getResults:        make(map[instrument.ID]map[instrument.JsonPort]any),
		schedulers: make(
			map[instrument.ID]map[instrument.ID]*MeasurementScheduler,
		),
		pendingMeasurements: make(
			map[instrument.ID]*MeasurementScheduler,
		),
		pendingGets: make(map[instrument.ID]any),
		NextChunkId: 1,
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

	h.log.Info("Unsubscribed from %s, %s, %s, and %s",
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
	h.log.Debug("Received %s", MeasurementReadyMessage)
	if err := json.Unmarshal(msg.Data, &measurementReady); err != nil {
		h.log.Error("Failed to unmarshal %s: %v", MeasurementReadyMessage, err)
		return
	}
	// Create stack item and add to queue with assigned ChunkId
	h.mutex.Lock()
	chunkId := h.NextChunkId
	h.NextChunkId++
	h.mutex.Unlock()

	stackItem := MeasurementStackItem{
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
	stackItem MeasurementStackItem,
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
		h.log.Error("Failed to collect all set instructions: %s", err)
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
			h.log.Error("Failed to get port configuration for setter %s: %v",
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

	readyChecklist := createBoolMap(setterInstruments)
	triggerGetterChecklist := createBoolMap(getterInstruments)

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

// getUniqueInstruments extracts unique instrument names from a list of ports
func (h *MeasurementReadyHandler) getUniqueInstruments(
	ports []instrument.JsonPort,
) []instrument.Name {
	instrumentSet := make(map[instrument.Name]bool)
	var uniqueInstruments []instrument.Name

	for _, port := range ports {
		portConfig, err := h.instrumentHandler.GetPortOptions(port)
		if err != nil {
			h.log.Error("Failed to get configuration for port %s: %v",
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
			h.log.Error("Failed to send %s command to arm instrument %s: %v",
				TriggerMessage,
				instrumentName,
				err,
			)
		}
	}
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
		return fmt.Errorf("failed to marshal %s command: %w",
			TriggerMessage,
			err,
		)
	}
	subject := fmt.Sprintf("%s.%s", TriggerMessage, instrumentName)
	if err := h.nc.Publish(subject, triggerCommandData); err != nil {
		return fmt.Errorf("failed to publish "+TriggerMessage+" command: %w",
			err,
		)
	}
	h.log.Debug("Sent %s command to %s instrument for %+v",
		TriggerMessage,
		instrumentName,
		measurementID,
	)
	return nil
}
