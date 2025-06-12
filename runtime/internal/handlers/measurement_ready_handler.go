package handlers

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
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
	MeasurementReadyHandlerName = "MEASUREMENT_READY_HANDLER"

	// Message type names for logging
	MeasurementReadyMessage = "MEASUREMENT_READY"
	ProcessDataMessage      = "PROCESS_DATA"
	GetMessage              = "GET"
	ReturnGetMessage        = "RETURN_GET"
	TriggerMessage          = "TRIGGER"
	ReturnDataMessage       = "RETURN_DATA"
	UploadDataMessage       = "UPLOAD_DATA"
)

type ID int64

// PendingGet tracks GET commands waiting for RETURN_GET responses
type PendingGet struct {
	Port      instrument.JsonPort     // Port for the GET command
	ProcessId ID                      // Process ID for this GET operation
	GetId     string                  // Unique identifier for this GET operation
	Property  instrument.PropertyName // Property used in the GET command
	Index     instrument.Index        // Index used in the GET command
}

// PendingBufferedMeasurement tracks buffered measurements waiting for
// RETURN_DATA
type PendingBufferedMeasurement struct {
	ProcessId         ID
	GetterPorts       []instrument.JsonPort       // Original getter ports
	GetterInstruments []instrument.Name           // Instruments that need to be armed
	SetterPort        instrument.JsonPort         // The single setter port
	ExpectedReturns   int                         // Number of RETURN_DATA messages expected
	ReceivedReturns   int                         // Number of RETURN_DATA messages received
	Results           map[instrument.JsonPort]any // Port -> Data mapping
	ArmedCount        int                         // Number of instruments successfully armed
}

// MeasurementReadyHandler handles MEASUREMENT_READY requests
type MeasurementReadyHandler struct {
	logger                      *logging.Logger
	nc                          *nats.Conn
	subscription                *nats.Subscription
	returnGetSub                *nats.Subscription
	returnDataSub               *nats.Subscription
	instrumentHandler           *instrument.Handler
	config                      *config.Config
	measurementStack            *measure.MeasurementStack
	currentMeasurement          *measure.MeasurementStackItem
	isProcessing                bool
	pendingGets                 map[string]*PendingGet                          // GetId -> PendingGet
	getResults                  map[ID]map[instrument.JsonPort]any              // ProcessId -> Port -> Value
	pendingBufferedMeasurements map[ID]*PendingBufferedMeasurement              // ProcessId -> PendingBufferedMeasurement
	portConfigCache             map[instrument.JsonPort]*instrument.PortOptions // Cache port configs locally
	mutex                       sync.RWMutex
}

// NewMeasurementReadyHandler creates a new handler
func NewMeasurementReadyHandler(
	logger *logging.Logger,
	instrumentHandler *instrument.Handler,
	cfg *config.Config,
) *MeasurementReadyHandler {
	return &MeasurementReadyHandler{
		logger:            logger,
		instrumentHandler: instrumentHandler,
		config:            cfg,
		measurementStack:  &measure.MeasurementStack{},
		isProcessing:      false,
		pendingGets:       make(map[string]*PendingGet),
		getResults:        make(map[ID]map[instrument.JsonPort]any),
		pendingBufferedMeasurements: make(
			map[ID]*PendingBufferedMeasurement,
		),
		portConfigCache: make(
			map[instrument.JsonPort]*instrument.PortOptions,
		),
	}
}

// Subscribe starts listening for MEASUREMENT_READY requests
func (h *MeasurementReadyHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error

	// Subscribe to MEASUREMENT_READY
	h.subscription, err = nc.Subscribe(
		MeasurementReadyMessage,
		h.handleMeasurementReady,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+MeasurementReadyMessage+": %w",
			err,
		)
	}

	// Subscribe to RETURN_GET responses
	h.returnGetSub, err = nc.Subscribe(
		ReturnGetMessage+".>",
		h.handleReturnGet,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+ReturnGetMessage+": %w",
			err,
		)
	}

	// Subscribe to RETURN_DATA responses for buffered measurements
	h.returnDataSub, err = nc.Subscribe(
		ReturnDataMessage+".>",
		h.handleReturnData,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+ReturnDataMessage+": %w",
			err,
		)
	}

	h.logger.Info(
		MeasurementReadyHandlerName,
		"Subscribed to "+MeasurementReadyMessage+", "+ReturnGetMessage+".>, and "+ReturnDataMessage+".>",
	)
	return nil
}

