package measure

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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

	if h.isProcessing {
		h.mutex.Unlock()
		h.log.Debug("Already processing a measurement, skipping")
		return
	}
	stackItem, hasNext := h.measurementStack.Pop()
	if !hasNext {
		h.mutex.Unlock()
		h.log.Debug("No measurements in queue")
		return
	}
	h.isProcessing = true
	h.currentMeasurement = &stackItem
	h.mutex.Unlock()

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

	h.createSchedulerForMeasurement(msg, chunkId)

	measurementID := instrument.MeasurementID{
		ProcessId: instrument.ID(msg.ProcessId),
		ChunkId:   instrument.ID(chunkId),
	}

	h.log.Info(
		"Processing SET commands for ProcessId %d, ChunkId %d (Setters: %d)",
		msg.ProcessId,
		chunkId,
		len(msg.Setters),
	)

	totalInstructions, err := collectAllRequirements(msg.Requirements)
	if err != nil {
		h.log.Error("Failed to collect all set instructions: %s", err)
	}

	// Extract all unique ports from instructions
	seen := make(map[instrument.JsonPort]bool)
	ports := make([]instrument.JsonPort, 0, len(totalInstructions))
	for _, instruction := range totalInstructions {
		if !seen[instruction.Port] {
			seen[instruction.Port] = true
			ports = append(ports, instruction.Port)
		}
	}

	// Get all port options in a single batch call
	portOptionsMap, err := h.instrumentHandler.GetMultiplePortOptions(ports)
	if err != nil {
		h.log.Error("Failed to get port configurations for ports: %v", err)
		return
	}

	// Group instructions by port
	instructionsByPort := make(map[instrument.JsonPort][]Instructions)
	for _, instruction := range totalInstructions {
		instructionsByPort[instruction.Port] = append(
			instructionsByPort[instruction.Port],
			instruction,
		)
	}

	// Begin sorting the instructions by instrument
	sortedInstructions := make(
		[]*InstrumentInstructions,
		0,
		len(totalInstructions),
	)
	h.log.Debug("The total instructions to send are: %#v", totalInstructions)

	// Create InstrumentInstructions for each unique instrument
	instrumentMap := make(map[instrument.Name]*InstrumentInstructions)
	for port, options := range portOptionsMap {
		instrumentName := options.Instrument

		// Create InstrumentInstructions if it doesn't exist
		if _, exists := instrumentMap[instrumentName]; !exists {
			instrumentMap[instrumentName] = &InstrumentInstructions{
				Name: instrumentName,
			}
			sortedInstructions = append(
				sortedInstructions,
				instrumentMap[instrumentName],
			)
		}

		// Add all instructions for this port
		for _, instruction := range instructionsByPort[port] {
			instrumentMap[instrumentName].append(instruction)
		}
	}
	err = h.armInstructions(measurementID, sortedInstructions)
	if err != nil {
		h.log.Error("Could not arm instructions: %s", err)
	}
	h.sendInstructions(measurementID, sortedInstructions)
}

func (h *MeasurementReadyHandler) armInstructions(
	measurementID instrument.MeasurementID,
	ii []*InstrumentInstructions,
) error {
	scheduler, err := h.selectScheduler(measurementID)
	if err != nil {
		return err
	}
	h.log.Debug("The total setter ports are %+v", scheduler.SetterPorts())
	h.log.Debug("The measurement ID to send is %v", measurementID)
	for _, instructions := range ii {
		name := instructions.Name
		// Need to determine if this instruction is for a setter or getter or
		// just a requirement
		getters := make([]instrument.PropertyIndex, 0)
		setters := make([]instrument.PropertyIndex, 0)
		if scheduler.GetterDeployment.Contains(name) {
			getters = scheduler.GetterDeployment.GetPrimaryPropertyIndexes()
		}
		if scheduler.SetterDeployment.Contains(name) {
			setters = scheduler.SetterDeployment.GetPrimaryPropertyIndexes()
		}
		armvalue := ArmValue{
			GetterPropertyPairs: getters,
			SetterPropertyPairs: setters,
		}
		instructions.arm(armvalue)
	}
	return nil
}

