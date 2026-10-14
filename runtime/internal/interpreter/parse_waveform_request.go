//go:build cgo

package interpreter

import (
	"fmt"
	"math"

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
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
)

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
	// Release any ports acquired before a failed check. On success,
	// ownership
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

	volts, err := symbolunit.NewVolt()
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

	// These handlers use projected voltages directly, so they cannot apply
	// a
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

	// A fixed source needs only its domain value. This also lets the 1D
	// handler decline DC requests without trying to project a constant
	// array.
	fixed, isFixed, err := fixedSourceVoltage(space, candidate.setter)
	if err != nil {
		return nil, err
	}
	if isFixed {
		candidate.voltages = []float64{fixed}
		success = true
		return candidate, nil
	}

	// Project the waveform onto its source port to obtain actual setpoints
	// in
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