// Unsubscribe stops listening for commands
func (h *MeasurementReadyHandler) Unsubscribe() error {
	var errs []error

	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
			errs = append(errs, err)
		}
		h.subscription = nil
	}

	if h.returnGetSub != nil {
		if err := h.returnGetSub.Unsubscribe(); err != nil {
			errs = append(errs, err)
		}
		h.returnGetSub = nil
	}

	if h.returnDataSub != nil {
		if err := h.returnDataSub.Unsubscribe(); err != nil {
			errs = append(errs, err)
		}
		h.returnDataSub = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to unsubscribe: %v", errs)
	}

	h.logger.Info(
		MeasurementReadyHandlerName,
		"Unsubscribed from "+MeasurementReadyMessage+", "+ReturnGetMessage+", and "+ReturnDataMessage,
	)
	return nil
}

// handleMeasurementReady processes incoming MEASUREMENT_READY requests
func (h *MeasurementReadyHandler) handleMeasurementReady(msg *nats.Msg) {
	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Received "+MeasurementReadyMessage+": %s",
			string(msg.Data),
		),
	)

	// Parse the incoming message
	var measurementReady api.MeasurementReady
	if err := json.Unmarshal(msg.Data, &measurementReady); err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to unmarshal "+MeasurementReadyMessage+": %v",
				err,
			),
		)
		return
	}
	// Create stack item and add to queue
	stackItem := measure.MeasurementStackItem{
		MeasurementReady: measurementReady,
		Timestamp:        time.Now(),
		Priority:         0, // Default priority
	}

	h.measurementStack.Push(stackItem)
	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Queued measurement for ProcessId %d. Queue size: %d",
			measurementReady.ProcessId,
			h.measurementStack.Size(),
		),
	)

	// Try to process the next measurement if not currently processing
	h.tryProcessNextMeasurement()
}

// tryProcessNextMeasurement attempts to start processing the next queued
// measurement
func (h *MeasurementReadyHandler) tryProcessNextMeasurement() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.isProcessing {
		h.logger.Debug(
			MeasurementReadyHandlerName,
			"Already processing a measurement, skipping",
		)
		return
	}

	stackItem, hasNext := h.measurementStack.Pop()
	if !hasNext {
		h.logger.Debug(
			MeasurementReadyHandlerName,
			"No measurements in queue",
		)
		return
	}

	h.isProcessing = true
	h.currentMeasurement = &stackItem

	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Starting processing of measurement ProcessId %d. Remaining in queue: %d",
			stackItem.MeasurementReady.ProcessId,
			h.measurementStack.Size(),
		),
	)

	// Process the measurement asynchronously to avoid blocking
	go h.processMeasurement(stackItem.MeasurementReady)
}

// processMeasurement handles the actual measurement processing
func (h *MeasurementReadyHandler) processMeasurement(
	msg api.MeasurementReady,
) {
	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Processing %s measurement for ProcessId %d (Getters: %d, Setters: %d)",
			map[bool]string{true: "buffered", false: "unbuffered"}[msg.Buffered],
			msg.ProcessId,
			len(msg.Getters),
			len(msg.Setters),
		),
	)

	if msg.Buffered {
		h.handleBufferedMeasurement(msg)
	} else {
		h.handleUnbufferedMeasurement(msg)
	}
}

