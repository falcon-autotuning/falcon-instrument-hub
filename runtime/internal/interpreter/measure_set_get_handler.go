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
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/port-transforms/porttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesinstrumentport"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/interpreter/commands"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdb"
)

const (
	measureGetSetHandlerName = "measure_set_get"
	measureSetVoltageCommand = "SET_VOLTAGE"
	measureGetCommand        = "GET_DATAPOINT"
	measureSetInput          = "voltage"
	measureGetOutput         = "voltage"
)

type measureGetSetHandler struct{}

func (*measureGetSetHandler) Name() string { return measureGetSetHandlerName }

type measureGetSetRequest struct {
	*measureVoltageWaveformRequest
	voltage float64
}

// measureVoltageWaveformRequest owns the ports and voltage setpoints shared by
// scalar and sequential 1D measurements. Setpoints are converted to volts.
type measureVoltageWaveformRequest struct {
	setter   *instrumentport.Handle
	getter   *instrumentport.Handle
	voltages []float64
}

func (r *measureVoltageWaveformRequest) Close() {
	if r == nil {
		return
	}
	if r.setter != nil {
		r.setter.Close()
	}
	if r.getter != nil {
		r.getter.Close()
	}
}

// parseMeasureGetSetRequest recognizes the first supported combined workflow:
// one identity-transformed voltage setpoint and one scalar voltage getter.
func parseMeasureGetSetRequest(
	req *FalconMeasurementRequest,
) (*measureGetSetRequest, error) {
	parsed, err := parseVoltageWaveformRequest(req)
	if err != nil || parsed == nil {
		return nil, err
	}
	if len(parsed.voltages) != 1 {
		parsed.Close()
		return nil, nil
	}
	return &measureGetSetRequest{
		measureVoltageWaveformRequest: parsed,
		voltage:                       parsed.voltages[0],
	}, nil
}

