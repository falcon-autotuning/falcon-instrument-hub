package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

	setters, err := falconReq.ExtractSetters()
	if err != nil || len(setters) == 0 {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to extract setters (got %d): %v", len(setters), err))
		return
	}

	getters, err := falconReq.ExtractGetters()
	if err != nil || len(getters) == 0 {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to extract getters (got %d): %v", len(getters), err))
		return
	}

	revWire := reverseWireMap(h.wireMap)

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

	scriptName, _ := falconReq.MeasurementName()

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
