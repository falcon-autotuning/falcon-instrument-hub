//go:build cgo

package interpreter

import (
	"fmt"
	"math"
	"strings"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/autotuner-interfaces/contexts/acquisitioncontext"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listlabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
)

const (
	getVoltageHandlerName = "get_voltage"
	getVoltageCommand     = "GET_VOLTAGE"
	getVoltageOutput      = "measured_voltage"
)

type getVoltageHandler struct{}

func (*getVoltageHandler) Name() string { return getVoltageHandlerName }

// getVoltageGetter recognizes the first supported vertical slice:
// one scalar voltage meter read, with no control waveform or meter transform.
// The caller owns the returned getter handle.
func getVoltageGetter(
	req *FalconMeasurementRequest,
) (*instrumentport.Handle, bool, error) {
	if req == nil || req.Handle() == nil {
		return nil, false, fmt.Errorf("request is nil")
	}

	waveforms, err := req.Handle().Waveforms()
	if err != nil {
		return nil, false, fmt.Errorf("read request waveforms: %w", err)
	}
	defer waveforms.Close()

	waveformCount, err := waveforms.Size()
	if err != nil {
		return nil, false, fmt.Errorf("read request waveform count: %w", err)
	}
	if waveformCount != 0 {
		return nil, false, nil
	}

	transforms, err := req.Handle().MeterTransforms()
	if err != nil {
		return nil, false, fmt.Errorf("read request meter transforms: %w", err)
	}
	defer transforms.Close()

	transformCount, err := transforms.Size()
	if err != nil {
		return nil, false, fmt.Errorf(
			"read request meter-transform count: %w",
			err,
		)
	}
	if transformCount != 0 {
		return nil, false, nil
	}

	getters, err := req.Handle().Getters()
	if err != nil {
		return nil, false, fmt.Errorf("read request getters: %w", err)
	}
	defer getters.Close()

	getterCount, err := getters.Size()
	if err != nil {
		return nil, false, fmt.Errorf("read request getter count: %w", err)
	}
	if getterCount != 1 {
		return nil, false, nil
	}

	getter, err := getters.At(0)
	if err != nil {
		return nil, false, fmt.Errorf("read request getter: %w", err)
	}

	isMeter, err := getter.IsMeter()
	if err != nil {
		getter.Close()
		return nil, false, fmt.Errorf("read getter port type: %w", err)
	}
	if !isMeter {
		getter.Close()
		return nil, false, nil
	}

	portAccess, err := getter.Access()
	if err != nil {
		getter.Close()
		return nil, false, fmt.Errorf("read getter access: %w", err)
	}
	if portAccess != access.Read && portAccess != access.Readwrite {
		getter.Close()
		return nil, false, nil
	}

	instrumentType, err := getter.InstrumentType()
	if err != nil {
		getter.Close()
		return nil, false, fmt.Errorf("read getter instrument type: %w", err)
	}
	switch instrumentType {
	case instrument.DcVoltageSource,
		instrument.VoltageSource,
		instrument.HfVoltageSource,
		instrument.Voltmeter:
	default:
		getter.Close()
		return nil, false, nil
	}

	units, err := getter.Units()
	if err != nil {
		getter.Close()
		return nil, false, fmt.Errorf("read getter units: %w", err)
	}
	defer units.Close()

	volts, err := SymbolUnitFromString("V")
	if err != nil {
		getter.Close()
		return nil, false, fmt.Errorf("create voltage unit: %w", err)
	}
	defer volts.Close()

	compatible, err := units.IsCompatibleWith(volts)
	if err != nil {
		getter.Close()
		return nil, false, fmt.Errorf(
			"compare getter units with volts: %w",
			err,
		)
	}
	if !compatible {
		getter.Close()
		return nil, false, nil
	}

	return getter, true, nil
}

func (*getVoltageHandler) CanHandle(
	req *FalconMeasurementRequest,
) (bool, error) {
	getter, matches, err := getVoltageGetter(req)
	if getter != nil {
		getter.Close()
	}
	return matches, err
}