// parseVoltageWaveformRequest validates one identity-transformed voltage
// waveform and one voltage getter, without restricting the setpoint count.
func parseVoltageWaveformRequest(
	req *FalconMeasurementRequest,
) (*measureVoltageWaveformRequest, error) {
	if req == nil || req.Handle() == nil {
		return nil, fmt.Errorf("request is nil")
	}

	candidate := &measureVoltageWaveformRequest{}
	success := false
	defer func() {
		if !success {
			candidate.Close()
		}
	}()

	meterTransforms, err := req.Handle().MeterTransforms()
	if err != nil {
		return nil, fmt.Errorf("read request meter transforms: %w", err)
	}
	defer meterTransforms.Close()
	meterTransformCount, err := meterTransforms.Size()
	if err != nil {
		return nil, fmt.Errorf(
			"read request meter-transform count: %w",
			err,
		)
	}
	if meterTransformCount != 0 {
		return nil, nil
	}

	getters, err := req.Handle().Getters()
	if err != nil {
		return nil, fmt.Errorf("read request getters: %w", err)
	}
	defer getters.Close()
	getterCount, err := getters.Size()
	if err != nil {
		return nil, fmt.Errorf("read request getter count: %w", err)
	}
	if getterCount != 1 {
		return nil, nil
	}
	candidate.getter, err = getters.At(0)
	if err != nil {
		return nil, fmt.Errorf("read request getter: %w", err)
	}

	isMeter, err := candidate.getter.IsMeter()
	if err != nil {
		return nil, fmt.Errorf("read getter port type: %w", err)
	}
	if !isMeter {
		return nil, nil
	}
	getterAccess, err := candidate.getter.Access()
	if err != nil {
		return nil, fmt.Errorf("read getter access: %w", err)
	}
	if getterAccess != access.Read && getterAccess != access.Readwrite {
		return nil, nil
	}
	getterInstrumentType, err := candidate.getter.InstrumentType()
	if err != nil {
		return nil, fmt.Errorf("read getter instrument type: %w", err)
	}
	if getterInstrumentType != instrument.Voltmeter {
		return nil, nil
	}

	getterUnits, err := candidate.getter.Units()
	if err != nil {
		return nil, fmt.Errorf("read getter units: %w", err)
	}
	defer getterUnits.Close()
	volts, err := SymbolUnitFromString("V")
	if err != nil {
		return nil, fmt.Errorf("create voltage unit: %w", err)
	}
	defer volts.Close()
	compatible, err := getterUnits.IsCompatibleWith(volts)
	if err != nil {
		return nil, fmt.Errorf(
			"compare getter units with volts: %w",
			err,
		)
	}
	if !compatible {
		return nil, nil
	}

	waveforms, err := req.Handle().Waveforms()
	if err != nil {
		return nil, fmt.Errorf("read request waveforms: %w", err)
	}
	defer waveforms.Close()
	waveformCount, err := waveforms.Size()
	if err != nil {
		return nil, fmt.Errorf("read request waveform count: %w", err)
	}
	if waveformCount != 1 {
		return nil, nil
	}
	waveform, err := waveforms.At(0)
	if err != nil {
		return nil, fmt.Errorf("read request waveform: %w", err)
	}
	defer waveform.Close()

	space, err := waveform.Space()
	if err != nil {
		return nil, fmt.Errorf("read waveform space: %w", err)
	}
	defer space.Close()
	knobs, err := space.Knobs()
	if err != nil {
		return nil, fmt.Errorf("read waveform knobs: %w", err)
	}
	defer knobs.Close()
	knobCount, err := knobs.Size()
	if err != nil {
		return nil, fmt.Errorf("read waveform knob count: %w", err)
	}
	if knobCount != 1 {
		return nil, nil
	}
	candidate.setter, err = knobs.At(0)
	if err != nil {
		return nil, fmt.Errorf("read waveform knob: %w", err)
	}

	isKnob, err := candidate.setter.IsKnob()
	if err != nil {
		return nil, fmt.Errorf("read setter port type: %w", err)
	}
	if !isKnob {
		return nil, nil
	}
	setterAccess, err := candidate.setter.Access()
	if err != nil {
		return nil, fmt.Errorf("read setter access: %w", err)
	}
	if setterAccess != access.Write && setterAccess != access.Readwrite {
		return nil, nil
	}
	setterInstrumentType, err := candidate.setter.InstrumentType()
	if err != nil {
		return nil, fmt.Errorf("read setter instrument type: %w", err)
	}
	if setterInstrumentType != instrument.DcVoltageSource {
		return nil, nil
	}
	setterUnits, err := candidate.setter.Units()
	if err != nil {
		return nil, fmt.Errorf("read setter units: %w", err)
	}
	defer setterUnits.Close()
	compatible, err = setterUnits.IsCompatibleWith(volts)
	if err != nil {
		return nil, fmt.Errorf(
			"compare setter units with volts: %w",
			err,
		)
	}
	if !compatible {
		return nil, nil
	}

	transforms, err := waveform.Transforms()
	if err != nil {
		return nil, fmt.Errorf("read waveform transforms: %w", err)
	}
	defer transforms.Close()
	transformCount, err := transforms.Size()
	if err != nil {
		return nil, fmt.Errorf("read waveform transform count: %w", err)
	}
	if transformCount != 1 {
		return nil, nil
	}
	transform, err := transforms.At(0)
	if err != nil {
		return nil, fmt.Errorf("read waveform transform: %w", err)
	}
	defer transform.Close()
	identity, err := porttransform.NewIdentityTransform(candidate.setter)
	if err != nil {
		return nil, fmt.Errorf("create identity transform: %w", err)
	}
	defer identity.Close()
	isIdentity, err := transform.Equal(identity)
	if err != nil {
		return nil, fmt.Errorf("compare waveform transform: %w", err)
	}
	if !isIdentity {
		return nil, nil
	}

	axes, err := axesinstrumentport.New(
		[]*instrumentport.Handle{candidate.setter},
	)
	if err != nil {
		return nil, fmt.Errorf("create setter projection axes: %w", err)
	}
	defer axes.Close()
	projection, err := space.GetProjection(axes)
	if err != nil {
		return nil, fmt.Errorf("project waveform setpoints: %w", err)
	}
	defer projection.Close()
	projectionCount, err := projection.Size()
	if err != nil {
		return nil, fmt.Errorf(
			"read waveform projection count: %w",
			err,
		)
	}
	if projectionCount != 1 {
		return nil, nil
	}
	setpoints, err := projection.At(0)
	if err != nil {
		return nil, fmt.Errorf("read waveform setpoint array: %w", err)
	}
	defer setpoints.Close()
	dimension, err := setpoints.Dimension()
	if err != nil {
		return nil, fmt.Errorf("read waveform dimension: %w", err)
	}
	if dimension != 1 {
		return nil, nil
	}
	values, err := setpoints.Data()
	if err != nil {
		return nil, fmt.Errorf("read waveform setpoints: %w", err)
	}
	if len(values) == 0 {
		return nil, nil
	}
	candidate.voltages = make([]float64, len(values))
	for i, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf(
				"waveform setpoint %d is not finite",
				i,
			)
		}
		candidate.voltages[i], err = setterUnits.ConvertValueTo(
			value,
			volts,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"convert waveform setpoint %d to volts: %w",
				i,
				err,
			)
		}
		if math.IsNaN(candidate.voltages[i]) ||
			math.IsInf(candidate.voltages[i], 0) {
			return nil, fmt.Errorf(
				"converted voltage setpoint %d is not finite",
				i,
			)
		}
	}
	success = true
	return candidate, nil
}

