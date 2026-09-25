package handlers

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/serverinterpreter"
)

const (
	MeasureCommandHandlerName = "MEASURE_COMMAND_HANDLER"
	// INSTRUMENTHUB.MEASURE_COMMAND is the subject published by falcon-comms
	// RoutineComms on the controller side (routine_comms.cpp make_measure_command_subject).
	MeasureCommandSubject = "INSTRUMENTHUB.MEASURE_COMMAND"
	// FALCON.MEASURE_RESPONSE.<timestamp> is subscribed to by falcon-comms
	// RoutineComms on the controller side.
	MeasureResponseSubject = "FALCON.MEASURE_RESPONSE"
	MeasureCommandName     = "MEASURE_COMMAND"
	MeasureResponseName    = "MEASURE_RESPONSE"
)

type MeasurementClient interface {
	Measure(
		scriptPath string,
		globals map[string]interface{},
		typeManifest map[string]interface{},
	) ([]serverinterpreter.ISSCallResult, error)

	ReadBuffer(bufferID string) ([]float64, error)
}

// ScriptDispatcher executes user-provided Lua measurement scripts in ISS.
type ScriptDispatcher struct {
	client      MeasurementClient
	scriptsPath string
}

func NewScriptDispatcher(
	client MeasurementClient,
	scriptsPath string,
) *ScriptDispatcher {
	return &ScriptDispatcher{
		client:      client,
		scriptsPath: scriptsPath,
	}
}

// FIX: This shouldn't deviate from the real implementation since this should be it
// ResolvedCallResult extends ISSCallResult with buffer data resolved inline.
type ResolvedCallResult struct {
	serverinterpreter.ISSCallResult
	BufferData []float64 // populated when Return.Type == "buffer"
}

// RunMeasurement calls ISS measure (sync), resolves all buffer results, returns
// the full call list with buffer data populated.
// typeManifest, if non-nil, tells ISS to call main with positional arguments
// (required for Teal-compiled scripts with named parameters).
func (d *ScriptDispatcher) RunMeasurement(scriptName string, globals map[string]interface{}, typeManifest map[string]interface{}) ([]ResolvedCallResult, error) {
	scriptPath := filepath.Join(d.scriptsPath, scriptName+".lua")
	results, err := d.client.Measure(scriptPath, globals, typeManifest)
	if err != nil {
		return nil, fmt.Errorf("measure script %s: %w", scriptName, err)
	}

	resolved := make([]ResolvedCallResult, len(results))
	for i, r := range results {
		resolved[i] = ResolvedCallResult{ISSCallResult: r}
		if r.Return.Type == "buffer" && r.Return.BufferID != "" {
			data, err := d.client.ReadBuffer(r.Return.BufferID)
			if err != nil {
				return nil, fmt.Errorf("read_buffer %s: %w", r.Return.BufferID, err)
			}
			resolved[i].BufferData = data
		}
	}
	return resolved, nil
}

// BusyManager interface allows the handler to manage busy state
type BusyManager interface {
	SetIsBusy(busy bool)
}

// MeasurementDispatcher dispatches measurement scripts to the instrument-script-server.
type MeasurementDispatcher interface {
	RunMeasurement(scriptName string, globals map[string]interface{}, typeManifest map[string]interface{}) ([]ResolvedCallResult, error)
}

type scriptPortRequirement struct {
	capability string
	role       string
}

type scriptTarget struct {
	id            string
	channel       int
	connectedPort *ports.ConnectedPort
}

func (h *MeasureCommandHandler) resolveScriptTarget(
	scriptName string,
	targetKind string,
	info serverinterpreter.ExtractedInstrumentInfo,
) (scriptTarget, error) {
	gateName, err := gateNameFromConnectionJSON(info.ConnectionJSON)
	if err != nil {
		return scriptTarget{}, fmt.Errorf("failed to get %s gate name: %w", targetKind, err)
	}

	return h.resolveScriptTargetForGate(scriptName, targetKind, gateName)
}

func (h *MeasureCommandHandler) resolveScriptTargetForGate(
	scriptName string,
	targetKind string,
	gateName string,
) (scriptTarget, error) {
	metadata := h.measurementMetadata
	if metadata.Measurements == nil {
		metadata = defaultMeasurementMetadataRegistry()
	}
	if req, ok := metadata.requirement(scriptName, targetKind); ok {
		connectedPort, err := h.ports.ResolveConnectedPort(gateName, req.capability, req.role)
		if err != nil {
			return scriptTarget{}, fmt.Errorf(
				"failed to resolve %s %q capability %q role %q for gate %q: %w",
				targetKind,
				scriptName,
				req.capability,
				req.role,
				gateName,
				err,
			)
		}
		cp := connectedPort
		return scriptTarget{
			id:            connectedPort.InstrumentName,
			channel:       connectedPort.ChannelIndex,
			connectedPort: &cp,
		}, nil
	}
	return scriptTarget{}, fmt.Errorf("%s gate %q not found in wiremap", targetKind, gateName)
}

func measurementResponseSubject(timestamp int64) string {
	return MeasureResponseSubject + "." + strconv.FormatInt(timestamp, 10)
}

func resolvedCallResultToFloatSlice(result ResolvedCallResult) []float64 {
	switch result.Return.Type {
	case "buffer":
		return append([]float64{}, result.BufferData...)
	case "float", "double", "number":
		if v, ok := result.Return.Value.(float64); ok {
			return []float64{v}
		}
	case "integer", "int":
		switch v := result.Return.Value.(type) {
		case float64:
			return []float64{v}
		case int:
			return []float64{float64(v)}
		}
	case "boolean":
		if v, ok := result.Return.Value.(bool); ok {
			if v {
				return []float64{1.0}
			}
			return []float64{0.0}
		}
	}
	return nil
}