// markMeasurementComplete marks the current measurement as complete and tries
// to process the next one
func (h *MeasurementReadyHandler) markMeasurementComplete() {
	h.mutex.Lock()
	h.isProcessing = false
	h.currentMeasurement = nil
	h.mutex.Unlock()

	h.logger.Debug(
		MeasurementReadyHandlerName,
		"Measurement processing complete, checking for next measurement",
	)

	// Try to process the next measurement
	h.tryProcessNextMeasurement()
}

type (
	Setter  map[instrument.PropertyName]any
	Setters map[instrument.JsonPort]Setter
)

// handleUnbufferedMeasurement handles unbuffered measurements (Option 1)
func (h *MeasurementReadyHandler) handleUnbufferedMeasurement(
	msg api.MeasurementReady,
) {
	if len(msg.Getters) == 0 {
		h.logger.Error(
			MeasurementReadyHandlerName,
			"No getters specified for unbuffered measurement",
		)
		return
	}
	for i, setter := range msg.Setters {
		var setters Setters

		err :=json.Unmarshal(setter, &setters)
		if err != nil {
			h.logger.Error(
				MeasurementReadyHandlerName,
				fmt.Sprintf("Failed to unmarshal setters: %v",
				err,
				),
				)
		}
		for 

		h.instrumentHandler.handleUpdateDaemonProperty()

	// TODO: need to handle setting

	// Initialize result map for this process
	h.mutex.Lock()
	h.getResults[ID(msg.ProcessId)] = make(
		map[instrument.JsonPort]any,
	)
	h.mutex.Unlock()

	// Send GET commands for each getter
	for _, port := range msg.Getters {
		if err := h.sendGetCommand(instrument.JsonPort(port), ID(msg.ProcessId)); err != nil {
			h.logger.Error(
				MeasurementReadyHandlerName,
				fmt.Sprintf(
					"Failed to send "+GetMessage+" command for port %s: %v",
					port,
					err,
				),
			)
		}
	}
}

// handleBufferedMeasurement handles buffered measurements (Option 2)
func (h *MeasurementReadyHandler) handleBufferedMeasurement(
	measurementReady api.MeasurementReady,
) {
	// Validate that there's only one setter
	if len(measurementReady.Setters) == 0 {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"No setters specified for buffered measurement ProcessId %d",
				measurementReady.ProcessId,
			),
		)
		return
	}

	if len(measurementReady.Setters) > 1 {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Multiple setters specified for buffered measurement ProcessId %d, using only the first one: %v",
				measurementReady.ProcessId,
				measurementReady.Setters,
			),
		)
	}

	setterPort := measurementReady.Setters[0] // TODO: actually set this

	// Get unique instruments from getters
	uniqueInstruments := h.getUniqueInstruments(
		convertToJsonPorts(measurementReady.Getters),
	)
	if len(uniqueInstruments) == 0 {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"No valid getter instruments found for ProcessId %d",
				measurementReady.ProcessId,
			),
		)
		return
	}

	// Initialize pending buffered measurement
	h.mutex.Lock()
	h.pendingBufferedMeasurements[ID(measurementReady.ProcessId)] = &PendingBufferedMeasurement{
		ProcessId:         ID(measurementReady.ProcessId),
		GetterPorts:       convertToJsonPorts(measurementReady.Getters),
		GetterInstruments: uniqueInstruments,
		SetterPort:        instrument.JsonPort(setterPort),
		ExpectedReturns: len(
			measurementReady.Getters,
		),
		ReceivedReturns: 0,
		Results:         make(map[instrument.JsonPort]any),
		ArmedCount:      0,
	}
	h.mutex.Unlock()

	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Starting buffered measurement for ProcessId %d: %d getter instruments to arm, setter port: %s",
			measurementReady.ProcessId,
			len(uniqueInstruments),
			setterPort,
		),
	)

	// Step 1: Arm all getter instruments
	h.armGetterInstruments(
		ID(measurementReady.ProcessId),
		convertToJsonPorts(measurementReady.Getters),
	)
}

