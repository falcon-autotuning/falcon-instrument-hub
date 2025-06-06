package handlers

import (
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	MeasurementReadyHandlerName = "MEASUREMENT_READY_HANDLER"
	MeasurementReadySubject     = "MEASUREMENT_READY.interpreter"
	ProcessDataSubject          = "PROCESS_DATA.interpreter"

	// Message type names for logging
	MeasurementReadyMessage = "MEASUREMENT_READY"
	ProcessDataMessage      = "PROCESS_DATA"
	GetMessage              = "GET"
	ReturnGetMessage        = "RETURN_GET"
	TriggerMessage          = "TRIGGER"
	ReturnDataMessage       = "RETURN_DATA"
	UploadDataMessage       = "UPLOAD_DATA"
)

// PendingGet tracks GET commands waiting for RETURN_GET responses
type PendingGet struct {
	Port      string
	ProcessId string
	GetId     string // Unique identifier for this GET operation
	Property  string // Property used in the GET command
	Index     int64  // Index used in the GET command
}

// PendingBufferedMeasurement tracks buffered measurements waiting for
// RETURN_DATA
type PendingBufferedMeasurement struct {
	ProcessId         string
	GetterPorts       []string               // Original getter ports
	GetterInstruments []string               // Instruments that need to be armed
	SetterPort        string                 // The single setter port
	ExpectedReturns   int                    // Number of RETURN_DATA messages expected
	ReceivedReturns   int                    // Number of RETURN_DATA messages received
	Results           map[string]interface{} // Port -> Data mapping
	ArmedCount        int                    // Number of instruments successfully armed
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
	pendingGets                 map[string]*PendingGet                   // GetId -> PendingGet
	getResults                  map[string]map[string]interface{}        // ProcessId -> Port -> Value
	pendingBufferedMeasurements map[string]*PendingBufferedMeasurement   // ProcessId -> PendingBufferedMeasurement
	portConfigCache             map[string]*instrument.PortConfiguration // Cache port configs locally
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
		pendingGets:       make(map[string]*PendingGet),
		getResults:        make(map[string]map[string]interface{}),
		pendingBufferedMeasurements: make(
			map[string]*PendingBufferedMeasurement,
		),
		portConfigCache: make(
			map[string]*instrument.PortConfiguration,
		),
	}
}

// Subscribe starts listening for MEASUREMENT_READY requests
func (h *MeasurementReadyHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error

	// Subscribe to MEASUREMENT_READY
	h.subscription, err = nc.Subscribe(
		MeasurementReadySubject,
		h.handleMeasurementReady,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+MeasurementReadySubject+": %w",
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
		"Subscribed to "+MeasurementReadySubject+", "+ReturnGetMessage+".>, and "+ReturnDataMessage+".>",
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

	// Determine if this is buffered or unbuffered measurement
	isBuffered := len(measurementReady.Setters) > 0

	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Processing %s measurement for ProcessId %s (Getters: %d, Setters: %d)",
			map[bool]string{true: "buffered", false: "unbuffered"}[isBuffered],
			measurementReady.ProcessId,
			len(measurementReady.Getters),
			len(measurementReady.Setters),
		),
	)

	if isBuffered {
		h.handleBufferedMeasurement(measurementReady)
	} else {
		h.handleUnbufferedMeasurement(measurementReady)
	}
}

// handleUnbufferedMeasurement handles unbuffered measurements (Option 1)
func (h *MeasurementReadyHandler) handleUnbufferedMeasurement(
	measurementReady api.MeasurementReady,
) {
	if len(measurementReady.Getters) == 0 {
		h.logger.Error(
			MeasurementReadyHandlerName,
			"No getters specified for unbuffered measurement",
		)
		return
	}

	// Initialize result map for this process
	h.mutex.Lock()
	h.getResults[measurementReady.ProcessId] = make(map[string]interface{})
	h.mutex.Unlock()

	// Send GET commands for each getter
	for _, port := range measurementReady.Getters {
		if err := h.sendGetCommand(port, measurementReady.ProcessId); err != nil {
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
				"No setters specified for buffered measurement ProcessId %s",
				measurementReady.ProcessId,
			),
		)
		return
	}

	if len(measurementReady.Setters) > 1 {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Multiple setters specified for buffered measurement ProcessId %s, using only the first one: %v",
				measurementReady.ProcessId,
				measurementReady.Setters,
			),
		)
	}

	setterPort := measurementReady.Setters[0]

	// Get unique instruments from getters
	uniqueInstruments := h.getUniqueInstruments(measurementReady.Getters)
	if len(uniqueInstruments) == 0 {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"No valid getter instruments found for ProcessId %s",
				measurementReady.ProcessId,
			),
		)
		return
	}

	// Initialize pending buffered measurement
	h.mutex.Lock()
	h.pendingBufferedMeasurements[measurementReady.ProcessId] = &PendingBufferedMeasurement{
		ProcessId:         measurementReady.ProcessId,
		GetterPorts:       measurementReady.Getters,
		GetterInstruments: uniqueInstruments,
		SetterPort:        setterPort,
		ExpectedReturns: len(
			measurementReady.Getters,
		), // One return per getter port
		ReceivedReturns: 0,
		Results:         make(map[string]interface{}),
		ArmedCount:      0,
	}
	h.mutex.Unlock()

	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Starting buffered measurement for ProcessId %s: %d getter instruments to arm, setter port: %s",
			measurementReady.ProcessId,
			len(uniqueInstruments),
			setterPort,
		),
	)

	// Step 1: Arm all getter instruments
	h.armGetterInstruments(measurementReady.ProcessId, measurementReady.Getters)
}