// parseWireMapEntry splits a wiremap key of the form
// "InstrumentId.channelGroup.index" (e.g. "Source1.analog.4") into
// the instrument ID ("Source1") and channel index (4).
func parseWireMapEntry(entry config.InstrumentConnection) (instrumentID string, channelIndex int, ok bool) {
	parts := strings.Split(string(entry), ".")
	// Need at least 3 parts: id . group . index
	if len(parts) < 3 {
		return "", 0, false
	}
	idx, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return "", 0, false
	}
	// Skip the channel group name (second-to-last part)
	return strings.Join(parts[:len(parts)-2], "."), idx, true
}

func responseTargetFromResolvedPort(
	bufferData []float64,
	target scriptTarget,
	connectionJSON string,
	fallback serverinterpreter.ExtractedInstrumentInfo,
) measurementResponseTarget {
	responseTarget := measurementResponseTarget{
		BufferData:     bufferData,
		PortJSON:       fallback.PortJSON,
		ConnectionJSON: connectionJSON,
		InstrumentType: fallback.InstrumentType,
		UnitsJSON:      fallback.UnitsJSON,
	}
	if target.connectedPort != nil {
		responseTarget.PortJSON = ""
		responseTarget.ConnectedPort = target.connectedPort
	}
	return responseTarget
}

// MeasureCommandHandler handles MEASURE_COMMAND requests
type MeasureCommandHandler struct {
	logger              *logging.Logger
	nc                  *nats.Conn
	js                  nats.JetStreamContext
	subscription        *nats.Subscription
	busyManager         BusyManager
	dispatcher          MeasurementDispatcher
	wiremap             *config.WireMap
	stateMu             sync.Mutex
	voltages            map[string]float64
	sampleRates         map[string]float64
	numberOfSamples     map[string]int
	slopes              map[string]float64
	triggerLevels       map[string]float64
	measurementMetadata measurementMetadataRegistry
	ports               *ports.ConnectedPorts
}

// NewMeasureCommandHandler creates a new handler
func NewMeasureCommandHandler(
	logger *logging.Logger,
	busyManager BusyManager,
	dispatcher MeasurementDispatcher,
	wireMap *config.WireMap,
	measurementMetadata measurementMetadataRegistry,
	ports *ports.ConnectedPorts,
) *MeasureCommandHandler {
	if measurementMetadata.Measurements == nil {
		measurementMetadata = defaultMeasurementMetadataRegistry()
	}
	return &MeasureCommandHandler{
		logger:              logger,
		busyManager:         busyManager,
		dispatcher:          dispatcher,
		wiremap:             wireMap,
		voltages:            map[string]float64{},
		sampleRates:         map[string]float64{},
		numberOfSamples:     map[string]int{},
		slopes:              map[string]float64{},
		triggerLevels:       map[string]float64{},
		measurementMetadata: measurementMetadata,
	}
}

// Subscribe starts listening for MEASURE_COMMAND requests
func (h *MeasureCommandHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error

	h.js, err = nc.JetStream()
	if err != nil {
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}
	_, addErr := h.js.AddStream(&nats.StreamConfig{
		Name:     "FALCON_MEASURE",
		Subjects: []string{"FALCON.MEASURE_DATA.*"},
		MaxAge:   60 * time.Second,
	})
	if addErr != nil && addErr != nats.ErrStreamNameAlreadyInUse {
		return fmt.Errorf("failed to ensure FALCON_MEASURE stream: %w", addErr)
	}

	h.subscription, err = nc.Subscribe(
		MeasureCommandSubject,
		h.handleMessage,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+MeasureCommandSubject+": %w",
			err,
		)
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		"Subscribed to "+MeasureCommandSubject,
	)
	return nil
}

// Unsubscribe stops listening for commands
func (h *MeasureCommandHandler) Unsubscribe() error {
	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
			return fmt.Errorf("failed to unsubscribe: %w", err)
		}
		h.subscription = nil
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		"Unsubscribed from "+MeasureCommandSubject,
	)
	return nil
}

func (h *MeasureCommandHandler) publishMeasurementResponse(cmd api.MeasureCommand, responseSubject, respJSON string) bool {
	measureSubject := "FALCON.MEASURE_DATA." + strconv.FormatInt(cmd.Timestamp, 10)
	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Publishing measurement data: subject=%s bytes=%d", measureSubject, len(respJSON)))
	if _, err := h.js.Publish(measureSubject, []byte(respJSON)); err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to publish measurement to JetStream subject %s: %v", measureSubject, err))
		return false
	}
	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Published measurement data: subject=%s", measureSubject))

	measureResp := api.MeasureResponse{
		Stream:    measureSubject,
		Response:  respJSON,
		Timestamp: cmd.Timestamp,
		Hash:      cmd.Hash,
	}
	respData, err := json.Marshal(measureResp)
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to marshal MeasureResponse: %v", err))
		return false
	}

	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Publishing %s: subject=%s stream=%s bytes=%d", MeasureResponseName, responseSubject, measureSubject, len(respData)))
	if err := h.nc.Publish(responseSubject, respData); err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to publish %s: %v", responseSubject, err))
		return false
	}
	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Published %s: subject=%s stream=%s", MeasureResponseName, responseSubject, measureSubject))
	return true
}