// getCachedPortConfiguration gets a port configuration with local caching
func (h *MeasurementReadyHandler) getCachedPortConfiguration(
	port instrument.JsonPort,
) (*instrument.PortOptions, error) {
	h.mutex.RLock()
	if cached, exists := h.portConfigCache[port]; exists {
		h.mutex.RUnlock()
		h.logger.Debug(
			MeasurementReadyHandlerName,
			fmt.Sprintf("Found cached port configuration for %s", port),
		)
		return cached, nil
	}
	h.mutex.RUnlock()

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Port %s not in cache, fetching from instrument handler",
			port,
		),
	)

	// Not in local cache, get from instrument handler
	portConfig, err := h.instrumentHandler.GetPortOptions(port)
	if err != nil {
		h.logger.Debug(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to get port configuration for %s: %v",
				port,
				err,
			),
		)
		return nil, err
	}

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Successfully retrieved port configuration for %s: instrument=%s, index=%s, properties=%v",
			port,
			portConfig.Instrument,
			portConfig.Index,
			portConfig.Properties,
		),
	)

	// Cache locally for future use
	h.mutex.Lock()
	h.portConfigCache[port] = portConfig
	h.mutex.Unlock()

	return portConfig, nil
}

// sendGetCommand sends a GET command for a specific port
func (h *MeasurementReadyHandler) sendGetCommand(
	port instrument.JsonPort,
	processId ID,
) error {
	// Get port configuration using cached version
	portConfig, err := h.getCachedPortConfiguration(port)
	if err != nil {
		return fmt.Errorf(
			"failed to get port configuration for %s: %w",
			port,
			err,
		)
	}

	// Generate unique ID for this GET operation
	getId := fmt.Sprintf("%d_%s_%d", processId, port, time.Now().UnixNano())

	// Store pending GET with property and index for matching
	h.mutex.Lock()
	h.pendingGets[getId] = &PendingGet{
		Port:      port,
		ProcessId: processId,
		GetId:     getId,
		Property:  portConfig.Properties[0], // Use first property
		Index:     portConfig.Index,
	}
	h.mutex.Unlock()

	// Use the first property for the GET command
	// Note: If a port has multiple properties, we might need to handle this
	// differently
	if len(portConfig.Properties) == 0 {
		// Clean up pending GET on error
		h.mutex.Lock()
		delete(h.pendingGets, getId)
		h.mutex.Unlock()
		return fmt.Errorf("port %s has no properties", port)
	}

	// Create GET command using the port configuration
	indexNumber, err := strconv.ParseInt(string(portConfig.Index), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse index for port %s: %w", port, err)
	}
	getCommand := api.Get{
		Index:    indexNumber,
		Property: string(portConfig.Properties[0]), // Use first property
	}

	// Marshal the command
	getCommandData, err := json.Marshal(getCommand)
	if err != nil {
		// Clean up pending GET on error
		h.mutex.Lock()
		delete(h.pendingGets, getId)
		h.mutex.Unlock()
		return fmt.Errorf("failed to marshal "+GetMessage+" command: %w", err)
	}

	// Send GET command to specific instrument
	subject := fmt.Sprintf("%s.%s", GetMessage, portConfig.Instrument)
	if err := h.nc.Publish(subject, getCommandData); err != nil {
		// Clean up pending GET on error
		h.mutex.Lock()
		delete(h.pendingGets, getId)
		h.mutex.Unlock()
		return fmt.Errorf("failed to publish "+GetMessage+" command: %w", err)
	}

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Sent "+GetMessage+" command for port %s to %s (Property: %s, Index: %s, GetId: %s)",
			port,
			portConfig.Instrument,
			portConfig.Properties[0],
			portConfig.Index,
			getId,
		),
	)

	return nil
}

