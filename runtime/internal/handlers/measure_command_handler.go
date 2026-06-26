package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/serverinterpreter"
)

const (
	MeasureCommandHandlerName = "MEASURE_COMMAND_HANDLER"
	// INSTRUMENTHUB.MEASURE_COMMAND is the subject published by falcon-comms
	// RoutineComms on the controller side (routine_comms.cpp make_measure_command_subject).
	MeasureCommandSubject = "INSTRUMENTHUB.MEASURE_COMMAND"
	// FALCON.MEASURE_RESPONSE is the subject subscribed to by falcon-comms
	// RoutineComms on the controller side (routine_comms.cpp make_measure_response_subject).
	MeasureResponseSubject = "FALCON.MEASURE_RESPONSE"
	MeasureCommandName     = "MEASURE_COMMAND"
	MeasureResponseName    = "MEASURE_RESPONSE"
)

// BusyManager interface allows the handler to manage busy state
type BusyManager interface {
	SetIsBusy(busy bool)
}

// MeasurementDispatcher dispatches measurement scripts to the instrument-script-server.
type MeasurementDispatcher interface {
	RunMeasurement(scriptName string, globals map[string]interface{}, typeManifest map[string]interface{}) ([]serverinterpreter.ResolvedCallResult, error)
}

func inferSetterOnlyScriptName(setters []serverinterpreter.ExtractedInstrumentInfo) string {
	if len(setters) == 0 {
		return ""
	}

	candidates := []string{
		setters[0].DefaultName,
		setters[0].InstrumentFacingName,
		setters[0].Description,
		setters[0].PortJSON,
	}
	for _, candidate := range candidates {
		lowerCandidate := strings.ToLower(candidate)
		switch {
		case strings.Contains(lowerCandidate, "sample_rate"):
			return "set_sample_rate"
		case strings.Contains(lowerCandidate, "trigger_leader"),
			strings.Contains(lowerCandidate, "trigger leader"):
			return "set_trigger_leader"
		case strings.Contains(lowerCandidate, "number_of_samples"),
			strings.Contains(lowerCandidate, "bin-count"),
			strings.Contains(lowerCandidate, ".bins"),
			strings.Contains(lowerCandidate, "\"bins\""):
			return "set_number_of_samples"
		case strings.Contains(lowerCandidate, "slope"):
			return "set_slope"
		case strings.Contains(lowerCandidate, "set_voltage"),
			strings.Contains(lowerCandidate, ".dc_v"),
			strings.Contains(lowerCandidate, "\"dc_v\""),
			strings.Contains(lowerCandidate, "voltage"):
			return "set_voltage"
		}
	}

	return ""
}

func targetStateKey(id string, channel int) string {
	return fmt.Sprintf("%s:%d", id, channel)
}