// FIX: Uncomment and implement
// handleMessage processes an INSTRUMENTHUB.MEASURE_COMMAND message, dispatches
// the measurement script to ISS, and publishes a timestamp-scoped response.
func (h *MeasureCommandHandler) handleMessage(msg *nats.Msg) {
	// h.logger.Debug(
	// 	MeasureCommandHandlerName,
	// 	fmt.Sprintf("Received command: %s", string(msg.Data)),
	// )
	//
	// var cmd api.MeasureCommand
	// if err := json.Unmarshal(msg.Data, &cmd); err != nil {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("failed to unmarshal MEASURE_COMMAND: %v", err))
	// 	return
	// }
	//
	// if cmd.Request == "" {
	// 	h.logger.Debug(MeasureCommandHandlerName, "empty request, ignoring")
	// 	return
	// }
	//
	// responseSubject := measurementResponseSubject(cmd.Timestamp)
	//
	// h.busyManager.SetIsBusy(true)
	// defer h.busyManager.SetIsBusy(false)
	//
	// falconReq, err := serverinterpreter.NewFalconMeasurementRequestFromJSON(cmd.Request)
	// if err != nil {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("failed to parse MeasurementRequest: %v", err))
	// 	return
	// }
	// defer falconReq.Close()
	//
	// scriptName, scriptNameErr := falconReq.MeasurementName()
	// if scriptNameErr != nil {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("failed to read measurement_name: %v", scriptNameErr))
	// 	return
	// }
	// scriptName = strings.TrimSpace(scriptName)
	// if scriptName == "" {
	// 	h.logger.Error(MeasureCommandHandlerName, "measurement_name is required")
	// 	return
	// }
	//
	// if scriptName == "get_many_voltages" || scriptName == "get_all_voltages" {
	// 	getters, err := falconReq.ExtractGetters()
	// 	if err != nil || len(getters) == 0 {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to extract getters (got %d): %v", len(getters), err))
	// 		return
	// 	}
	//
	// 	getterTargets := make([]map[string]interface{}, 0, len(getters))
	// 	responseTargets := make([]measurementResponseTarget, 0, len(getters))
	// 	cachedVoltages := make([][]float64, len(getters))
	//
	// 	for i, getter := range getters {
	// 		getterTarget, err := h.resolveScriptTarget(scriptName, "getter", getter)
	// 		if err != nil {
	// 			h.logger.Error(MeasureCommandHandlerName,
	// 				fmt.Sprintf("failed to resolve getter target at index %d: %v", i, err))
	// 			return
	// 		}
	//
	// 		getterTargets = append(getterTargets, getterTarget.asMap())
	// 		responseTargets = append(responseTargets,
	// 			responseTargetFromResolvedPort(nil, getterTarget, getter.ConnectionJSON, getter))
	//
	// 		h.stateMu.Lock()
	// 		if voltage, ok := h.voltages[getterTarget.stateKey()]; ok {
	// 			cachedVoltages[i] = []float64{voltage}
	// 		}
	// 		h.stateMu.Unlock()
	// 	}
	//
	// 	globals := map[string]interface{}{
	// 		"getters": getterTargets,
	// 	}
	// 	typeManifest := map[string]interface{}{
	// 		"parameters": []map[string]interface{}{
	// 			{"name": "ctx", "type": "RuntimeContext"},
	// 			{"name": "getters", "type": "{InstrumentTarget}"},
	// 		},
	// 	}
	//
	// 	results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
	// 	h.logger.Info(MeasureCommandHandlerName,
	// 		fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("measurement dispatch failed: %v", err))
	// 		return
	// 	}
	//
	// 	for i := range responseTargets {
	// 		if i < len(results) {
	// 			responseTargets[i].BufferData = resolvedCallResultToFloatSlice(results[i])
	// 		}
	// 		if len(responseTargets[i].BufferData) == 0 && len(cachedVoltages[i]) > 0 {
	// 			responseTargets[i].BufferData = cachedVoltages[i]
	// 		}
	// 		if len(responseTargets[i].BufferData) == 0 {
	// 			h.logger.Error(MeasureCommandHandlerName,
	// 				fmt.Sprintf("no response value available for getter index %d in %s", i, scriptName))
	// 			return
	// 		}
	// 	}
	//
	// 	respJSON, err := buildMeasurementResponseJSONForTargets(responseTargets, cmd.Hash)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to build MeasurementResponse: %v", err))
	// 		return
	// 	}
	//
	// 	h.publishMeasurementResponse(cmd, responseSubject, respJSON)
	// 	return
	// }
	//
	// if scriptName == "measure_current" || scriptName == "measure_illumination" {
	// 	getters, err := falconReq.ExtractGetters()
	// 	if err != nil || len(getters) == 0 {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to extract getters (got %d): %v", len(getters), err))
	// 		return
	// 	}
	//
	// 	getterTargets := make([]map[string]interface{}, 0, len(getters))
	// 	responseTargets := make([]measurementResponseTarget, 0, len(getters))
	// 	for i, getter := range getters {
	// 		getterTarget, err := h.resolveScriptTarget(scriptName, "getter", getter)
	// 		if err != nil {
	// 			h.logger.Error(MeasureCommandHandlerName,
	// 				fmt.Sprintf("failed to resolve getter target at index %d: %v", i, err))
	// 			return
	// 		}
	//
	// 		getterTargets = append(getterTargets, getterTarget.asMap())
	// 		responseTargets = append(responseTargets,
	// 			responseTargetFromResolvedPort(nil, getterTarget, getter.ConnectionJSON, getter))
	// 	}
	//
	// 	globals := map[string]interface{}{
	// 		"sampleRate": 1000,
	// 		"getters":    getterTargets,
	// 	}
	// 	parameters := []map[string]interface{}{
	// 		{"name": "ctx", "type": "RuntimeContext"},
	// 		{"name": "sampleRate", "type": "number"},
	// 		{"name": "getters", "type": "{InstrumentTarget}"},
	// 	}
	// 	if scriptName == "measure_illumination" {
	// 		globals["illuminationTime"] = 0.1
	// 		parameters = append(parameters, map[string]interface{}{
	// 			"name": "illuminationTime",
	// 			"type": "number",
	// 		})
	// 	}
	//
	// 	typeManifest := map[string]interface{}{"parameters": parameters}
	// 	results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
	// 	h.logger.Info(MeasureCommandHandlerName,
	// 		fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("measurement dispatch failed: %v", err))
	// 		return
	// 	}
	//
	// 	datapointResults := make([][]float64, 0, len(responseTargets))
	// 	for _, result := range results {
	// 		if strings.EqualFold(result.Verb, "GET_DATAPOINT") {
	// 			if data := resolvedCallResultToFloatSlice(result); len(data) > 0 {
	// 				datapointResults = append(datapointResults, data)
	// 			}
	// 		}
	// 	}
	// 	if len(datapointResults) == 0 {
	// 		for _, result := range results {
	// 			if data := resolvedCallResultToFloatSlice(result); len(data) > 0 {
	// 				datapointResults = append(datapointResults, data)
	// 			}
	// 		}
	// 	}
	//
	// 	for i := range responseTargets {
	// 		if i < len(datapointResults) {
	// 			responseTargets[i].BufferData = datapointResults[i]
	// 		}
	// 		if len(responseTargets[i].BufferData) == 0 {
	// 			h.logger.Error(MeasureCommandHandlerName,
	// 				fmt.Sprintf("no scalar response returned for getter index %d in %s", i, scriptName))
	// 			return
	// 		}
	// 	}
	//
	// 	respJSON, err := buildMeasurementResponseJSONForTargets(responseTargets, cmd.Hash)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to build MeasurementResponse: %v", err))
	// 		return
	// 	}
	//
	// 	h.publishMeasurementResponse(cmd, responseSubject, respJSON)
	// 	return
	// }
	//
	// if scriptName == "get_voltage" || scriptName == "get_sample_rate" ||
	// 	scriptName == "get_number_of_samples" || scriptName == "get_slope" ||
	// 	scriptName == "get_trigger_level" || scriptName == "get_trigger_leader" {
	// 	getters, err := falconReq.ExtractGetters()
	// 	if err != nil || len(getters) == 0 {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to extract getters (got %d): %v", len(getters), err))
	// 		return
	// 	}
	// 	h.logger.Debug(MeasureCommandHandlerName,
	// 		fmt.Sprintf(
	// 			"Resolved measurement name: %q (getter default=%q instrument-facing=%q)",
	// 			scriptName,
	// 			getters[0].DefaultName,
	// 			getters[0].InstrumentFacingName,
	// 		))
	//
	// 	getterTarget, err := h.resolveScriptTarget(scriptName, "getter", getters[0])
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to resolve getter target: %v", err))
	// 		return
	// 	}
	//
	// 	globals := map[string]interface{}{
	// 		"getter": getterTarget.asMap(),
	// 	}
	// 	parameters := []map[string]interface{}{
	// 		{"name": "ctx", "type": "RuntimeContext"},
	// 		{"name": "getter", "type": "InstrumentTarget"},
	// 	}
	//
	// 	stateKey := getterTarget.stateKey()
	// 	h.stateMu.Lock()
	// 	voltage, hasVoltage := h.voltages[stateKey]
	// 	sampleRate, hasSampleRate := h.sampleRates[stateKey]
	// 	numberOfSamples, hasNumberOfSamples := h.numberOfSamples[stateKey]
	// 	slope, _ := h.slopes[stateKey]
	// 	triggerLevel, hasTriggerLevel := h.triggerLevels[stateKey]
	// 	h.stateMu.Unlock()
	//
	// 	switch scriptName {
	// 	case "get_sample_rate":
	// 		globals["sampleRate"] = sampleRate
	// 		parameters = append(parameters, map[string]interface{}{
	// 			"name": "sampleRate",
	// 			"type": "number",
	// 		})
	// 	case "get_number_of_samples":
	// 		globals["numberOfSamples"] = numberOfSamples
	// 		parameters = append(parameters, map[string]interface{}{
	// 			"name": "numberOfSamples",
	// 			"type": "number",
	// 		})
	// 	case "get_slope":
	// 		globals["slope"] = slope
	// 		parameters = append(parameters, map[string]interface{}{
	// 			"name": "slope",
	// 			"type": "number",
	// 		})
	// 	case "get_trigger_level":
	// 		globals["triggerLevel"] = triggerLevel
	// 		parameters = append(parameters, map[string]interface{}{
	// 			"name": "triggerLevel",
	// 			"type": "number",
	// 		})
	// 	case "get_trigger_leader":
	// 		globals["triggerLeader"] = triggerLevel != 0
	// 		parameters = append(parameters, map[string]interface{}{
	// 			"name": "triggerLeader",
	// 			"type": "boolean",
	// 		})
	// 	}
	//
	// 	typeManifest := map[string]interface{}{"parameters": parameters}
	// 	results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
	// 	h.logger.Info(MeasureCommandHandlerName,
	// 		fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("measurement dispatch failed: %v", err))
	// 		return
	// 	}
	//
	// 	var bufferData []float64
	// 	for _, r := range results {
	// 		switch r.Return.Type {
	// 		case "buffer":
	// 			bufferData = append(bufferData, r.BufferData...)
	// 		case "float", "double", "number":
	// 			if v, ok := r.Return.Value.(float64); ok {
	// 				bufferData = append(bufferData, v)
	// 			}
	// 		case "integer", "int":
	// 			switch v := r.Return.Value.(type) {
	// 			case float64:
	// 				bufferData = append(bufferData, v)
	// 			case int:
	// 				bufferData = append(bufferData, float64(v))
	// 			}
	// 		case "boolean":
	// 			if v, ok := r.Return.Value.(bool); ok {
	// 				if v {
	// 					bufferData = append(bufferData, 1.0)
	// 				} else {
	// 					bufferData = append(bufferData, 0.0)
	// 				}
	// 			}
	// 		}
	// 	}
	// 	if len(bufferData) == 0 {
	// 		switch scriptName {
	// 		case "get_voltage":
	// 			if hasVoltage {
	// 				bufferData = []float64{voltage}
	// 			}
	// 		case "get_sample_rate":
	// 			if hasSampleRate {
	// 				bufferData = []float64{sampleRate}
	// 			}
	// 		case "get_number_of_samples":
	// 			if hasNumberOfSamples {
	// 				bufferData = []float64{float64(numberOfSamples)}
	// 			}
	// 		case "get_trigger_leader":
	// 			if hasTriggerLevel {
	// 				bufferData = []float64{triggerLevel}
	// 			}
	// 		}
	// 		if len(bufferData) > 0 {
	// 			h.logger.Info(MeasureCommandHandlerName,
	// 				fmt.Sprintf("No explicit getter result returned for %s; using cached state fallback", scriptName))
	// 		}
	// 	}
	//
	// 	respJSON, err := buildMeasurementResponseJSONForTargets(
	// 		[]measurementResponseTarget{
	// 			responseTargetFromResolvedPort(bufferData, getterTarget, getters[0].ConnectionJSON, getters[0]),
	// 		},
	// 		cmd.Hash,
	// 	)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to build MeasurementResponse: %v", err))
	// 		return
	// 	}
	//
	// 	h.publishMeasurementResponse(cmd, responseSubject, respJSON)
	// 	return
	// }
	//
	// setters, err := falconReq.ExtractSetters()
	// if err != nil || len(setters) == 0 {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("failed to extract setters (got %d): %v", len(setters), err))
	// 	return
	// }
	// h.logger.Debug(MeasureCommandHandlerName,
	// 	fmt.Sprintf(
	// 		"Resolved measurement name: %q (setter default=%q instrument-facing=%q)",
	// 		scriptName,
	// 		setters[0].DefaultName,
	// 		setters[0].InstrumentFacingName,
	// 	))
	//
	// if scriptName == "set_voltage" || scriptName == "set_sample_rate" ||
	// 	scriptName == "set_number_of_samples" || scriptName == "set_slope" ||
	// 	scriptName == "set_trigger_level" || scriptName == "set_trigger_leader" {
	// 	waveformData, err := serverinterpreter.ExtractWaveformDataFromRequestByIndex(falconReq, 0)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to extract %s waveform data: %v", scriptName, err))
	// 		return
	// 	}
	//
	// 	scalarValue := waveformData.TimeDomain.Min
	// 	if len(waveformData.RawTimeTrace) > 0 && len(waveformData.RawTimeTrace[0]) > 0 {
	// 		scalarValue = waveformData.RawTimeTrace[0][0]
	// 	}
	//
	// 	targetName := "setter"
	// 	valueName := "setVoltage"
	// 	responseValue := scalarValue
	// 	globals := map[string]interface{}{}
	// 	includeValue := true
	// 	valueType := "number"
	//
	// 	switch scriptName {
	// 	case "set_voltage":
	// 		targetName = "setter"
	// 		valueName = "setVoltage"
	// 		responseValue = scalarValue
	// 	case "set_sample_rate":
	// 		targetName = "getter"
	// 		valueName = "sampleRate"
	// 		responseValue = scalarValue
	// 	case "set_number_of_samples":
	// 		targetName = "getter"
	// 		valueName = "numberOfSamples"
	// 		responseValue = float64(int(scalarValue))
	// 	case "set_slope":
	// 		targetName = "setter"
	// 		valueName = "slope"
	// 		responseValue = scalarValue
	// 	case "set_trigger_level":
	// 		targetName = "getter"
	// 		valueName = "triggerLevel"
	// 		responseValue = scalarValue
	// 	case "set_trigger_leader":
	// 		targetName = "getter"
	// 		valueName = ""
	// 		responseValue = 1.0
	// 		includeValue = false
	// 		valueType = ""
	// 	}
	//
	// 	target, err := h.resolveScriptTarget(scriptName, targetName, setters[0])
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to resolve %s target: %v", targetName, err))
	// 		return
	// 	}
	// 	targetValue := target.asMap()
	//
	// 	globals[targetName] = targetValue
	// 	if includeValue {
	// 		if scriptName == "set_number_of_samples" {
	// 			globals[valueName] = int(responseValue)
	// 		} else {
	// 			globals[valueName] = responseValue
	// 		}
	// 	}
	//
	// 	parameters := []map[string]interface{}{
	// 		{"name": "ctx", "type": "RuntimeContext"},
	// 		{"name": targetName, "type": "InstrumentTarget"},
	// 	}
	// 	if includeValue {
	// 		parameters = append(parameters, map[string]interface{}{
	// 			"name": valueName,
	// 			"type": valueType,
	// 		})
	// 	}
	// 	typeManifest := map[string]interface{}{"parameters": parameters}
	//
	// 	results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
	// 	h.logger.Info(MeasureCommandHandlerName,
	// 		fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("measurement dispatch failed: %v", err))
	// 		return
	// 	}
	//
	// 	stateKey := target.stateKey()
	// 	h.stateMu.Lock()
	// 	switch scriptName {
	// 	case "set_voltage":
	// 		h.voltages[stateKey] = responseValue
	// 	case "set_sample_rate":
	// 		h.sampleRates[stateKey] = responseValue
	// 	case "set_number_of_samples":
	// 		h.numberOfSamples[stateKey] = int(responseValue)
	// 	case "set_slope":
	// 		h.slopes[stateKey] = responseValue
	// 	case "set_trigger_level":
	// 		h.triggerLevels[stateKey] = responseValue
	// 	case "set_trigger_leader":
	// 		h.triggerLevels[stateKey] = 1.0
	// 	}
	// 	h.stateMu.Unlock()
	//
	// 	bufferData := []float64{responseValue}
	// 	respJSON, err := buildMeasurementResponseJSONForTargets(
	// 		[]measurementResponseTarget{
	// 			responseTargetFromResolvedPort(bufferData, target, setters[0].ConnectionJSON, setters[0]),
	// 		},
	// 		cmd.Hash,
	// 	)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to build MeasurementResponse: %v", err))
	// 		return
	// 	}
	//
	// 	h.publishMeasurementResponse(cmd, responseSubject, respJSON)
	// 	return
	// }
	//
	// if scriptName == "set_many_voltages" || scriptName == "ramp" {
	// 	setterTargets := make([]map[string]interface{}, 0, len(setters))
	// 	setVoltages := make(map[string]float64, len(setters))
	// 	responseTargets := make([]measurementResponseTarget, 0, len(setters))
	//
	// 	for i, setter := range setters {
	// 		setterTarget, err := h.resolveScriptTarget(scriptName, "setter", setter)
	// 		if err != nil {
	// 			h.logger.Error(MeasureCommandHandlerName,
	// 				fmt.Sprintf("failed to resolve setter target at index %d: %v", i, err))
	// 			return
	// 		}
	//
	// 		waveformData, err := serverinterpreter.ExtractWaveformDataFromRequestByIndex(falconReq, i)
	// 		if err != nil {
	// 			h.logger.Error(MeasureCommandHandlerName,
	// 				fmt.Sprintf("failed to extract %s waveform data at index %d: %v", scriptName, i, err))
	// 			return
	// 		}
	//
	// 		scalarValue := waveformData.TimeDomain.Min
	// 		if len(waveformData.RawTimeTrace) > 0 && len(waveformData.RawTimeTrace[0]) > 0 {
	// 			scalarValue = waveformData.RawTimeTrace[0][0]
	// 		}
	//
	// 		setterTargets = append(setterTargets, setterTarget.asMap())
	// 		setVoltages[setterTarget.stateKey()] = scalarValue
	// 		h.stateMu.Lock()
	// 		h.voltages[setterTarget.stateKey()] = scalarValue
	// 		h.stateMu.Unlock()
	// 		responseTargets = append(responseTargets,
	// 			responseTargetFromResolvedPort([]float64{scalarValue}, setterTarget, setter.ConnectionJSON, setter))
	// 	}
	//
	// 	globals := map[string]interface{}{
	// 		"setters":     setterTargets,
	// 		"setVoltages": setVoltages,
	// 	}
	// 	typeManifest := map[string]interface{}{
	// 		"parameters": []map[string]interface{}{
	// 			{"name": "ctx", "type": "RuntimeContext"},
	// 			{"name": "setters", "type": "{InstrumentTarget}"},
	// 			{"name": "setVoltages", "type": "{string: number}"},
	// 		},
	// 	}
	//
	// 	results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
	// 	h.logger.Info(MeasureCommandHandlerName,
	// 		fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("measurement dispatch failed: %v", err))
	// 		return
	// 	}
	//
	// 	respJSON, err := buildMeasurementResponseJSONForTargets(responseTargets, cmd.Hash)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to build MeasurementResponse: %v", err))
	// 		return
	// 	}
	//
	// 	h.publishMeasurementResponse(cmd, responseSubject, respJSON)
	// 	return
	// }
	//
	// getters, err := falconReq.ExtractGetters()
	// if err != nil || len(getters) == 0 {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("failed to extract getters (got %d): %v", len(getters), err))
	// 	return
	// }
	//
	// getterTarget, err := h.resolveScriptTarget(scriptName, "getter", getters[0], revWire)
	// if err != nil {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("failed to resolve getter target: %v", err))
	// 	return
	// }
	//
	// if scriptName == "measure_leakage" {
	// 	waveformData, _, err := serverinterpreter.ExtractWaveformDataFromRequest(falconReq)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to extract waveform data for measure_leakage: %v", err))
	// 		return
	// 	}
	//
	// 	leakageVoltage := waveformData.TimeDomain.Min
	// 	if len(waveformData.RawTimeTrace) > 0 && len(waveformData.RawTimeTrace[0]) > 0 {
	// 		leakageVoltage = waveformData.RawTimeTrace[0][0]
	// 	}
	//
	// 	globals := map[string]interface{}{
	// 		"getter":  getterTarget.asMap(),
	// 		"voltage": leakageVoltage,
	// 	}
	// 	typeManifest := map[string]interface{}{
	// 		"parameters": []map[string]interface{}{
	// 			{"name": "ctx", "type": "RuntimeContext"},
	// 			{"name": "getter", "type": "InstrumentTarget"},
	// 			{"name": "voltage", "type": "number"},
	// 		},
	// 	}
	//
	// 	results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
	// 	h.logger.Info(MeasureCommandHandlerName,
	// 		fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("measurement dispatch failed: %v", err))
	// 		return
	// 	}
	//
	// 	var bufferData []float64
	// 	for _, r := range results {
	// 		bufferData = append(bufferData, resolvedCallResultToFloatSlice(r)...)
	// 	}
	// 	if len(bufferData) == 0 {
	// 		bufferData = []float64{leakageVoltage}
	// 	}
	//
	// 	respJSON, err := buildMeasurementResponseJSONForTargets(
	// 		[]measurementResponseTarget{
	// 			responseTargetFromResolvedPort(bufferData, getterTarget, getters[0].ConnectionJSON, getters[0]),
	// 		},
	// 		cmd.Hash,
	// 	)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to build MeasurementResponse: %v", err))
	// 		return
	// 	}
	//
	// 	h.publishMeasurementResponse(cmd, responseSubject, respJSON)
	// 	return
	// }
	//
	// setterTarget, err := h.resolveScriptTarget(scriptName, "setter", setters[0], revWire)
	// if err != nil {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("failed to resolve setter target: %v", err))
	// 	return
	// }
	//
	// waveformData, _, err := serverinterpreter.ExtractWaveformDataFromRequest(falconReq)
	// if err != nil {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("failed to extract waveform data: %v", err))
	// 	return
	// }
	//
	// sweepVoltages := make([]interface{}, len(waveformData.RawTimeTrace))
	// for i, row := range waveformData.RawTimeTrace {
	// 	if len(row) > 0 {
	// 		sweepVoltages[i] = row[0]
	// 	} else {
	// 		sweepVoltages[i] = 0.0
	// 	}
	// }
	//
	// var globals map[string]interface{}
	// var typeManifest map[string]interface{}
	// if len(setters) >= 2 {
	// 	// 2D sweep: fast axis = setters[0], slow axis = setters[1]
	// 	slowSetterTarget, err := h.resolveScriptTarget(scriptName, "setter", setters[1], revWire)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to resolve slow setter target: %v", err))
	// 		return
	// 	}
	// 	slowWaveformData, err := serverinterpreter.ExtractWaveformDataFromRequestByIndex(falconReq, 1)
	// 	if err != nil {
	// 		h.logger.Error(MeasureCommandHandlerName,
	// 			fmt.Sprintf("failed to extract slow axis waveform data: %v", err))
	// 		return
	// 	}
	// 	slowSweepVoltages := make([]interface{}, len(slowWaveformData.RawTimeTrace))
	// 	for i, row := range slowWaveformData.RawTimeTrace {
	// 		if len(row) > 0 {
	// 			slowSweepVoltages[i] = row[0]
	// 		} else {
	// 			slowSweepVoltages[i] = 0.0
	// 		}
	// 	}
	// 	if scriptName == "measure_2D_buffered" {
	// 		numXSteps := len(sweepVoltages)
	// 		if numXSteps == 0 {
	// 			numXSteps = 1
	// 		}
	// 		numYSteps := len(slowSweepVoltages)
	// 		if numYSteps == 0 {
	// 			numYSteps = 1
	// 		}
	// 		globals = map[string]interface{}{
	// 			"bufferedXSetters": []map[string]interface{}{
	// 				setterTarget.asMap(),
	// 			},
	// 			"sampleRate": 1000,
	// 			"bufferedGetters": []map[string]interface{}{
	// 				getterTarget.asMap(),
	// 			},
	// 			"bufferedYSetters": []map[string]interface{}{
	// 				slowSetterTarget.asMap(),
	// 			},
	// 			"numXSteps": numXSteps,
	// 			"setYVoltageDomains": map[string]interface{}{
	// 				slowSetterTarget.id: map[string]interface{}{
	// 					"min": slowWaveformData.TimeDomain.Min,
	// 					"max": slowWaveformData.TimeDomain.Max,
	// 				},
	// 			},
	// 			"setXVoltageDomains": map[string]interface{}{
	// 				setterTarget.id: map[string]interface{}{
	// 					"min": waveformData.TimeDomain.Min,
	// 					"max": waveformData.TimeDomain.Max,
	// 				},
	// 			},
	// 			"numPoints": 1,
	// 			"numYSteps": numYSteps,
	// 			"setters":   []map[string]interface{}{},
	// 		}
	// 		typeManifest = map[string]interface{}{
	// 			"parameters": []map[string]interface{}{
	// 				{"name": "ctx", "type": "RuntimeContext"},
	// 				{"name": "bufferedXSetters", "type": "{InstrumentTarget}"},
	// 				{"name": "sampleRate", "type": "number"},
	// 				{"name": "bufferedGetters", "type": "{InstrumentTarget}"},
	// 				{"name": "bufferedYSetters", "type": "{InstrumentTarget}"},
	// 				{"name": "numXSteps", "type": "number"},
	// 				{"name": "setYVoltageDomains", "type": "table"},
	// 				{"name": "setXVoltageDomains", "type": "table"},
	// 				{"name": "numPoints", "type": "number"},
	// 				{"name": "numYSteps", "type": "number"},
	// 				{"name": "setters", "type": "{InstrumentTarget}"},
	// 			},
	// 		}
	// 	} else {
	// 		globals = map[string]interface{}{
	// 			"getters":           []map[string]interface{}{getterTarget.asMap()},
	// 			"fastSweepVoltages": sweepVoltages,
	// 			"slowSweepVoltages": slowSweepVoltages,
	// 			"fastSetter":        setterTarget.asMap(),
	// 			"slowSetter":        slowSetterTarget.asMap(),
	// 		}
	// 		typeManifest = map[string]interface{}{
	// 			"parameters": []map[string]interface{}{
	// 				{"name": "ctx", "type": "RuntimeContext"},
	// 				{"name": "getters", "type": "{InstrumentTarget}"},
	// 				{"name": "fastSweepVoltages", "type": "{number}"},
	// 				{"name": "slowSweepVoltages", "type": "{number}"},
	// 				{"name": "fastSetter", "type": "InstrumentTarget"},
	// 				{"name": "slowSetter", "type": "InstrumentTarget"},
	// 			},
	// 		}
	// 	}
	// } else {
	// 	if scriptName == "measure_get_set" {
	// 		numPoints := len(sweepVoltages)
	// 		if numPoints == 0 {
	// 			numPoints = 1
	// 		}
	// 		sampleRate := 1000
	// 		setVoltage := 0.0
	// 		if len(sweepVoltages) > 0 {
	// 			if v, ok := sweepVoltages[0].(float64); ok {
	// 				setVoltage = v
	// 			}
	// 		}
	// 		globals = map[string]interface{}{
	// 			"getters":    []map[string]interface{}{getterTarget.asMap()},
	// 			"numPoints":  numPoints,
	// 			"sampleRate": sampleRate,
	// 			"setVoltages": map[string]interface{}{
	// 				setterTarget.id: setVoltage,
	// 			},
	// 			"setters": []map[string]interface{}{setterTarget.asMap()},
	// 		}
	// 		typeManifest = map[string]interface{}{
	// 			"parameters": []map[string]interface{}{
	// 				{"name": "ctx", "type": "RuntimeContext"},
	// 				{"name": "getters", "type": "{InstrumentTarget}"},
	// 				{"name": "numPoints", "type": "number"},
	// 				{"name": "sampleRate", "type": "number"},
	// 				{"name": "setVoltages", "type": "{string: number}"},
	// 				{"name": "setters", "type": "{InstrumentTarget}"},
	// 			},
	// 		}
	// 	} else if scriptName == "measure_1D_buffered" {
	// 		numSteps := len(sweepVoltages)
	// 		if numSteps == 0 {
	// 			numSteps = 1
	// 		}
	// 		globals = map[string]interface{}{
	// 			"sampleRate": 1000,
	// 			"setters":    []map[string]interface{}{},
	// 			"setVoltageDomains": map[string]interface{}{
	// 				setterTarget.id: map[string]interface{}{
	// 					"min": waveformData.TimeDomain.Min,
	// 					"max": waveformData.TimeDomain.Max,
	// 				},
	// 			},
	// 			"bufferedGetters": []map[string]interface{}{
	// 				getterTarget.asMap(),
	// 			},
	// 			"numPoints": 1,
	// 			"numSteps":  numSteps,
	// 			"bufferedSetters": []map[string]interface{}{
	// 				setterTarget.asMap(),
	// 			},
	// 		}
	// 		typeManifest = map[string]interface{}{
	// 			"parameters": []map[string]interface{}{
	// 				{"name": "ctx", "type": "RuntimeContext"},
	// 				{"name": "sampleRate", "type": "number"},
	// 				{"name": "setters", "type": "{InstrumentTarget}"},
	// 				{"name": "setVoltageDomains", "type": "table"},
	// 				{"name": "bufferedGetters", "type": "{InstrumentTarget}"},
	// 				{"name": "numPoints", "type": "number"},
	// 				{"name": "numSteps", "type": "number"},
	// 				{"name": "bufferedSetters", "type": "{InstrumentTarget}"},
	// 			},
	// 		}
	// 	} else {
	// 		// 1D sweep
	// 		globals = map[string]interface{}{
	// 			"getters":       []map[string]interface{}{getterTarget.asMap()},
	// 			"setters":       []map[string]interface{}{setterTarget.asMap()},
	// 			"sweepVoltages": sweepVoltages,
	// 		}
	// 		typeManifest = map[string]interface{}{
	// 			"parameters": []map[string]interface{}{
	// 				{"name": "ctx", "type": "RuntimeContext"},
	// 				{"name": "getters", "type": "{InstrumentTarget}"},
	// 				{"name": "sweepVoltages", "type": "{number}"},
	// 				{"name": "setters", "type": "{InstrumentTarget}"},
	// 			},
	// 		}
	// 	}
	// }
	//
	// results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
	// h.logger.Info(MeasureCommandHandlerName,
	// 	fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
	// if err != nil {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("measurement dispatch failed: %v", err))
	// 	return
	// }
	//
	// var bufferData []float64
	// for _, r := range results {
	// 	switch r.Return.Type {
	// 	case "buffer":
	// 		bufferData = append(bufferData, r.BufferData...)
	// 	case "float", "double", "number":
	// 		if v, ok := r.Return.Value.(float64); ok {
	// 			bufferData = append(bufferData, v)
	// 		}
	// 	}
	// }
	// h.logger.Info(MeasureCommandHandlerName,
	// 	fmt.Sprintf("bufferData collected: len=%d", len(bufferData)))
	//
	// h.logger.Info(MeasureCommandHandlerName, "Calling buildMeasurementResponseJSON")
	// respJSON, err := buildMeasurementResponseJSONForTargets(
	// 	[]measurementResponseTarget{
	// 		responseTargetFromResolvedPort(bufferData, getterTarget, setters[0].ConnectionJSON, getters[0]),
	// 	},
	// 	cmd.Hash,
	// )
	// if err != nil {
	// 	h.logger.Error(MeasureCommandHandlerName,
	// 		fmt.Sprintf("failed to build MeasurementResponse: %v", err))
	// 	return
	// }
	// h.logger.Info(MeasureCommandHandlerName, "buildMeasurementResponseJSON complete")
	//
	// h.publishMeasurementResponse(cmd, responseSubject, respJSON)
}