// handleReturnGet processes RETURN_GET responses
func (h *MeasurementReadyHandler) handleReturnGet(msg *nats.Msg) {
	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf("Received "+ReturnGetMessage+": %s", string(msg.Data)),
	)

	// Parse the RETURN_GET response
	var returnGet api.ReturnGet
	if err := json.Unmarshal(msg.Data, &returnGet); err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf("Failed to unmarshal "+ReturnGetMessage+": %v", err),
		)
		return
	}

	// Find the corresponding pending GET using Index and Property
	h.mutex.Lock()
	defer h.mutex.Unlock()

	var matchingGet *PendingGet
	var matchingGetId string

	// Find the GET that matches this RETURN_GET by Index and Property
	for getId, pendingGet := range h.pendingGets {
		if pendingGet.Index == instrument.Index(
			strconv.FormatInt(returnGet.Index, 10),
		) &&
			pendingGet.Property == instrument.PropertyName(returnGet.Property) {
			matchingGet = pendingGet
			matchingGetId = getId
			break
		}
	}

	if matchingGet == nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"No pending "+GetMessage+" found for "+ReturnGetMessage+" (Property: %s, Index: %d)",
				returnGet.Property,
				returnGet.Index,
			),
		)
		return
	}

	// Store the result
	if h.getResults[matchingGet.ProcessId] == nil {
		h.getResults[matchingGet.ProcessId] = make(
			map[instrument.JsonPort]any,
		)
	}
	h.getResults[matchingGet.ProcessId][matchingGet.Port] = returnGet.Value

	delete(h.pendingGets, matchingGetId)

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Stored result for port %s, ProcessId %d: %v",
			matchingGet.Port,
			matchingGet.ProcessId,
			returnGet.Value,
		),
	)

	h.checkAndSendProcessData(matchingGet.ProcessId)
	h.markMeasurementComplete()
}

// getUniqueInstruments extracts unique instrument names from a list of ports
func (h *MeasurementReadyHandler) getUniqueInstruments(
	ports []instrument.JsonPort,
) []instrument.Name {
	instrumentSet := make(map[instrument.Name]bool)
	var uniqueInstruments []instrument.Name

	for _, port := range ports {
		portConfig, err := h.getCachedPortConfiguration(port)
		if err != nil {
			h.logger.Error(
				MeasurementReadyHandlerName,
				fmt.Sprintf(
					"Failed to get configuration for port %s: %v",
					port,
					err,
				),
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

// armGetterInstruments sends TRIGGER commands to arm all getter instruments
func (h *MeasurementReadyHandler) armGetterInstruments(
	processId ID,
	getterPorts []instrument.JsonPort,
) {
	// Get the first port for each unique instrument to use for trigger
	// configuration
	out := make(map[instrument.Name]instrument.JsonPort)

	for _, port := range getterPorts {
		portConfig, err := h.getCachedPortConfiguration(port)
		if err != nil {
			h.logger.Error(
				MeasurementReadyHandlerName,
				fmt.Sprintf(
					"Failed to get configuration for getter port %s: %v",
					port,
					err,
				),
			)
			continue
		}

		// Use the first port we encounter for each instrument
		if _, exists := out[portConfig.Instrument]; !exists {
			out[portConfig.Instrument] = port
		}
	}

	// Send TRIGGER command for each unique instrument
	for instrumentName, firstPort := range out {
		if err := h.sendTriggerCommand(instrumentName, firstPort, processId, true); err != nil {
			h.logger.Error(
				MeasurementReadyHandlerName,
				fmt.Sprintf(
					"Failed to send "+TriggerMessage+" command to arm instrument %s: %v",
					instrumentName,
					err,
				),
			)
		} else {
			h.incrementArmedCount(processId)
		}
	}
}

// sendTriggerCommand sends a TRIGGER command to an instrument
func (h *MeasurementReadyHandler) sendTriggerCommand(
	instrumentName instrument.Name,
	port instrument.JsonPort,
	processId ID,
	isGetter bool,
) error {
	portConfig, err := h.getCachedPortConfiguration(port)
	if err != nil {
		return fmt.Errorf(
			"failed to get port configuration for %s: %w",
			port,
			err,
		)
	}

	// Use the first property for the TRIGGER command
	if len(portConfig.Properties) == 0 {
		return fmt.Errorf("port %s has no properties", port)
	}

	// Create TRIGGER command
	indexNumber, err := strconv.ParseInt(string(portConfig.Index), 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse index for port %s: %w", port, err)
	}
	triggerCommand := api.Trigger{
		Property: string(portConfig.Properties[0]), // Use first property
		Index:    indexNumber,
	}

	// Marshal the command
	triggerCommandData, err := json.Marshal(triggerCommand)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal "+TriggerMessage+" command: %w",
			err,
		)
	}

	// Send TRIGGER command to specific instrument
	subject := fmt.Sprintf("%s.%s", TriggerMessage, instrumentName)
	if err := h.nc.Publish(subject, triggerCommandData); err != nil {
		return fmt.Errorf(
			"failed to publish "+TriggerMessage+" command: %w",
			err,
		)
	}

	action := "set"
	if isGetter {
		action = "arm"
	}

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Sent "+TriggerMessage+" command to %s instrument %s (Property: %s, Index: %s, ProcessId: %d)",
			action,
			instrumentName,
			portConfig.Properties[0],
			portConfig.Index,
			processId,
		),
	)

	return nil
}