func (*measureGetSetHandler) CanHandle(
	req *FalconMeasurementRequest,
) (bool, error) {
	if req == nil || req.Handle() == nil {
		return false, fmt.Errorf("request is nil")
	}

	getters, err := req.Handle().Getters()
	if err != nil {
		return false, fmt.Errorf("read request getters: %w", err)
	}
	defer getters.Close()

	getterCount, err := getters.Size()
	if err != nil {
		return false, fmt.Errorf("read request getter count: %w", err)
	}
	if getterCount != 1 {
		return false, nil
	}

	waveforms, err := req.Handle().Waveforms()
	if err != nil {
		return false, fmt.Errorf("read request waveforms: %w", err)
	}
	defer waveforms.Close()

	waveformCount, err := waveforms.Size()
	if err != nil {
		return false, fmt.Errorf("read request waveform count: %w", err)
	}
	if waveformCount != 1 {
		return false, nil
	}

	waveform, err := waveforms.At(0)
	if err != nil {
		return false, fmt.Errorf("read request waveform: %w", err)
	}
	defer waveform.Close()

	space, err := waveform.Space()
	if err != nil {
		return false, fmt.Errorf("read waveform space: %w", err)
	}
	defer space.Close()

	setters, err := space.Knobs()
	if err != nil {
		return false, fmt.Errorf("read waveform setters: %w", err)
	}
	defer setters.Close()

	setterCount, err := setters.Size()
	if err != nil {
		return false, fmt.Errorf("read waveform setter count: %w", err)
	}

	return setterCount == 1, nil
}

func validateMeasureGetSetCall(
	call instrumentserver.CallResult,
	expected config.ConnectedPort,
	command string,
	returnCount int,
) error {
	if call.Instrument != expected.InstrumentName ||
		call.Group != expected.ChannelGroup ||
		call.Channel != int64(expected.Channel) ||
		call.Verb != command {
		return fmt.Errorf(
			"unexpected %s result identity: instrument=%q group=%q channel=%d command=%q",
			measureGetSetHandlerName,
			call.Instrument,
			call.Group,
			call.Channel,
			call.Verb,
		)
	}
	if len(call.Return) != returnCount {
		return fmt.Errorf(
			"%s %s returned %d values, want %d",
			measureGetSetHandlerName,
			command,
			len(call.Return),
			returnCount,
		)
	}
	return nil
}

