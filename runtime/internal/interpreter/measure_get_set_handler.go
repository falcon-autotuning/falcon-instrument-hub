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
	measureGetSetHandlerName = "measure_get_set"
	measureSetInput          = "voltage"
	measureGetOutput         = "voltage"
)

// measureGetSetHandler sets one source voltage and then reads one meter value.
// It translates a Falcon waveform request into arguments for the Lua script.
type measureGetSetHandler struct{}

// Name takes no arguments and returns the handler's identifier, which is also
// the Lua script name used by Handle.
func (*measureGetSetHandler) Name() string { return measureGetSetHandlerName }

// measureGetSetRequest is the single-point form of a parsed voltage waveform.
type measureGetSetRequest struct {
	*measureVoltageWaveformRequest
	voltage float64 // The one requested source voltage, in volts.
}

// measureVoltageWaveformRequest owns the ports and voltage setpoints shared by
// scalar and sequential 1D measurements. Setpoints are converted to volts.
type measureVoltageWaveformRequest struct {
	setter   *instrumentport.Handle // Source port whose output will be changed.
	getter   *instrumentport.Handle // Meter port whose reading will be returned.
	voltages []float64              // Requested source voltages in sweep order.
}

// Close releases the Falcon port handles owned by r. A nil request or a request
// with missing handles is allowed. It returns nothing and ignores close errors.
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

// parseMeasureGetSetRequest reads req and accepts exactly one source voltage
// and one voltage getter, using the checks in parseVoltageWaveformRequest.
// It only inspects the request; it does not contact instruments.
//
// It returns the parsed request on success, (nil, nil) for an unsupported
// request, or (nil, error) if reading or converting the request fails.
// The caller must Close a successfully returned request.
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

// parseVoltageWaveformRequest extracts source voltages and ports from req for
// the single-point and sequential 1D handlers. It requires one writable DC
// voltage source, one readable voltmeter, and one nonempty 1D setpoint array.
// Both ports must use voltage-compatible units. The waveform transform must
// leave source values unchanged (identity), and meter transforms must be empty.
//
// It returns the parsed ports with source setpoints converted to volts.
// It returns (nil, nil) for an unsupported request, or (nil, error) for
// missing/unreadable data, conversion failures, or non-finite setpoints.
// The caller must Close the returned request.
// This function does not resolve physical channels or contact instruments.
func parseVoltageWaveformRequest(
	req *FalconMeasurementRequest,
) (*measureVoltageWaveformRequest, error) {
	if req == nil || req.Handle() == nil {
		return nil, fmt.Errorf("request is nil")
	}

	candidate := &measureVoltageWaveformRequest{}
	// Release any ports acquired before a failed check. On success, ownership
	// passes to the caller instead.
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

	// These handlers use projected voltages directly, so they cannot apply a
	// waveform transform that changes the requested source values.
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

	// Project the waveform onto its source port to obtain actual setpoints in
	// that port's units. Convert them to volts before passing them to Lua.
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

// CanHandle gives the router a broad match for req without contacting hardware.
// It returns (true, nil) when there is one getter, one waveform, and one setter;
// (false, nil) for other counts; or (false, error) if the request cannot be read.
//
// A true result does not guarantee Handle will accept the request: this check
// does not validate point count, units, access, instrument types, or transforms.
// More specific handlers, such as the 1D sweep handler, should be tried first.
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

// measureGetSetValue extracts a voltage reading from one Lua execution result.
// result is expected to contain two calls (set, then read), with at least one
// return value on the second call. The return must be named "voltage" and hold
// a number or a DataBuffer. The second call's return count is not checked here.
//
// setter is currently unused. getter identifies the meter channel, and connected
// supplies its available settings for checking multi-sample buffers. getterPort
// supplies the requested output units and acquisition metadata.
// measurementDispatcher consumes and releases buffers owned by result.ID.
//
// It returns the sample average in getterPort's units, or (0, error) if execution,
// sample validation, unit conversion, or the database insert fails. The database
// write is currently incomplete: it passes an empty DatabaseEntry, not samples.
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
	// Prepare metadata in the instrument's original units. The intended raw
	// sample storage below is still incomplete; only the converted average is
	// returned to the caller.
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

// newMeasureGetSetResponse wraps one measured value in a Falcon response.
// value must already be in getter's units; getter supplies the measurement
// label and units. It returns a response containing a one-element array, or
// (nil, error) if construction fails. The caller must Close the response.
func newMeasureGetSetResponse(
	value float64,
	getter *instrumentport.Handle,
) (*FalconMeasurementResponse, error) {
	return newVoltageArrayResponse([]float64{value}, getter)
}

// newVoltageArrayResponse builds one labelled 1D measurement array from values,
// preserving their order (one reading per setpoint for a sweep). values must be
// nonempty and already expressed in getter's units; no conversion is done here.
// getter supplies the acquisition label, instrument type, and units.
//
// It returns a Falcon response with array shape [len(values)], or (nil, error)
// for empty input or construction failure. The caller must Close the response;
// this function does not close the supplied getter.
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

// Handle executes a single-point request: set the source voltage, read the
// meter, process the reading, and record the source's new state.
//
// req supplies the waveform and getter. measurementDispatcher runs the Lua
// script and manages returned buffers. connected maps the requested ports to
// instrument names, channel groups, and channel numbers. tracker records the
// applied source voltage. The WireMap argument and recorder are currently
// unused; measureGetSetValue attempts its database write directly instead.
//
// Lua receives setter and getter InstrumentTargets plus voltage in volts.
// On success, this returns a one-element response in the getter's units; the
// caller must Close it. Unsupported requests or failures return (nil, error).
// An error after script execution does not undo instrument changes.
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

	// Lua uses concrete instrument/channel targets rather than Falcon ports.
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