// incrementArmedCount increments the armed count and checks if we can proceed
// to setters
func (h *MeasurementReadyHandler) incrementArmedCount(processId ID) {
	h.mutex.Lock()

	var shouldTriggerSetter bool
	if pending, exists := h.pendingBufferedMeasurements[processId]; exists {
		pending.ArmedCount++

		h.logger.Debug(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Armed instrument count for ProcessId %d: %d/%d",
				processId,
				pending.ArmedCount,
				len(pending.GetterInstruments),
			),
		)

		// If all getters are armed, proceed to trigger the setter
		shouldTriggerSetter = pending.ArmedCount >= len(
			pending.GetterInstruments,
		)
	}

	h.mutex.Unlock()

	// Call triggerSetter outside of the mutex to avoid deadlock
	if shouldTriggerSetter {
		h.triggerSetter(processId)
	}
}

// triggerSetter sends the TRIGGER command to the setter instrument
func (h *MeasurementReadyHandler) triggerSetter(processId ID) {
	pending, exists := h.pendingBufferedMeasurements[processId]
	if !exists {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"No pending buffered measurement found for ProcessId %d",
				processId,
			),
		)
		return
	}

	setterPortConfig, err := h.getCachedPortConfiguration(pending.SetterPort)
	if err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to get setter port configuration for %s: %v",
				pending.SetterPort,
				err,
			),
		)
		return
	}

	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"All getters armed for ProcessId %d, triggering setter on instrument %s",
			processId,
			setterPortConfig.Instrument,
		),
	)

	// Send TRIGGER to setter
	if err := h.sendTriggerCommand(setterPortConfig.Instrument, pending.SetterPort, processId, false); err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to send "+TriggerMessage+" command to setter instrument %s: %v",
				setterPortConfig.Instrument,
				err,
			),
		)
	}
}