// sendInstructions sends all SET, TIMEOUT, and ARM instructions to be processed
// before published
func (h *MeasurementReadyHandler) sendInstructions(
	measurementID instrument.MeasurementID,
	ii []*InstrumentInstructions,
) {
	h.log.Info("Sending %d instructions for %+v", len(ii), measurementID)

	// Parallelize instruction sending since each instrument is independent
	var wg sync.WaitGroup
	for _, instructions := range ii {
		wg.Add(1)
		go func(instr *InstrumentInstructions) {
			defer wg.Done()

			// Send regular SET instructions
			for _, setInstruction := range instr.SetInstructions {
				h.instrumentHandler.SetProperty(setInstruction, measurementID)
			}

			// Send TIMEOUT instructions directly
			for _, timeoutInstruction := range instr.TimeoutInstructions {
				directInstruction := instrument.DirectSetInstruction{
					InstrumentName: instr.Name,
					Property:       timeoutInstruction.Property,
					Index:          -1, // Global instrument command
					Value:          timeoutInstruction.Value,
				}
				h.instrumentHandler.SendDirectSetInstruction(
					directInstruction,
					measurementID,
				)
			}

			// Send ARM instructions directly
			for _, armInstruction := range instr.ArmInstruction {
				directInstruction := instrument.DirectSetInstruction{
					InstrumentName: instr.Name,
					Property:       armInstruction.Property,
					Index:          -1, // Global instrument command
					Value:          armInstruction.Value,
				}
				h.instrumentHandler.SendDirectSetInstruction(
					directInstruction,
					measurementID,
				)
			}
		}(instructions)
	}
	wg.Wait()
}

// createSchedulerForMeasurement creates the scheduler before sending SET
// commands
func (h *MeasurementReadyHandler) createSchedulerForMeasurement(
	msg api.MeasurementReady,
	chunkId int64,
) {
	if len(msg.Getters) == 0 {
		h.log.Warn(
			"No getters specified for measurement. Hope you are ramping on purpose.",
		)
	}
	totalInstructions, err := collectAllRequirements(msg.Requirements)
	if err != nil {
		h.log.Error("Failed to collect all required instructions: %s", err)
	}
	requiredPorts := make([]instrument.JsonPort, 0, len(totalInstructions))
	for _, instruction := range totalInstructions {
		requiredPorts = append(requiredPorts, instruction.Port)
	}

	setterPorts, err := convertToJsonPorts(msg.Setters)
	if err != nil {
		h.log.Error("Failed to convert setters to JsonPorts: %s", err)
		return
	}

	getterPorts, err := convertToJsonPorts(msg.Getters)
	if err != nil {
		h.log.Error("Failed to convert getters to JsonPorts: %s", err)
		return
	}

	// Get unique instruments from getters and setters
	getterDeployment := h.getUniqueInstruments(getterPorts)
	setterDeployment := h.getUniqueInstruments(setterPorts)
	setterInstruments := setterDeployment.GetInstruments()
	requiredDeployment := h.getUniqueInstruments(requiredPorts)
	h.log.Debug(
		"The setter instruments are %v",
		setterInstruments,
	)
	masterInstruments := setterDeployment.GetInstruments() // default for unbuffered

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

	readyChecklist := createBoolMap(setterDeployment.GetInstruments())
	triggerGetterChecklist := createBoolMap(getterDeployment.GetInstruments())
	processId := instrument.ID(msg.ProcessId)
	measurementId := instrument.MeasurementID{
		ProcessId: processId,
		ChunkId:   instrument.ID(chunkId),
	}

	// Initialize scheduler for this specific chunk
	scheduler := &MeasurementScheduler{
		ID:                       measurementId,
		GetterDeployment:         getterDeployment,
		SetterDeployment:         setterDeployment,
		RequirementDeployment:    requiredDeployment,
		MasterTriggerInstruments: masterInstruments,
		ReceivedReturns:          0,
		ExpectedReturns:          len(getterPorts),
		ReadyChecklist:           readyChecklist,
		TriggeredGetterChecklist: triggerGetterChecklist,
		Results:                  make(map[instrument.JsonPort]any),
	}

	h.log.Debug(
		"Created scheduler for %+v with setter instruments: %v, getter instruments: %v, required insturments: %v",
		scheduler.ID,
		setterInstruments,
		getterDeployment.GetInstruments(),
		requiredDeployment.GetInstruments(),
	)
	h.mutex.Lock()
	h.setScheduler(measurementId, scheduler)
	h.mutex.Unlock()
}