func resolvedCallResultToFloatSlice(result serverinterpreter.ResolvedCallResult) []float64 {
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

// reverseWireMap builds a gate-name → InstrumentConnection lookup from the
// standard wiremap (which stores InstrumentConnection → gate-name).
func reverseWireMap(wm *config.WireMap) map[string]config.InstrumentConnection {
	if wm == nil {
		return nil
	}
	rev := make(map[string]config.InstrumentConnection, len(*wm))
	for instrConn, gateName := range *wm {
		rev[string(gateName)] = instrConn
	}
	return rev
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

// MeasureCommandHandler handles MEASURE_COMMAND requests
type MeasureCommandHandler struct {
	logger             *logging.Logger
	nc                 *nats.Conn
	js                 nats.JetStreamContext
	subscription       *nats.Subscription
	measurementManager *measurements.Manager
	instrumentHandler  *instrument.Handler
	busyManager        BusyManager
	dispatcher         MeasurementDispatcher
	wireMap            *config.WireMap
	stateMu            sync.Mutex
	voltages           map[string]float64
	sampleRates        map[string]float64
	numberOfSamples    map[string]int
	slopes             map[string]float64
	triggerLeaders     map[string]bool
}

// NewMeasureCommandHandler creates a new handler
func NewMeasureCommandHandler(
	logger *logging.Logger,
	measurementManager *measurements.Manager,
	instrumentHandler *instrument.Handler,
	busyManager BusyManager,
	dispatcher MeasurementDispatcher,
	wireMap *config.WireMap,
) *MeasureCommandHandler {
	return &MeasureCommandHandler{
		logger:             logger,
		measurementManager: measurementManager,
		instrumentHandler:  instrumentHandler,
		busyManager:        busyManager,
		dispatcher:         dispatcher,
		wireMap:            wireMap,
		voltages:           map[string]float64{},
		sampleRates:        map[string]float64{},
		numberOfSamples:    map[string]int{},
		slopes:             map[string]float64{},
		triggerLeaders:     map[string]bool{},
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

// handleMessage processes an INSTRUMENTHUB.MEASURE_COMMAND message, dispatches
// the measurement script to ISS, and publishes a FALCON.MEASURE_RESPONSE.
func (h *MeasureCommandHandler) handleMessage(msg *nats.Msg) {
	h.logger.Debug(
		MeasureCommandHandlerName,
		fmt.Sprintf("Received command: %s", string(msg.Data)),
	)

	var cmd api.MeasureCommand
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to unmarshal MEASURE_COMMAND: %v", err))
		return
	}

	if cmd.Request == "" {
		h.logger.Debug(MeasureCommandHandlerName, "empty request, ignoring")
		return
	}

	h.busyManager.SetIsBusy(true)
	defer h.busyManager.SetIsBusy(false)

	falconReq, err := serverinterpreter.NewFalconMeasurementRequestFromJSON(cmd.Request)
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to parse MeasurementRequest: %v", err))
		return
	}
	defer falconReq.Close()

	scriptName, scriptNameErr := falconReq.MeasurementName()
	if scriptNameErr != nil {
		h.logger.Debug(MeasureCommandHandlerName,
			fmt.Sprintf("MeasurementName() returned error: %v", scriptNameErr))
	}
	scriptName = strings.TrimSpace(scriptName)

	revWire := reverseWireMap(h.wireMap)

	if scriptName == "get_many_voltages" || scriptName == "get_all_voltages" {
		getters, err := falconReq.ExtractGetters()
		if err != nil || len(getters) == 0 {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to extract getters (got %d): %v", len(getters), err))
			return
		}

		getterTargets := make([]map[string]interface{}, 0, len(getters))
		responseTargets := make([]measurementResponseTarget, 0, len(getters))
		cachedVoltages := make([][]float64, len(getters))

		for i, getter := range getters {
			getterGate, err := gateNameFromConnectionJSON(getter.ConnectionJSON)
			if err != nil {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("failed to get getter gate name at index %d: %v", i, err))
				return
			}
			getterEntry, ok := revWire[getterGate]
			if !ok {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("getter gate %q not found in wiremap", getterGate))
				return
			}
			getterInstrID, getterChIdx, ok := parseWireMapEntry(getterEntry)
			if !ok {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("failed to parse getter wiremap entry %q", getterEntry))
				return
			}

			getterTargets = append(getterTargets, map[string]interface{}{
				"id":      getterInstrID,
				"channel": getterChIdx,
			})
			responseTargets = append(responseTargets, measurementResponseTarget{
				PortJSON:       getter.PortJSON,
				ConnectionJSON: getter.ConnectionJSON,
				InstrumentType: getter.InstrumentType,
				UnitsJSON:      getter.UnitsJSON,
			})

			h.stateMu.Lock()
			if voltage, ok := h.voltages[targetStateKey(getterInstrID, getterChIdx)]; ok {
				cachedVoltages[i] = []float64{voltage}
			}
			h.stateMu.Unlock()
		}

		globals := map[string]interface{}{
			"getters": getterTargets,
		}
		typeManifest := map[string]interface{}{
			"parameters": []map[string]interface{}{
				{"name": "ctx", "type": "RuntimeContext"},
				{"name": "getters", "type": "{InstrumentTarget}"},
			},
		}

		results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
		h.logger.Info(MeasureCommandHandlerName,
			fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("measurement dispatch failed: %v", err))
			return
		}

		for i := range responseTargets {
			if i < len(results) {
				responseTargets[i].BufferData = resolvedCallResultToFloatSlice(results[i])
			}
			if len(responseTargets[i].BufferData) == 0 && len(cachedVoltages[i]) > 0 {
				responseTargets[i].BufferData = cachedVoltages[i]
			}
			if len(responseTargets[i].BufferData) == 0 {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("no response value available for getter index %d in %s", i, scriptName))
				return
			}
		}

		respJSON, err := buildMeasurementResponseJSONForTargets(responseTargets, cmd.Hash)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to build MeasurementResponse: %v", err))
			return
		}

		measureSubject := "FALCON.MEASURE_DATA." + strconv.FormatInt(cmd.Hash, 10)
		if _, err := h.js.Publish(measureSubject, []byte(respJSON)); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish measurement to JetStream subject %s: %v", measureSubject, err))
			return
		}

		measureResp := api.MeasureResponse{
			Stream:    measureSubject,
			Response:  respJSON,
			Timestamp: time.Now().UnixMicro(),
			Hash:      cmd.Hash,
		}
		respData, err := json.Marshal(measureResp)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to marshal MeasureResponse: %v", err))
			return
		}

		if err := h.nc.Publish(MeasureResponseSubject, respData); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish %s: %v", MeasureResponseSubject, err))
		}
		return
	}

	if scriptName == "measure_current" || scriptName == "measure_illumination" {
		getters, err := falconReq.ExtractGetters()
		if err != nil || len(getters) == 0 {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to extract getters (got %d): %v", len(getters), err))
			return
		}

		getterTargets := make([]map[string]interface{}, 0, len(getters))
		responseTargets := make([]measurementResponseTarget, 0, len(getters))
		for i, getter := range getters {
			getterGate, err := gateNameFromConnectionJSON(getter.ConnectionJSON)
			if err != nil {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("failed to get getter gate name at index %d: %v", i, err))
				return
			}
			getterEntry, ok := revWire[getterGate]
			if !ok {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("getter gate %q not found in wiremap", getterGate))
				return
			}
			getterInstrID, getterChIdx, ok := parseWireMapEntry(getterEntry)
			if !ok {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("failed to parse getter wiremap entry %q", getterEntry))
				return
			}

			getterTargets = append(getterTargets, map[string]interface{}{
				"id":      getterInstrID,
				"channel": getterChIdx,
			})
			responseTargets = append(responseTargets, measurementResponseTarget{
				PortJSON:       getter.PortJSON,
				ConnectionJSON: getter.ConnectionJSON,
				InstrumentType: getter.InstrumentType,
				UnitsJSON:      getter.UnitsJSON,
			})
		}

		globals := map[string]interface{}{
			"sampleRate": 1000,
			"getters":    getterTargets,
		}
		parameters := []map[string]interface{}{
			{"name": "ctx", "type": "RuntimeContext"},
			{"name": "sampleRate", "type": "number"},
			{"name": "getters", "type": "{InstrumentTarget}"},
		}
		if scriptName == "measure_illumination" {
			globals["illuminationTime"] = 0.1
			parameters = append(parameters, map[string]interface{}{
				"name": "illuminationTime",
				"type": "number",
			})
		}

		typeManifest := map[string]interface{}{"parameters": parameters}
		results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
		h.logger.Info(MeasureCommandHandlerName,
			fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("measurement dispatch failed: %v", err))
			return
		}

		datapointResults := make([][]float64, 0, len(responseTargets))
		for _, result := range results {
			if strings.EqualFold(result.Verb, "GET_DATAPOINT") {
				if data := resolvedCallResultToFloatSlice(result); len(data) > 0 {
					datapointResults = append(datapointResults, data)
				}
			}
		}
		if len(datapointResults) == 0 {
			for _, result := range results {
				if data := resolvedCallResultToFloatSlice(result); len(data) > 0 {
					datapointResults = append(datapointResults, data)
				}
			}
		}

		for i := range responseTargets {
			if i < len(datapointResults) {
				responseTargets[i].BufferData = datapointResults[i]
			}
			if len(responseTargets[i].BufferData) == 0 {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("no scalar response returned for getter index %d in %s", i, scriptName))
				return
			}
		}

		respJSON, err := buildMeasurementResponseJSONForTargets(responseTargets, cmd.Hash)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to build MeasurementResponse: %v", err))
			return
		}

		measureSubject := "FALCON.MEASURE_DATA." + strconv.FormatInt(cmd.Hash, 10)
		if _, err := h.js.Publish(measureSubject, []byte(respJSON)); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish measurement to JetStream subject %s: %v", measureSubject, err))
			return
		}

		measureResp := api.MeasureResponse{
			Stream:    measureSubject,
			Response:  respJSON,
			Timestamp: time.Now().UnixMicro(),
			Hash:      cmd.Hash,
		}
		respData, err := json.Marshal(measureResp)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to marshal MeasureResponse: %v", err))
			return
		}

		if err := h.nc.Publish(MeasureResponseSubject, respData); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish %s: %v", MeasureResponseSubject, err))
		}
		return
	}

	if scriptName == "get_voltage" || scriptName == "get_sample_rate" ||
		scriptName == "get_number_of_samples" || scriptName == "get_slope" ||
		scriptName == "get_trigger_leader" {
		getters, err := falconReq.ExtractGetters()
		if err != nil || len(getters) == 0 {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to extract getters (got %d): %v", len(getters), err))
			return
		}
		h.logger.Debug(MeasureCommandHandlerName,
			fmt.Sprintf(
				"Resolved measurement name: %q (getter default=%q instrument-facing=%q)",
				scriptName,
				getters[0].DefaultName,
				getters[0].InstrumentFacingName,
			))

		getterGate, err := gateNameFromConnectionJSON(getters[0].ConnectionJSON)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to get getter gate name: %v", err))
			return
		}
		getterEntry, ok := revWire[getterGate]
		if !ok {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("getter gate %q not found in wiremap", getterGate))
			return
		}
		getterInstrID, getterChIdx, ok := parseWireMapEntry(getterEntry)
		if !ok {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to parse getter wiremap entry %q", getterEntry))
			return
		}

		globals := map[string]interface{}{
			"getter": map[string]interface{}{"id": getterInstrID, "channel": getterChIdx},
		}
		parameters := []map[string]interface{}{
			{"name": "ctx", "type": "RuntimeContext"},
			{"name": "getter", "type": "InstrumentTarget"},
		}

		stateKey := targetStateKey(getterInstrID, getterChIdx)
		h.stateMu.Lock()
		voltage, hasVoltage := h.voltages[stateKey]
		sampleRate, hasSampleRate := h.sampleRates[stateKey]
		numberOfSamples, hasNumberOfSamples := h.numberOfSamples[stateKey]
		slope, hasSlope := h.slopes[stateKey]
		triggerLeader, hasTriggerLeader := h.triggerLeaders[stateKey]
		h.stateMu.Unlock()

		switch scriptName {
		case "get_sample_rate":
			globals["sampleRate"] = sampleRate
			parameters = append(parameters, map[string]interface{}{
				"name": "sampleRate",
				"type": "number",
			})
		case "get_number_of_samples":
			globals["numberOfSamples"] = numberOfSamples
			parameters = append(parameters, map[string]interface{}{
				"name": "numberOfSamples",
				"type": "number",
			})
		case "get_slope":
			globals["slope"] = slope
			parameters = append(parameters, map[string]interface{}{
				"name": "slope",
				"type": "number",
			})
		case "get_trigger_leader":
			globals["triggerLeader"] = triggerLeader
			parameters = append(parameters, map[string]interface{}{
				"name": "triggerLeader",
				"type": "boolean",
			})
		}

		typeManifest := map[string]interface{}{"parameters": parameters}
		results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
		h.logger.Info(MeasureCommandHandlerName,
			fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("measurement dispatch failed: %v", err))
			return
		}

		var bufferData []float64
		for _, r := range results {
			switch r.Return.Type {
			case "buffer":
				bufferData = append(bufferData, r.BufferData...)
			case "float", "double", "number":
				if v, ok := r.Return.Value.(float64); ok {
					bufferData = append(bufferData, v)
				}
			case "integer", "int":
				switch v := r.Return.Value.(type) {
				case float64:
					bufferData = append(bufferData, v)
				case int:
					bufferData = append(bufferData, float64(v))
				}
			case "boolean":
				if v, ok := r.Return.Value.(bool); ok {
					if v {
						bufferData = append(bufferData, 1.0)
					} else {
						bufferData = append(bufferData, 0.0)
					}
				}
			}
		}
		if len(bufferData) == 0 {
			switch scriptName {
			case "get_voltage":
				if hasVoltage {
					bufferData = []float64{voltage}
				}
			case "get_sample_rate":
				if hasSampleRate {
					bufferData = []float64{sampleRate}
				}
			case "get_number_of_samples":
				if hasNumberOfSamples {
					bufferData = []float64{float64(numberOfSamples)}
				}
			case "get_slope":
				if hasSlope {
					bufferData = []float64{slope}
				}
			case "get_trigger_leader":
				if hasTriggerLeader {
					if triggerLeader {
						bufferData = []float64{1.0}
					} else {
						bufferData = []float64{0.0}
					}
				}
			}
			if len(bufferData) > 0 {
				h.logger.Info(MeasureCommandHandlerName,
					fmt.Sprintf("No explicit getter result returned for %s; using cached state fallback", scriptName))
			}
		}

		respJSON, err := buildMeasurementResponseJSON(
			bufferData,
			getters[0].PortJSON,
			getters[0].ConnectionJSON,
			getters[0].InstrumentType,
			getters[0].UnitsJSON,
			cmd.Hash,
		)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to build MeasurementResponse: %v", err))
			return
		}

		measureSubject := "FALCON.MEASURE_DATA." + strconv.FormatInt(cmd.Hash, 10)
		if _, err := h.js.Publish(measureSubject, []byte(respJSON)); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish measurement to JetStream subject %s: %v", measureSubject, err))
			return
		}

		measureResp := api.MeasureResponse{
			Stream:    measureSubject,
			Response:  respJSON,
			Timestamp: time.Now().UnixMicro(),
			Hash:      cmd.Hash,
		}
		respData, err := json.Marshal(measureResp)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to marshal MeasureResponse: %v", err))
			return
		}

		if err := h.nc.Publish(MeasureResponseSubject, respData); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish %s: %v", MeasureResponseSubject, err))
		}
		return
	}

	setters, err := falconReq.ExtractSetters()
	if err != nil || len(setters) == 0 {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to extract setters (got %d): %v", len(setters), err))
		return
	}
	if scriptName == "" {
		scriptName = inferSetterOnlyScriptName(setters)
	}
	h.logger.Debug(MeasureCommandHandlerName,
		fmt.Sprintf(
			"Resolved measurement name: %q (setter default=%q instrument-facing=%q)",
			scriptName,
			setters[0].DefaultName,
			setters[0].InstrumentFacingName,
		))

	// Setter: ConnectionJSON → gate name → reverse wiremap → {id, channel}
	setterGate, err := gateNameFromConnectionJSON(setters[0].ConnectionJSON)
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to get setter gate name: %v", err))
		return
	}
	setterEntry, ok := revWire[setterGate]
	if !ok {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("setter gate %q not found in wiremap", setterGate))
		return
	}
	setterInstrID, setterChIdx, ok := parseWireMapEntry(setterEntry)
	if !ok {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to parse setter wiremap entry %q", setterEntry))
		return
	}

	if scriptName == "set_voltage" || scriptName == "set_sample_rate" ||
		scriptName == "set_number_of_samples" || scriptName == "set_slope" ||
		scriptName == "set_trigger_leader" {
		waveformData, err := serverinterpreter.ExtractWaveformDataFromRequestByIndex(falconReq, 0)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to extract %s waveform data: %v", scriptName, err))
			return
		}

		scalarValue := waveformData.TimeDomain.Min
		if len(waveformData.RawTimeTrace) > 0 && len(waveformData.RawTimeTrace[0]) > 0 {
			scalarValue = waveformData.RawTimeTrace[0][0]
		}

		targetName := "setter"
		valueName := "setVoltage"
		responseValue := scalarValue
		globals := map[string]interface{}{}
		targetValue := map[string]interface{}{"id": setterInstrID, "channel": setterChIdx}
		includeValue := true
		valueType := "number"

		switch scriptName {
		case "set_voltage":
			targetName = "setter"
			valueName = "setVoltage"
			responseValue = scalarValue
		case "set_sample_rate":
			targetName = "getter"
			valueName = "sampleRate"
			responseValue = scalarValue
		case "set_number_of_samples":
			targetName = "getter"
			valueName = "numberOfSamples"
			responseValue = float64(int(scalarValue))
		case "set_slope":
			targetName = "setter"
			valueName = "slope"
			responseValue = scalarValue
		case "set_trigger_leader":
			targetName = "getter"
			valueName = ""
			responseValue = 1.0
			includeValue = false
			valueType = ""
		}

		globals[targetName] = targetValue
		if includeValue {
			if scriptName == "set_number_of_samples" {
				globals[valueName] = int(responseValue)
			} else {
				globals[valueName] = responseValue
			}
		}

		parameters := []map[string]interface{}{
			{"name": "ctx", "type": "RuntimeContext"},
			{"name": targetName, "type": "InstrumentTarget"},
		}
		if includeValue {
			parameters = append(parameters, map[string]interface{}{
				"name": valueName,
				"type": valueType,
			})
		}
		typeManifest := map[string]interface{}{"parameters": parameters}

		results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
		h.logger.Info(MeasureCommandHandlerName,
			fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("measurement dispatch failed: %v", err))
			return
		}

		stateKey := targetStateKey(setterInstrID, setterChIdx)
		h.stateMu.Lock()
		switch scriptName {
		case "set_voltage":
			h.voltages[stateKey] = responseValue
		case "set_sample_rate":
			h.sampleRates[stateKey] = responseValue
		case "set_number_of_samples":
			h.numberOfSamples[stateKey] = int(responseValue)
		case "set_slope":
			h.slopes[stateKey] = responseValue
		case "set_trigger_leader":
			h.triggerLeaders[stateKey] = true
		}
		h.stateMu.Unlock()

		bufferData := []float64{responseValue}
		respJSON, err := buildMeasurementResponseJSON(
			bufferData,
			setters[0].PortJSON,
			setters[0].ConnectionJSON,
			setters[0].InstrumentType,
			setters[0].UnitsJSON,
			cmd.Hash,
		)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to build MeasurementResponse: %v", err))
			return
		}

		measureSubject := "FALCON.MEASURE_DATA." + strconv.FormatInt(cmd.Hash, 10)
		if _, err := h.js.Publish(measureSubject, []byte(respJSON)); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish measurement to JetStream subject %s: %v", measureSubject, err))
			return
		}

		measureResp := api.MeasureResponse{
			Stream:    measureSubject,
			Response:  respJSON,
			Timestamp: time.Now().UnixMicro(),
			Hash:      cmd.Hash,
		}
		respData, err := json.Marshal(measureResp)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to marshal MeasureResponse: %v", err))
			return
		}

		if err := h.nc.Publish(MeasureResponseSubject, respData); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish %s: %v", MeasureResponseSubject, err))
		}
		return
	}

	if scriptName == "set_many_voltages" || scriptName == "ramp" {
		setterTargets := make([]map[string]interface{}, 0, len(setters))
		setVoltages := make(map[string]float64, len(setters))
		responseTargets := make([]measurementResponseTarget, 0, len(setters))

		for i, setter := range setters {
			setterGate, err := gateNameFromConnectionJSON(setter.ConnectionJSON)
			if err != nil {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("failed to get setter gate name at index %d: %v", i, err))
				return
			}
			setterEntry, ok := revWire[setterGate]
			if !ok {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("setter gate %q not found in wiremap", setterGate))
				return
			}
			setterInstrID, setterChIdx, ok := parseWireMapEntry(setterEntry)
			if !ok {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("failed to parse setter wiremap entry %q", setterEntry))
				return
			}

			waveformData, err := serverinterpreter.ExtractWaveformDataFromRequestByIndex(falconReq, i)
			if err != nil {
				h.logger.Error(MeasureCommandHandlerName,
					fmt.Sprintf("failed to extract %s waveform data at index %d: %v", scriptName, i, err))
				return
			}

			scalarValue := waveformData.TimeDomain.Min
			if len(waveformData.RawTimeTrace) > 0 && len(waveformData.RawTimeTrace[0]) > 0 {
				scalarValue = waveformData.RawTimeTrace[0][0]
			}

			setterTargets = append(setterTargets, map[string]interface{}{
				"id":      setterInstrID,
				"channel": setterChIdx,
			})
			setVoltages[fmt.Sprintf("%s:%d", setterInstrID, setterChIdx)] = scalarValue
			h.stateMu.Lock()
			h.voltages[targetStateKey(setterInstrID, setterChIdx)] = scalarValue
			h.stateMu.Unlock()
			responseTargets = append(responseTargets, measurementResponseTarget{
				BufferData:     []float64{scalarValue},
				PortJSON:       setter.PortJSON,
				ConnectionJSON: setter.ConnectionJSON,
				InstrumentType: setter.InstrumentType,
				UnitsJSON:      setter.UnitsJSON,
			})
		}

		globals := map[string]interface{}{
			"setters":     setterTargets,
			"setVoltages": setVoltages,
		}
		typeManifest := map[string]interface{}{
			"parameters": []map[string]interface{}{
				{"name": "ctx", "type": "RuntimeContext"},
				{"name": "setters", "type": "{InstrumentTarget}"},
				{"name": "setVoltages", "type": "{string: number}"},
			},
		}

		results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
		h.logger.Info(MeasureCommandHandlerName,
			fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("measurement dispatch failed: %v", err))
			return
		}

		respJSON, err := buildMeasurementResponseJSONForTargets(responseTargets, cmd.Hash)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to build MeasurementResponse: %v", err))
			return
		}

		measureSubject := "FALCON.MEASURE_DATA." + strconv.FormatInt(cmd.Hash, 10)
		if _, err := h.js.Publish(measureSubject, []byte(respJSON)); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish measurement to JetStream subject %s: %v", measureSubject, err))
			return
		}

		measureResp := api.MeasureResponse{
			Stream:    measureSubject,
			Response:  respJSON,
			Timestamp: time.Now().UnixMicro(),
			Hash:      cmd.Hash,
		}
		respData, err := json.Marshal(measureResp)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to marshal MeasureResponse: %v", err))
			return
		}

		if err := h.nc.Publish(MeasureResponseSubject, respData); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish %s: %v", MeasureResponseSubject, err))
		}
		return
	}

	getters, err := falconReq.ExtractGetters()
	if err != nil || len(getters) == 0 {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to extract getters (got %d): %v", len(getters), err))
		return
	}

	// Getter: ConnectionJSON → gate name → reverse wiremap → {id, channel}
	getterGate, err := gateNameFromConnectionJSON(getters[0].ConnectionJSON)
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to get getter gate name: %v", err))
		return
	}
	getterEntry, ok := revWire[getterGate]
	if !ok {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("getter gate %q not found in wiremap", getterGate))
		return
	}
	getterInstrID, getterChIdx, ok := parseWireMapEntry(getterEntry)
	if !ok {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to parse getter wiremap entry %q", getterEntry))
		return
	}

	if scriptName == "measure_leakage" {
		waveformData, _, err := serverinterpreter.ExtractWaveformDataFromRequest(falconReq)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to extract waveform data for measure_leakage: %v", err))
			return
		}

		leakageVoltage := waveformData.TimeDomain.Min
		if len(waveformData.RawTimeTrace) > 0 && len(waveformData.RawTimeTrace[0]) > 0 {
			leakageVoltage = waveformData.RawTimeTrace[0][0]
		}

		globals := map[string]interface{}{
			"getter": map[string]interface{}{
				"id":      getterInstrID,
				"channel": getterChIdx,
			},
			"voltage": leakageVoltage,
		}
		typeManifest := map[string]interface{}{
			"parameters": []map[string]interface{}{
				{"name": "ctx", "type": "RuntimeContext"},
				{"name": "getter", "type": "InstrumentTarget"},
				{"name": "voltage", "type": "number"},
			},
		}

		results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
		h.logger.Info(MeasureCommandHandlerName,
			fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("measurement dispatch failed: %v", err))
			return
		}

		var bufferData []float64
		for _, r := range results {
			bufferData = append(bufferData, resolvedCallResultToFloatSlice(r)...)
		}
		if len(bufferData) == 0 {
			bufferData = []float64{leakageVoltage}
		}

		respJSON, err := buildMeasurementResponseJSON(
			bufferData,
			getters[0].PortJSON,
			getters[0].ConnectionJSON,
			getters[0].InstrumentType,
			getters[0].UnitsJSON,
			cmd.Hash,
		)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to build MeasurementResponse: %v", err))
			return
		}

		measureSubject := "FALCON.MEASURE_DATA." + strconv.FormatInt(cmd.Hash, 10)
		if _, err := h.js.Publish(measureSubject, []byte(respJSON)); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish measurement to JetStream subject %s: %v", measureSubject, err))
			return
		}

		measureResp := api.MeasureResponse{
			Stream:    measureSubject,
			Response:  respJSON,
			Timestamp: time.Now().UnixMicro(),
			Hash:      cmd.Hash,
		}
		respData, err := json.Marshal(measureResp)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to marshal MeasureResponse: %v", err))
			return
		}

		if err := h.nc.Publish(MeasureResponseSubject, respData); err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to publish %s: %v", MeasureResponseSubject, err))
		}
		return
	}

	waveformData, _, err := serverinterpreter.ExtractWaveformDataFromRequest(falconReq)
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to extract waveform data: %v", err))
		return
	}

	sweepVoltages := make([]interface{}, len(waveformData.RawTimeTrace))
	for i, row := range waveformData.RawTimeTrace {
		if len(row) > 0 {
			sweepVoltages[i] = row[0]
		} else {
			sweepVoltages[i] = 0.0
		}
	}

	var globals map[string]interface{}
	var typeManifest map[string]interface{}
	if len(setters) >= 2 {
		// 2D sweep: fast axis = setters[0], slow axis = setters[1]
		slowSetterGate, err := gateNameFromConnectionJSON(setters[1].ConnectionJSON)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to get slow setter gate name: %v", err))
			return
		}
		slowSetterEntry, ok := revWire[slowSetterGate]
		if !ok {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("slow setter gate %q not found in wiremap", slowSetterGate))
			return
		}
		slowSetterInstrID, slowSetterChIdx, ok := parseWireMapEntry(slowSetterEntry)
		if !ok {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to parse slow setter wiremap entry %q", slowSetterEntry))
			return
		}
		slowWaveformData, err := serverinterpreter.ExtractWaveformDataFromRequestByIndex(falconReq, 1)
		if err != nil {
			h.logger.Error(MeasureCommandHandlerName,
				fmt.Sprintf("failed to extract slow axis waveform data: %v", err))
			return
		}
		slowSweepVoltages := make([]interface{}, len(slowWaveformData.RawTimeTrace))
		for i, row := range slowWaveformData.RawTimeTrace {
			if len(row) > 0 {
				slowSweepVoltages[i] = row[0]
			} else {
				slowSweepVoltages[i] = 0.0
			}
		}
		if scriptName == "measure_2D_buffered" {
			numXSteps := len(sweepVoltages)
			if numXSteps == 0 {
				numXSteps = 1
			}
			numYSteps := len(slowSweepVoltages)
			if numYSteps == 0 {
				numYSteps = 1
			}
			globals = map[string]interface{}{
				"bufferedXSetters": []map[string]interface{}{
					{"id": setterInstrID, "channel": setterChIdx},
				},
				"sampleRate": 1000,
				"bufferedGetters": []map[string]interface{}{
					{"id": getterInstrID, "channel": getterChIdx},
				},
				"bufferedYSetters": []map[string]interface{}{
					{"id": slowSetterInstrID, "channel": slowSetterChIdx},
				},
				"numXSteps": numXSteps,
				"setYVoltageDomains": map[string]interface{}{
					slowSetterInstrID: map[string]interface{}{
						"min": slowWaveformData.TimeDomain.Min,
						"max": slowWaveformData.TimeDomain.Max,
					},
				},
				"setXVoltageDomains": map[string]interface{}{
					setterInstrID: map[string]interface{}{
						"min": waveformData.TimeDomain.Min,
						"max": waveformData.TimeDomain.Max,
					},
				},
				"numPoints": 1,
				"numYSteps": numYSteps,
				"setters":   []map[string]interface{}{},
			}
			typeManifest = map[string]interface{}{
				"parameters": []map[string]interface{}{
					{"name": "ctx", "type": "RuntimeContext"},
					{"name": "bufferedXSetters", "type": "{InstrumentTarget}"},
					{"name": "sampleRate", "type": "number"},
					{"name": "bufferedGetters", "type": "{InstrumentTarget}"},
					{"name": "bufferedYSetters", "type": "{InstrumentTarget}"},
					{"name": "numXSteps", "type": "number"},
					{"name": "setYVoltageDomains", "type": "table"},
					{"name": "setXVoltageDomains", "type": "table"},
					{"name": "numPoints", "type": "number"},
					{"name": "numYSteps", "type": "number"},
					{"name": "setters", "type": "{InstrumentTarget}"},
				},
			}
		} else {
			globals = map[string]interface{}{
				"getters":           []map[string]interface{}{{"id": getterInstrID, "channel": getterChIdx}},
				"fastSweepVoltages": sweepVoltages,
				"slowSweepVoltages": slowSweepVoltages,
				"fastSetter":        map[string]interface{}{"id": setterInstrID, "channel": setterChIdx},
				"slowSetter":        map[string]interface{}{"id": slowSetterInstrID, "channel": slowSetterChIdx},
			}
			typeManifest = map[string]interface{}{
				"parameters": []map[string]interface{}{
					{"name": "ctx", "type": "RuntimeContext"},
					{"name": "getters", "type": "{InstrumentTarget}"},
					{"name": "fastSweepVoltages", "type": "{number}"},
					{"name": "slowSweepVoltages", "type": "{number}"},
					{"name": "fastSetter", "type": "InstrumentTarget"},
					{"name": "slowSetter", "type": "InstrumentTarget"},
				},
			}
		}
	} else {
		if scriptName == "measure_get_set" {
			numPoints := len(sweepVoltages)
			if numPoints == 0 {
				numPoints = 1
			}
			sampleRate := 1000
			setVoltage := 0.0
			if len(sweepVoltages) > 0 {
				if v, ok := sweepVoltages[0].(float64); ok {
					setVoltage = v
				}
			}
			globals = map[string]interface{}{
				"getters":    []map[string]interface{}{{"id": getterInstrID, "channel": getterChIdx}},
				"numPoints":  numPoints,
				"sampleRate": sampleRate,
				"setVoltages": map[string]interface{}{
					setterInstrID: setVoltage,
				},
				"setters": []map[string]interface{}{{"id": setterInstrID, "channel": setterChIdx}},
			}
			typeManifest = map[string]interface{}{
				"parameters": []map[string]interface{}{
					{"name": "ctx", "type": "RuntimeContext"},
					{"name": "getters", "type": "{InstrumentTarget}"},
					{"name": "numPoints", "type": "number"},
					{"name": "sampleRate", "type": "number"},
					{"name": "setVoltages", "type": "{string: number}"},
					{"name": "setters", "type": "{InstrumentTarget}"},
				},
			}
		} else if scriptName == "measure_1D_buffered" {
			numSteps := len(sweepVoltages)
			if numSteps == 0 {
				numSteps = 1
			}
			globals = map[string]interface{}{
				"sampleRate": 1000,
				"setters":    []map[string]interface{}{},
				"setVoltageDomains": map[string]interface{}{
					setterInstrID: map[string]interface{}{
						"min": waveformData.TimeDomain.Min,
						"max": waveformData.TimeDomain.Max,
					},
				},
				"bufferedGetters": []map[string]interface{}{
					{"id": getterInstrID, "channel": getterChIdx},
				},
				"numPoints": 1,
				"numSteps":  numSteps,
				"bufferedSetters": []map[string]interface{}{
					{"id": setterInstrID, "channel": setterChIdx},
				},
			}
			typeManifest = map[string]interface{}{
				"parameters": []map[string]interface{}{
					{"name": "ctx", "type": "RuntimeContext"},
					{"name": "sampleRate", "type": "number"},
					{"name": "setters", "type": "{InstrumentTarget}"},
					{"name": "setVoltageDomains", "type": "table"},
					{"name": "bufferedGetters", "type": "{InstrumentTarget}"},
					{"name": "numPoints", "type": "number"},
					{"name": "numSteps", "type": "number"},
					{"name": "bufferedSetters", "type": "{InstrumentTarget}"},
				},
			}
		} else {
			// 1D sweep
			globals = map[string]interface{}{
				"getters":       []map[string]interface{}{{"id": getterInstrID, "channel": getterChIdx}},
				"setters":       []map[string]interface{}{{"id": setterInstrID, "channel": setterChIdx}},
				"sweepVoltages": sweepVoltages,
			}
			typeManifest = map[string]interface{}{
				"parameters": []map[string]interface{}{
					{"name": "ctx", "type": "RuntimeContext"},
					{"name": "getters", "type": "{InstrumentTarget}"},
					{"name": "sweepVoltages", "type": "{number}"},
					{"name": "setters", "type": "{InstrumentTarget}"},
				},
			}
		}
	}

	results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("RunMeasurement returned: resultCount=%d err=%v", len(results), err))
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("measurement dispatch failed: %v", err))
		return
	}

	var bufferData []float64
	for _, r := range results {
		switch r.Return.Type {
		case "buffer":
			bufferData = append(bufferData, r.BufferData...)
		case "float", "double", "number":
			if v, ok := r.Return.Value.(float64); ok {
				bufferData = append(bufferData, v)
			}
		}
	}
	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("bufferData collected: len=%d", len(bufferData)))

	h.logger.Info(MeasureCommandHandlerName, "Calling buildMeasurementResponseJSON")
	respJSON, err := buildMeasurementResponseJSON(
		bufferData,
		"",
		setters[0].ConnectionJSON,
		getters[0].InstrumentType,
		getters[0].UnitsJSON,
		cmd.Hash,
	)
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to build MeasurementResponse: %v", err))
		return
	}
	h.logger.Info(MeasureCommandHandlerName, "buildMeasurementResponseJSON complete")

	measureSubject := "FALCON.MEASURE_DATA." + strconv.FormatInt(cmd.Hash, 10)
	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Publishing measurement to JetStream subject %s", measureSubject))
	if _, err := h.js.Publish(measureSubject, []byte(respJSON)); err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to publish measurement to JetStream subject %s: %v", measureSubject, err))
		return
	}

	measureResp := api.MeasureResponse{
		Stream:    measureSubject,
		Response:  respJSON,
		Timestamp: time.Now().UnixMicro(),
		Hash:      cmd.Hash,
	}
	respData, err := json.Marshal(measureResp)
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to marshal MeasureResponse: %v", err))
		return
	}

	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Publishing to NATS subject %s", MeasureResponseSubject))
	if err := h.nc.Publish(MeasureResponseSubject, respData); err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to publish %s: %v", MeasureResponseSubject, err))
	}
	h.logger.Info(MeasureCommandHandlerName, "NATS publish complete; handler done")
}
