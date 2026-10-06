//go:build cgo && falcon_core

package interpreter

import (
	"fmt"
	"math"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
)

const getVoltageCommand = "GET_VOLTAGE"

type getVoltageHandler struct{}

func (*getVoltageHandler) Name() string { return "get_voltage" }

func (*getVoltageHandler) CanHandle(req *FalconMeasurementRequest) (bool, error) {
	if req == nil {
		return false, fmt.Errorf("request is nil")
	}
	return true, nil
}

func getVoltageFromResult(result MeasurementResult, expected config.ConnectedPort) (float64, error) {
	if result.Err != nil {
		return 0, result.Err
	}
	if len(result.Results) != 1 {
		return 0, fmt.Errorf("get_voltage returned %d instrument calls, want 1", len(result.Results))
	}

	call := result.Results[0]
	if call.Instrument != expected.InstrumentName ||
		call.Group != expected.ChannelName ||
		call.Channel != int64(expected.ChannelIndex) ||
		call.Verb != getVoltageCommand {
		return 0, fmt.Errorf(
			"unexpected get_voltage result identity: instrument=%q group=%q channel=%d command=%q",
			call.Instrument,
			call.Group,
			call.Channel,
			call.Verb,
		)
	}
	if len(call.Return) != 1 {
		return 0, fmt.Errorf("get_voltage returned %d values, want 1", len(call.Return))
	}

	var voltage float64
	switch value := call.Return[0].Value.Value.(type) {
	case float64:
		voltage = value
	case int64:
		voltage = float64(value)
	default:
		return 0, fmt.Errorf("get_voltage returned unsupported value type %T", value)
	}
	if math.IsNaN(voltage) || math.IsInf(voltage, 0) {
		return 0, fmt.Errorf("get_voltage returned a non-finite value")
	}
	return voltage, nil
}

func (*getVoltageHandler) Handle(
	req *FalconMeasurementRequest,
	dispatcher *MeasurementDispatcher,
	_ *config.WireMap,
	connected *config.ConnectedPorts,
) (*FalconMeasurementResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if dispatcher == nil {
		return nil, fmt.Errorf("measurement dispatcher is nil")
	}

	getters, err := req.ExtractGetters()
	if err != nil {
		return nil, fmt.Errorf("extract get_voltage getter: %w", err)
	}
	if len(getters) != 1 {
		return nil, fmt.Errorf("get_voltage requires exactly one getter, got %d", len(getters))
	}
	getter := getters[0]

	gateName, err := gateNameFromConnectionJSON(getter.ConnectionJSON)
	if err != nil {
		return nil, fmt.Errorf("resolve get_voltage gate: %w", err)
	}
	resolved, err := connected.ResolveConnectedPort(connected, gateName)
	if err != nil {
		return nil, err
	}

	serialized, err := (instrumenttarget.Descriptor{
		Instrument: resolved.InstrumentName,
		Group:      resolved.ChannelName,
		Channel:    resolved.ChannelIndex,
	}).Serialize()
	if err != nil {
		return nil, fmt.Errorf("serialize get_voltage CallStack: %w", err)
	}

	results := dispatcher.RunAll([]MeasurementRequest{{
		Script: "get_voltage",
		Variables: []instrumentserver.MeasureVariable{{
			Name: "getter",
			Value: instrumentserver.VariableValue{
				Value: instrumentserver.CallStack(serialized),
			},
		}},
	}})
	if len(results) != 1 {
		return nil, fmt.Errorf("get_voltage returned %d measurement results, want 1", len(results))
	}

	voltage, err := getVoltageFromResult(results[0], resolved)
	if err != nil {
		return nil, err
	}
	responseJSON, err := buildMeasurementResponseJSON(
		[]float64{voltage},
		getter.PortJSON,
		getter.ConnectionJSON,
		getter.InstrumentType,
		getter.UnitsJSON,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("build get_voltage response: %w", err)
	}
	return NewFalconMeasurementResponseFromJSON(responseJSON)
}