func (h *MeasurementReadyHandler) setScheduler(
	id instrument.MeasurementID,
	scheduler *MeasurementScheduler,
) {
	if h.schedulers[id.ProcessId] == nil {
		h.schedulers[id.ProcessId] = make(
			map[instrument.ID]*MeasurementScheduler,
		)
	}
	h.schedulers[id.ProcessId][id.ChunkId] = scheduler
}

// selectScheduler retrieves the scheduler for a given measurement ID
func (h *MeasurementReadyHandler) selectScheduler(
	id instrument.MeasurementID,
) (*MeasurementScheduler, error) {
	schedulerMap, exists := h.schedulers[id.ProcessId]
	if !exists {
		return nil, fmt.Errorf(
			"no scheduler map found for ProcessId %d",
			id.ProcessId,
		)
	}

	scheduler, exists := schedulerMap[id.ChunkId]
	if !exists {
		return nil, fmt.Errorf("no scheduler found for ChunkId %+v", id.ChunkId)
	}
	return scheduler, nil
}

// getUniqueInstruments extracts unique instrument names from a list of ports
// and returns a ScheduledPortDeployment containing the mapping
func (h *MeasurementReadyHandler) getUniqueInstruments(
	ports []instrument.JsonPort,
) *ScheduledPortDeployments {
	deployment := NewScheduledPortDeployment()

	for _, port := range ports {
		portConfig, err := h.instrumentHandler.GetPortOptions(port)
		if err != nil {
			h.log.Error("Failed to get configuration for port %s: %v",
				port,
				err,
			)
			continue
		}

		deployment.Add(portConfig.Instrument, port, portConfig)
	}

	return deployment
}

// triggerGetterInstruments sends TRIGGER commands to arm all getter instruments
func (h *MeasurementReadyHandler) triggerGetterInstruments(
	measurementID instrument.MeasurementID,
	getterInstruments []instrument.Name,
) {
	var wg sync.WaitGroup
	for _, instrumentName := range getterInstruments {
		wg.Add(1)
		go func(name instrument.Name) {
			defer wg.Done()

			if err := h.sendTriggerCommand(name, measurementID, true); err != nil {
				h.log.Error(
					"Failed to send %s command to arm instrument %s: %v",
					TriggerMessage,
					instrumentName,
					err,
				)
			}
		}(instrumentName)
	}
	wg.Wait()
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

	var wg sync.WaitGroup
	for _, instrumentName := range triggerNames {
		wg.Add(1)
		go func(name instrument.Name) {
			defer wg.Done()

			if err := h.sendTriggerCommand(name, measurementID, true); err != nil {
				h.log.Error(
					"Failed to send %s command to register triggers instrument %s: %v",
					TriggerMessage,
					name,
					err,
				)
			}
		}(instrumentName)
	}
	wg.Wait()
}

// sendTriggerCommand sends a TRIGGER command to an instrument
func (h *MeasurementReadyHandler) sendTriggerCommand(
	instrumentName instrument.Name,
	measurementID instrument.MeasurementID,
	is_setter bool,
) error {
	triggerCommand := api.Trigger{
		Timestamp: time.Now().UnixMicro(),
		ProcessId: int64(measurementID.ProcessId),
		ChunkId:   int64(measurementID.ChunkId),
		IsSetter:  is_setter,
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