func measureGetSetValue(
	result dispatcher.MeasurementResult,
	setter config.ConnectedPort,
	getter config.ConnectedPort,
	getterPort *instrumentport.Handle,
	connected *config.ConnectedPorts,
	measurementDispatcher *dispatcher.MeasurementDispatcher,
) (float64, error) {
	if result.Err != nil {
		return 0, result.Err
	}
	if len(result.Results) != 2 {
		return 0, fmt.Errorf(
			"%s returned %d instrument calls, want 2",
			measureGetSetHandlerName,
			len(result.Results),
		)
	}
	if err := validateMeasureGetSetCall(
		result.Results[0],
		setter,
		measureSetVoltageCommand,
		0,
	); err != nil {
		return 0, err
	}
	if err := validateMeasureGetSetCall(
		result.Results[1],
		getter,
		measureGetCommand,
		1,
	); err != nil {
		return 0, err
	}

	measured := result.Results[1].Return[0]
	if measured.Name != measureGetOutput {
		return 0, fmt.Errorf(
			"%s returned output %q, want %q",
			measureGetSetHandlerName,
			measured.Name,
			measureGetOutput,
		)
	}

	var samples []float64
	switch raw := measured.Value.Value.(type) {
	case float64:
		samples = []float64{raw}
	case int64:
		samples = []float64{float64(raw)}
	case instrumentserver.DataBuffer:
		var err error
		samples, err = measurementDispatcher.ConsumeMeasurementSamples(
			result.ID,
			string(raw),
		)
		if err != nil {
			return 0, fmt.Errorf(
				"consume measurement buffer %q: %w",
				raw,
				err,
			)
		}
		// A single sample needs no sampling settings. A bin of multiple
		// samples requires the getter's sample_rate and API bins
		// settings.
		if len(samples) > 1 {
			if err := commands.Validate(
				measured.Value,
				getter,
				connected,
			); err != nil {
				return 0, err
			}
		}
	default:
		return 0, fmt.Errorf(
			"%s returned unsupported value type %T",
			measureGetSetHandlerName,
			measured.Value.Value,
		)
	}
	if len(samples) == 0 {
		return 0, fmt.Errorf("measurement contains no samples")
	}
	var value float64
	for i, sample := range samples {
		if math.IsNaN(sample) || math.IsInf(sample, 0) {
			return 0, fmt.Errorf(
				"measurement sample %d is not finite",
				i,
			)
		}
		// Divide before summing to avoid overflowing a large raw sum.
		value += sample / float64(len(samples))
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("measurement average is not finite")
	}

	resultUnits, err := SymbolUnitFromString(measured.Unit)
	if err != nil {
		return 0, fmt.Errorf(
			"read measurement result units %q: %w",
			measured.Unit,
			err,
		)
	}
	defer resultUnits.Close()
	getterUnits, err := getterPort.Units()
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
			"measurement result unit %q is incompatible with the requested getter",
			measured.Unit,
		)
	}

	converted, err := resultUnits.ConvertValueTo(value, getterUnits)
	if err != nil {
		return 0, fmt.Errorf(
			"convert measurement result to getter units: %w",
			err,
		)
	}
	if math.IsNaN(converted) || math.IsInf(converted, 0) {
		return 0, fmt.Errorf(
			"converted measurement result is not finite",
		)
	}
	// Preserve raw samples in their ISS unit; the average is only returned.
	connection, err := getterPort.PseudoName()
	if err != nil {
		return 0, fmt.Errorf(
			"read getter connection for storage: %w",
			err,
		)
	}
	defer connection.Close()
	instrumentType, err := getterPort.InstrumentType()
	if err != nil {
		return 0, err
	}
	context, err := acquisitioncontext.New(
		connection,
		instrumentType,
		resultUnits,
	)
	if err != nil {
		return 0, fmt.Errorf("create raw measurement context: %w", err)
	}
	defer context.Close()
	// TODO: Fix populating the measurement database
	if _, err := measurementdb.Insert(measurementdb.InsertEntry{
		CoreEntry: measurementdb.CoreEntry{
			MeasurementID:   result.ID,
			MeasurementName: measureGetSetHandlerName,
		},
		Shape: []int{1, len(samples)},
	}, falconcore.DatabaseEntry{}); err != nil {
		return 0, fmt.Errorf("store set/get measurement: %w", err)
	}
	return converted, nil
}

func newMeasureGetSetResponse(
	value float64,
	getter *instrumentport.Handle,
) (*FalconMeasurementResponse, error) {
	return newVoltageArrayResponse([]float64{value}, getter)
}

// newVoltageArrayResponse preserves one returned value per sweep setpoint.
func newVoltageArrayResponse(
	values []float64,
	getter *instrumentport.Handle,
) (*FalconMeasurementResponse, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("measurement response is empty")
	}
	context, err := acquisitioncontext.NewFromPort(getter)
	if err != nil {
		return nil, fmt.Errorf(
			"create acquisition context from getter: %w",
			err,
		)
	}
	defer context.Close()
	data, err := farraydouble.FromData(
		values,
		[]uint64{uint64(len(values))},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create measurement data: %w",
			err,
		)
	}
	defer data.Close()
	array, err := labelledmeasuredarray.FromFArray(data, context)
	if err != nil {
		return nil, fmt.Errorf("label measurement data: %w", err)
	}
	defer array.Close()
	list, err := listlabelledmeasuredarray.New(
		[]*labelledmeasuredarray.Handle{array},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create measurement array list: %w",
			err,
		)
	}
	defer list.Close()
	arrays, err := labelledarrayslabelledmeasuredarray.NewFromList(list)
	if err != nil {
		return nil, fmt.Errorf(
			"create labelled measurement arrays: %w",
			err,
		)
	}
	defer arrays.Close()
	response, err := measurementresponse.New(arrays)
	if err != nil {
		return nil, fmt.Errorf(
			"create MeasurementResponse: %w",
			err,
		)
	}
	return &FalconMeasurementResponse{handle: response}, nil
}