// handleReturnData processes RETURN_DATA responses from buffered measurements
func (h *MeasurementReadyHandler) handleReturnData(msg *nats.Msg) {
	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf("Received "+ReturnDataMessage+": %s", string(msg.Data)),
	)

	// Parse the RETURN_DATA response
	var returnData api.ReturnData
	if err := json.Unmarshal(msg.Data, &returnData); err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf("Failed to unmarshal "+ReturnDataMessage+": %v", err),
		)
		return
	}

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Processing RETURN_DATA: property=%s, index=%d, data=%v",
			returnData.Property,
			returnData.Index,
			returnData.Data,
		),
	)

	// Find the corresponding port using property and index
	port, err := h.findPortByPropertyAndIndex(
		instrument.PropertyName(returnData.Property),
		instrument.Index(strconv.FormatInt(returnData.Index, 10)),
	)
	if err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to find port for "+ReturnDataMessage+" (property: %s, index: %d): %v",
				returnData.Property,
				returnData.Index,
				err,
			),
		)
		return
	}

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Found port '%s' for RETURN_DATA (property: %s, index: %d)",
			port,
			returnData.Property,
			returnData.Index,
		),
	)

	h.logger.Debug(
		MeasurementReadyHandlerName,
		"About to acquire mutex lock to search for pending buffered measurements",
	)

	// Find which pending buffered measurement this belongs to
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Looking for pending buffered measurements, found %d total",
			len(h.pendingBufferedMeasurements),
		),
	)

	var matchingProcessId ID
	for processId, pending := range h.pendingBufferedMeasurements {
		h.logger.Debug(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Checking ProcessId %d: GetterPorts=%v",
				processId,
				pending.GetterPorts,
			),
		)

		// Debug the exact comparison
		for i, getterPort := range pending.GetterPorts {
			h.logger.Debug(
				MeasurementReadyHandlerName,
				fmt.Sprintf(
					"Comparing getter[%d] (len=%d): %s",
					i,
					len(getterPort),
					getterPort,
				),
			)
			h.logger.Debug(
				MeasurementReadyHandlerName,
				fmt.Sprintf("With found port (len=%d): %s", len(port), port),
			)
			h.logger.Debug(
				MeasurementReadyHandlerName,
				fmt.Sprintf("Strings equal: %t", getterPort == port),
			)
		}

		// Check if this port was part of the getters for this measurement
		if h.portInGetters(port, pending.GetterPorts) {
			matchingProcessId = processId
			h.logger.Debug(
				MeasurementReadyHandlerName,
				fmt.Sprintf(
					"Found matching ProcessId %d for port %s",
					processId,
					port,
				),
			)
			break
		} else {
			h.logger.Debug(
				MeasurementReadyHandlerName,
				fmt.Sprintf("Port %s not found in getters for ProcessId %d", port, processId),
			)
		}
	}

	if matchingProcessId == 0 {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"No pending buffered measurement found for "+ReturnDataMessage+" from port %s",
				port,
			),
		)
		return
	}

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Processing RETURN_DATA for matching ProcessId %d",
			matchingProcessId,
		),
	)

	pending := h.pendingBufferedMeasurements[matchingProcessId]

	// Store the result
	pending.Results[port] = returnData.Data
	pending.ReceivedReturns++

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Stored buffered result for port %s, ProcessId %d (%d/%d received): %v",
			port,
			matchingProcessId,
			pending.ReceivedReturns,
			pending.ExpectedReturns,
			returnData.Data,
		),
	)

	// Check if we have all expected returns
	if pending.ReceivedReturns >= pending.ExpectedReturns {
		h.sendProcessDataForBuffered(matchingProcessId)
	}
}

// findPortByPropertyAndIndex finds a port name given property and index
func (h *MeasurementReadyHandler) findPortByPropertyAndIndex(
	property instrument.PropertyName,
	index instrument.Index,
) (instrument.JsonPort, error) {
	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Looking for port with property=%s, index=%s",
			property,
			index,
		),
	)

	// Get all port configurations
	portConfigurations, err := h.instrumentHandler.BuildPortConfigurations()
	if err != nil {
		return "", fmt.Errorf("failed to build port configurations: %w", err)
	}

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf("Built %d port configurations", len(portConfigurations)),
	)

	// Search for matching port
	for portName, portConfig := range portConfigurations {
		h.logger.Debug(
			MeasurementReadyHandlerName,
			fmt.Sprintf("Checking port %s: %+v", portName, portConfig),
		)

		if portConfig.Index == index {
			// Check if any of the properties match
			if slices.Contains(portConfig.Properties, property) {
				h.logger.Debug(
					MeasurementReadyHandlerName,
					fmt.Sprintf("Found matching port: %s", portName),
				)
				return portName, nil
			}
		}
	}

	return "", fmt.Errorf(
		"no port found for property %s, index %s",
		property,
		index,
	)
}