// getCachedPortConfiguration gets a port configuration with local caching
func (h *MeasurementReadyHandler) getCachedPortConfiguration(
	port string,
) (*instrument.PortConfiguration, error) {
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
	portConfig, err := h.instrumentHandler.GetPortConfiguration(port)
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
			"Successfully retrieved port configuration for %s: instrument=%s, index=%d, properties=%v",
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

// invalidatePortConfigCache clears the local port configuration cache
func (h *MeasurementReadyHandler) invalidatePortConfigCache() {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	h.portConfigCache = make(map[string]*instrument.PortConfiguration)
}

// sendGetCommand sends a GET command for a specific port
func (h *MeasurementReadyHandler) sendGetCommand(port, processId string) error {
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
	getId := fmt.Sprintf("%s_%s_%d", processId, port, time.Now().UnixNano())

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
	getCommand := api.Get{
		Index:    portConfig.Index,
		Property: portConfig.Properties[0], // Use first property
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
			"Sent "+GetMessage+" command for port %s to %s (Property: %s, Index: %d, GetId: %s)",
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
		if pendingGet.Index == returnGet.Index &&
			pendingGet.Property == returnGet.Property {
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
		h.getResults[matchingGet.ProcessId] = make(map[string]interface{})
	}
	h.getResults[matchingGet.ProcessId][matchingGet.Port] = returnGet.Value

	// Remove the pending GET
	delete(h.pendingGets, matchingGetId)

	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Stored result for port %s, ProcessId %s: %v",
			matchingGet.Port,
			matchingGet.ProcessId,
			returnGet.Value,
		),
	)

	// Check if we have all results for this process
	h.checkAndSendProcessData(matchingGet.ProcessId)
}