func (*measureGetSetHandler) Handle(
	req *FalconMeasurementRequest,
	measurementDispatcher *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	connected *config.ConnectedPorts,
	tracker StateUpdater,
	recorder MeasurementRecorder,
) (*FalconMeasurementResponse, error) {
	parsed, err := parseMeasureGetSetRequest(req)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, fmt.Errorf(
			"request does not match %s",
			measureGetSetHandlerName,
		)
	}
	defer parsed.Close()
	if tracker == nil {
		return nil, fmt.Errorf("state updater is nil")
	}
	if measurementDispatcher == nil {
		return nil, fmt.Errorf("measurement dispatcher is nil")
	}
	if connected == nil {
		return nil, fmt.Errorf("connected port catalog is nil")
	}

	setter, err := connected.ResolveConnectedPort(parsed.setter)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve measure_set_get setter: %w",
			err,
		)
	}
	if !strings.HasSuffix(string(setter.PortName), "."+measureSetInput) {
		return nil, fmt.Errorf(
			"measure_set_get requires the voltage setter port, resolved %q",
			setter.PortName,
		)
	}
	getter, err := connected.ResolveConnectedPort(parsed.getter)
	if err != nil {
		return nil, fmt.Errorf(
			"resolve measure_set_get getter: %w",
			err,
		)
	}
	if !strings.HasSuffix(string(getter.PortName), "."+measureGetOutput) {
		return nil, fmt.Errorf(
			"measure_set_get requires the scalar voltage getter port, resolved %q",
			getter.PortName,
		)
	}

	setterTarget, err := (instrumenttarget.Target{
		Instrument: setter.InstrumentName,
		Group:      setter.ChannelGroup,
		Channel:    setter.Channel,
	}).Serialize()
	if err != nil {
		return nil, fmt.Errorf(
			"serialize measure_get_set setter target: %w",
			err,
		)
	}
	getterTarget, err := (instrumenttarget.Target{
		Instrument: getter.InstrumentName,
		Group:      getter.ChannelGroup,
		Channel:    getter.Channel,
	}).Serialize()
	if err != nil {
		return nil, fmt.Errorf(
			"serialize measure_get_set getter target: %w",
			err,
		)
	}

	results := measurementDispatcher.RunAll(
		[]dispatcher.MeasurementRequest{{
			Script: measureGetSetHandlerName,
			Variables: []instrumentserver.MeasureVariable{
				{
					Name: "setter",
					Value: instrumentserver.VariableValue{
						Value: instrumentserver.InstrumentTarget(
							setterTarget,
						),
					},
				},
				{
					Name: "getter",
					Value: instrumentserver.VariableValue{
						Value: instrumentserver.InstrumentTarget(
							getterTarget,
						),
					},
				},
				{
					Name: "voltage",
					Value: instrumentserver.VariableValue{
						Value: parsed.voltage,
					},
				},
			},
		}},
	)
	for _, result := range results {
		defer measurementDispatcher.ReleaseMeasurementBuffers(result.ID)
	}
	if len(results) != 1 {
		return nil, fmt.Errorf(
			"measure_get_set returned %d measurement results, want 1",
			len(results),
		)
	}

	value, err := measureGetSetValue(
		results[0],
		setter,
		getter,
		parsed.getter,
		connected,
		measurementDispatcher,
	)
	if err != nil {
		return nil, err
	}
	if err := tracker.UpdatePort(setter.DeviceName, falconcore.Quantity{
		Value: parsed.voltage,
		Unit:  falconcore.Volt,
	}); err != nil {
		return nil, fmt.Errorf("update setter state: %w", err)
	}
	return newMeasureGetSetResponse(value, parsed.getter)
}