// portInGetters checks if a port was part of the getters for a specific
// measurement
func (h *MeasurementReadyHandler) portInGetters(
	port instrument.JsonPort,
	getterPorts []instrument.JsonPort,
) bool {
	return slices.Contains(getterPorts, port)
}

// sendProcessDataForBuffered sends the collected buffered data as PROCESS_DATA
func (h *MeasurementReadyHandler) sendProcessDataForBuffered(processId ID) {
	pending, exists := h.pendingBufferedMeasurements[processId]
	if !exists {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"No pending buffered measurement found for ProcessId %d",
				processId,
			),
		)
		return
	}

	// Marshal the results to JSON string
	dataBytes, err := json.Marshal(pending.Results)
	if err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to marshal buffered results for ProcessId %d: %v",
				processId,
				err,
			),
		)
		return
	}

	// Create PROCESS_DATA message (same as unbuffered)
	processData := api.ProcessData{
		Data:      string(dataBytes),
		ProcessId: int64(processId),
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal the PROCESS_DATA
	processDataBytes, err := json.Marshal(processData)
	if err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to marshal "+ProcessDataMessage+" for ProcessId %d: %v",
				processId,
				err,
			),
		)
		return
	}

	if err := h.nc.Publish(ProcessDataMessage, processDataBytes); err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to publish "+ProcessDataMessage+" for ProcessId %d: %v",
				processId,
				err,
			),
		)
		return
	}
	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Sent "+ProcessDataMessage+" for buffered measurement ProcessId %d with %d results",
			processId,
			len(pending.Results),
		),
	)

	delete(h.pendingBufferedMeasurements, processId)
	h.markMeasurementComplete()
}

// checkAndSendProcessData checks if all GET results are collected and sends
// PROCESS_DATA
func (h *MeasurementReadyHandler) checkAndSendProcessData(processId ID) {
	// Count pending GETs for this process
	pendingCount := 0
	expectedCount := 0

	for _, pendingGet := range h.pendingGets {
		if pendingGet.ProcessId == processId {
			pendingCount++
		}
	}

	// Count expected results
	if results, exists := h.getResults[processId]; exists {
		expectedCount = len(results)
	}

	// If no pending GETs remain for this process, send PROCESS_DATA
	if pendingCount == 0 && expectedCount > 0 {
		if err := h.sendProcessData(processId); err != nil {
			h.logger.Error(
				MeasurementReadyHandlerName,
				fmt.Sprintf(
					"Failed to send "+ProcessDataMessage+" for ProcessId %d: %v",
					processId,
					err,
				),
			)
		}
	}
}

// sendProcessData sends the collected data as PROCESS_DATA
func (h *MeasurementReadyHandler) sendProcessData(processId ID) error {
	results, exists := h.getResults[processId]
	if !exists {
		return fmt.Errorf("no results found for ProcessId %d", processId)
	}

	// Marshal the results to JSON string
	dataBytes, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	// Create PROCESS_DATA message
	processData := api.ProcessData{
		Data:      string(dataBytes),
		ProcessId: int64(processId),
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal the PROCESS_DATA
	processDataBytes, err := json.Marshal(processData)
	if err != nil {
		return fmt.Errorf("failed to marshal "+ProcessDataMessage+": %w", err)
	}

	// Send PROCESS_DATA
	if err := h.nc.Publish(ProcessDataMessage, processDataBytes); err != nil {
		return fmt.Errorf("failed to publish "+ProcessDataMessage+": %w", err)
	}

	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Sent "+ProcessDataMessage+" for ProcessId %d with %d measurements",
			processId,
			len(results),
		),
	)

	// Clean up results for this process
	delete(h.getResults, processId)

	return nil
}

func convertToJsonPorts(strings []string) []instrument.JsonPort {
	result := make([]instrument.JsonPort, len(strings))
	for i, s := range strings {
		result[i] = instrument.JsonPort(s)
	}
	return result
}
