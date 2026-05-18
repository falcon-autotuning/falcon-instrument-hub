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
	if scriptName == "" {
		scriptName = "measure_get_set"
	}

	numPoints, _ := falconReq.ExtractNumPoints()
	if numPoints <= 0 {
		numPoints = 100
	}

	globals := map[string]interface{}{
		"getters":     []map[string]interface{}{{"id": getterInstrID, "channel": getterChIdx}},
		"setters":     []map[string]interface{}{{"id": setterInstrID, "channel": setterChIdx}},
		"setVoltages": map[string]interface{}{setterInstrID: 0.0},
		"numPoints":   numPoints,
		"sampleRate":  1000.0,
	}
	typeManifest := map[string]interface{}{
		"parameters": []map[string]interface{}{
			{"name": "ctx", "type": "RuntimeContext"},
			{"name": "getters", "type": "{InstrumentTarget}"},
			{"name": "numPoints", "type": "number"},
			{"name": "sampleRate", "type": "number"},
			{"name": "setVoltages", "type": "{string:number}"},
			{"name": "setters", "type": "{InstrumentTarget}"},
		},
	}

	results, err := h.dispatcher.RunMeasurement(scriptName, globals, typeManifest)
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("measurement dispatch failed: %v", err))
		return
	}

	var bufferData []float64
	for _, r := range results {
		if r.Return.Type == "buffer" {
			bufferData = r.BufferData
			break
		}
	}

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

	measureResp := api.MeasureResponse{
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
}