// getUniqueInstruments extracts unique instrument names from a list of ports
func (h *MeasurementReadyHandler) getUniqueInstruments(
	ports []string,
) []string {
	instrumentSet := make(map[string]bool)
	var uniqueInstruments []string

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
	processId string,
	getterPorts []string,
) {
	// Get the first port for each unique instrument to use for trigger
	// configuration
	instrumentFirstPort := make(map[string]string)

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
		if _, exists := instrumentFirstPort[portConfig.Instrument]; !exists {
			instrumentFirstPort[portConfig.Instrument] = port
		}
	}

	// Send TRIGGER command for each unique instrument
	for instrumentName, firstPort := range instrumentFirstPort {
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
	instrumentName, port, processId string,
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
	triggerCommand := api.Trigger{
		Property: portConfig.Properties[0], // Use first property
		Index:    portConfig.Index,
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
			"Sent "+TriggerMessage+" command to %s instrument %s (Property: %s, Index: %d, ProcessId: %s)",
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
func (h *MeasurementReadyHandler) incrementArmedCount(processId string) {
	h.mutex.Lock()

	var shouldTriggerSetter bool
	if pending, exists := h.pendingBufferedMeasurements[processId]; exists {
		pending.ArmedCount++

		h.logger.Debug(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Armed instrument count for ProcessId %s: %d/%d",
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
func (h *MeasurementReadyHandler) triggerSetter(processId string) {
	pending, exists := h.pendingBufferedMeasurements[processId]
	if !exists {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"No pending buffered measurement found for ProcessId %s",
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
			"All getters armed for ProcessId %s, triggering setter on instrument %s",
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
		returnData.Property,
		returnData.Index,
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
		fmt.Sprintf(
			"About to acquire mutex lock to search for pending buffered measurements",
		),
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

	var matchingProcessId string
	for processId, pending := range h.pendingBufferedMeasurements {
		h.logger.Debug(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Checking ProcessId %s: GetterPorts=%v",
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
					"Found matching ProcessId %s for port %s",
					processId,
					port,
				),
			)
			break
		} else {
			h.logger.Debug(
				MeasurementReadyHandlerName,
				fmt.Sprintf("Port %s not found in getters for ProcessId %s", port, processId),
			)
		}
	}

	if matchingProcessId == "" {
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
			"Processing RETURN_DATA for matching ProcessId %s",
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
			"Stored buffered result for port %s, ProcessId %s (%d/%d received): %v",
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
	property string,
	index int64,
) (string, error) {
	h.logger.Debug(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Looking for port with property=%s, index=%d",
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
	for portName, configValue := range portConfigurations {
		h.logger.Debug(
			MeasurementReadyHandlerName,
			fmt.Sprintf("Checking port %s: %+v", portName, configValue),
		)

		if portConfig, ok := configValue.(instrument.PortConfiguration); ok {
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
	}

	return "", fmt.Errorf(
		"no port found for property %s, index %d",
		property,
		index,
	)
}

// portInGetters checks if a port was part of the getters for a specific
// measurement
func (h *MeasurementReadyHandler) portInGetters(
	port string,
	getterPorts []string,
) bool {
	for _, getterPort := range getterPorts {
		if getterPort == port {
			return true
		}
	}
	return false
}

// sendProcessDataForBuffered sends the collected buffered data as PROCESS_DATA
func (h *MeasurementReadyHandler) sendProcessDataForBuffered(processId string) {
	pending, exists := h.pendingBufferedMeasurements[processId]
	if !exists {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"No pending buffered measurement found for ProcessId %s",
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
				"Failed to marshal buffered results for ProcessId %s: %v",
				processId,
				err,
			),
		)
		return
	}

	// Create PROCESS_DATA message (same as unbuffered)
	processData := api.ProcessData{
		Data:      string(dataBytes),
		ProcessId: processId,
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal the PROCESS_DATA
	processDataBytes, err := json.Marshal(processData)
	if err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to marshal "+ProcessDataMessage+" for ProcessId %s: %v",
				processId,
				err,
			),
		)
		return
	}

	// Send PROCESS_DATA (not UPLOAD_DATA)
	if err := h.nc.Publish(ProcessDataSubject, processDataBytes); err != nil {
		h.logger.Error(
			MeasurementReadyHandlerName,
			fmt.Sprintf(
				"Failed to publish "+ProcessDataMessage+" for ProcessId %s: %v",
				processId,
				err,
			),
		)
		return
	}

	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Sent "+ProcessDataMessage+" for buffered measurement ProcessId %s with %d results",
			processId,
			len(pending.Results),
		),
	)

	// Clean up the pending measurement
	delete(h.pendingBufferedMeasurements, processId)
}

// checkAndSendProcessData checks if all GET results are collected and sends
// PROCESS_DATA
func (h *MeasurementReadyHandler) checkAndSendProcessData(processId string) {
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
					"Failed to send "+ProcessDataMessage+" for ProcessId %s: %v",
					processId,
					err,
				),
			)
		}
	}
}

// sendProcessData sends the collected data as PROCESS_DATA
func (h *MeasurementReadyHandler) sendProcessData(processId string) error {
	results, exists := h.getResults[processId]
	if !exists {
		return fmt.Errorf("no results found for ProcessId %s", processId)
	}

	// Marshal the results to JSON string
	dataBytes, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	// Create PROCESS_DATA message
	processData := api.ProcessData{
		Data:      string(dataBytes),
		ProcessId: processId,
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal the PROCESS_DATA
	processDataBytes, err := json.Marshal(processData)
	if err != nil {
		return fmt.Errorf("failed to marshal "+ProcessDataMessage+": %w", err)
	}

	// Send PROCESS_DATA
	if err := h.nc.Publish(ProcessDataSubject, processDataBytes); err != nil {
		return fmt.Errorf("failed to publish "+ProcessDataMessage+": %w", err)
	}

	h.logger.Info(
		MeasurementReadyHandlerName,
		fmt.Sprintf(
			"Sent "+ProcessDataMessage+" for ProcessId %s with %d measurements",
			processId,
			len(results),
		),
	)

	// Clean up results for this process
	delete(h.getResults, processId)

	return nil
}