func voltageFromResult(
	result dispatcher.MeasurementResult,
	expected config.ConnectedPort,
	getter *instrumentport.Handle,
) (float64, error) {
	if result.Err != nil {
		return 0, result.Err
	}
	if len(result.Results) != 1 {
		return 0, fmt.Errorf(
			"get_voltage returned %d instrument calls, want 1",
			len(result.Results),
		)
	}

	call := result.Results[0]
	if call.Instrument != expected.InstrumentName ||
		call.Group != expected.ChannelGroup ||
		call.Channel != int64(expected.Channel) ||
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
		return 0, fmt.Errorf(
			"get_voltage returned %d values, want 1",
			len(call.Return),
		)
	}
	if call.Return[0].Name != getVoltageOutput {
		return 0, fmt.Errorf(
			"get_voltage returned output %q, want %q",
			call.Return[0].Name,
			getVoltageOutput,
		)
	}

	var value float64
	switch raw := call.Return[0].Value.Value.(type) {
	case float64:
		value = raw
	case int64:
		value = float64(raw)
	default:
		return 0, fmt.Errorf("get_voltage returned unsupported value type %T", raw)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("get_voltage returned a non-finite value")
	}

	resultUnits, err := SymbolUnitFromString(call.Return[0].Unit)
	if err != nil {
		return 0, fmt.Errorf(
			"read get_voltage result units %q: %w",
			call.Return[0].Unit,
			err,
		)
	}
	defer resultUnits.Close()

	getterUnits, err := getter.Units()
	if err != nil {
		return 0, fmt.Errorf("read getter units: %w", err)
	}
	defer getterUnits.Close()

	compatible, err := resultUnits.IsCompatibleWith(getterUnits)
	if err != nil {
		return 0, fmt.Errorf("compare result and getter units: %w", err)
	}
	if !compatible {
		return 0, fmt.Errorf(
			"get_voltage result unit %q is incompatible with the requested getter",
			call.Return[0].Unit,
		)
	}

	converted, err := resultUnits.ConvertValueTo(value, getterUnits)
	if err != nil {
		return 0, fmt.Errorf(
			"convert get_voltage result to getter units: %w",
			err,
		)
	}
	return converted, nil
}

// DEPRECATED: newScalarMeasurementResponse is a temporary shim to convert a
// single scalar voltage value into a FalconMeasurementResponse.
// It will be removed once the get_voltage handler is refactored to return a MeasurementResponse directly.
// get_voltage handler is refactored to return a MeasurementResponse directly.
// newScalarMeasurementResponse attributes one scalar result to the original
// request port. Keeping the original port preserves its full Falcon identity.
func newScalarMeasurementResponse(
	value float64,
	port *instrumentport.Handle,
) (*FalconMeasurementResponse, error) {
	context, err := acquisitioncontext.NewFromPort(port)
	if err != nil {
		return nil, fmt.Errorf(
			"create acquisition context from getter: %w",
			err,
		)
	}
	defer context.Close()

	data, err := farraydouble.FromData([]float64{value}, []uint64{1})
	if err != nil {
		return nil, fmt.Errorf("create scalar measurement data: %w", err)
	}
	defer data.Close()

	array, err := labelledmeasuredarray.FromFArray(data, context)
	if err != nil {
		return nil, fmt.Errorf("label scalar measurement data: %w", err)
	}
	defer array.Close()

	list, err := listlabelledmeasuredarray.New(
		[]*labelledmeasuredarray.Handle{array},
	)
	if err != nil {
		return nil, fmt.Errorf("create measurement array list: %w", err)
	}
	defer list.Close()

	arrays, err := labelledarrayslabelledmeasuredarray.NewFromList(list)
	if err != nil {
		return nil, fmt.Errorf("create labelled measurement arrays: %w", err)
	}
	defer arrays.Close()

	response, err := measurementresponse.New(arrays)
	if err != nil {
		return nil, fmt.Errorf("create MeasurementResponse: %w", err)
	}

	return &FalconMeasurementResponse{handle: response}, nil
}

func (*getVoltageHandler) Handle(
	req *FalconMeasurementRequest,
	measurementDispatcher *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	connected *config.ConnectedPorts,
) (*FalconMeasurementResponse, error) {
	getter, matches, err := getVoltageGetter(req)
	if err != nil {
		return nil, err
	}
	if !matches {
		return nil, fmt.Errorf("request does not match get_voltage")
	}
	defer getter.Close()

	if measurementDispatcher == nil {
		return nil, fmt.Errorf("measurement dispatcher is nil")
	}
	if connected == nil {
		return nil, fmt.Errorf("connected port catalog is nil")
	}

	resolved, err := connected.ResolveConnectedPort(getter)
	if err != nil {
		return nil, fmt.Errorf("resolve get_voltage getter: %w", err)
	}
	// This script consumes the measured_voltage API output. Reject a different
	// resolved meter before dispatching GET_VOLTAGE to it.
	if !strings.HasSuffix(string(resolved.PortName), "."+getVoltageOutput) {
		return nil, fmt.Errorf(
			"get_voltage requires the %q port, resolved %q",
			getVoltageOutput,
			resolved.PortName,
		)
	}

	target, err := (instrumenttarget.Target{
		Instrument: resolved.InstrumentName,
		Group:      resolved.ChannelGroup,
		Channel:    resolved.Channel,
	}).Serialize()
	if err != nil {
		return nil, fmt.Errorf(
			"serialize get_voltage InstrumentTarget: %w",
			err,
		)
	}

	results := measurementDispatcher.RunAll([]dispatcher.MeasurementRequest{{
		Script: getVoltageHandlerName,
		Variables: []instrumentserver.MeasureVariable{{
			Name: "getter",
			Value: instrumentserver.VariableValue{
				Value: instrumentserver.InstrumentTarget(target),
			},
		}},
	}})
	if len(results) != 1 {
		return nil, fmt.Errorf(
			"get_voltage returned %d measurement results, want 1",
			len(results),
		)
	}

	voltage, err := voltageFromResult(results[0], resolved, getter)
	if err != nil {
		return nil, err
	}

	return newScalarMeasurementResponse(voltage, getter)
}
