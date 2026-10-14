//go:build cgo

package interpreter

import (
	"fmt"
	"math"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/port-transforms/porttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretespace"
)

// measureGetSetRequest owns the ports for one DC measurement. voltages[i] is
// the fixed source voltage in volts for setters[i]; getters retain request
// order.
type measureGetSetRequest struct {
	setters  []*instrumentport.Handle
	getters  []*instrumentport.Handle
	voltages []float64
}

// Close releases the parsed ports. It returns nothing and accepts a nil
// receiver.
func (r *measureGetSetRequest) Close() {
	if r == nil {
		return
	}
	for _, port := range append(r.setters, r.getters...) {
		port.Close()
	}
}

// fixedSourceVoltage reads a knob's domain without constructing a control
// array.
// Equal, included bounds mean a fixed DC voltage, even if the normalized grid
// contains several coordinates. The returned voltage is in V. A varying domain
// returns (0, false, nil); malformed domains or conversion failures return
// errors.
func fixedSourceVoltage(
	space *discretespace.Handle,
	port *instrumentport.Handle,
) (float64, bool, error) {
	domain, err := space.GetDomain(port)
	if err != nil {
		return 0, false, err
	}
	defer domain.Close()
	low, err := domain.LesserBound()
	if err != nil {
		return 0, false, err
	}
	high, err := domain.GreaterBound()
	if err != nil {
		return 0, false, err
	}
	if math.IsNaN(low) || math.IsInf(low, 0) || math.IsNaN(high) ||
		math.IsInf(high, 0) {
		return 0, false, fmt.Errorf(
			"source domain bounds must be finite",
		)
	}
	if low != high {
		return 0, false, nil
	}
	empty, err := domain.IsEmpty()
	if err != nil {
		return 0, false, err
	}
	if empty {
		return 0, false, fmt.Errorf(
			"fixed source domain must include its voltage",
		)
	}
	units, err := port.Units()
	if err != nil {
		return 0, false, err
	}
	defer units.Close()
	volts, err := SymbolUnitFromString("V")
	if err != nil {
		return 0, false, err
	}
	defer volts.Close()
	value, err := units.ConvertValueTo(low, volts)
	if err != nil {
		return 0, false, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false, fmt.Errorf("source voltage must be finite")
	}
	return value, true, nil
}

// isSetGetVoltagePort checks the command capabilities supported by this script.
// setter selects a writable DC source; otherwise port must be a readable
// voltmeter. Both need voltage-compatible units. Unsupported ports return
// false.
func isSetGetVoltagePort(
	port *instrumentport.Handle,
	setter bool,
) (bool, error) {
	portAccess, err := port.Access()
	if err != nil {
		return false, err
	}
	kind, err := port.InstrumentType()
	if err != nil {
		return false, err
	}
	var roleOK bool
	if setter {
		roleOK, err = port.IsKnob()
		roleOK = roleOK && kind == instrument.DcVoltageSource &&
			(portAccess == access.Write || portAccess == access.Readwrite)
	} else {
		roleOK, err = port.IsMeter()
		roleOK = roleOK && kind == instrument.Voltmeter &&
			(portAccess == access.Read || portAccess == access.Readwrite)
	}
	if err != nil || !roleOK {
		return false, err
	}
	units, err := port.Units()
	if err != nil {
		return false, err
	}
	defer units.Close()
	volts, err := SymbolUnitFromString("V")
	if err != nil {
		return false, err
	}
	defer volts.Close()
	return units.IsCompatibleWith(volts)
}

// parseMeasureGetSetRequest recognizes one identity-transformed waveform with
// one or more fixed source voltages and readable voltmeters. Meter transforms
// are not supported. It only inspects req; it does not contact instruments.
// It returns (nil, nil) for an unsupported request, or an error for unreadable
// or malformed data. The caller must Close a successfully returned request.
func parseMeasureGetSetRequest(
	req *FalconMeasurementRequest,
) (*measureGetSetRequest, error) {
	if req == nil || req.Handle() == nil {
		return nil, fmt.Errorf("request is nil")
	}
	parsed := &measureGetSetRequest{}
	success := false
	defer func() {
		if !success {
			parsed.Close()
		}
	}()
	meters, err := req.Handle().MeterTransforms()
	if err != nil {
		return nil, err
	}
	defer meters.Close()
	n, err := meters.Size()
	if err != nil || n != 0 {
		return nil, err
	}
	getters, err := req.Handle().Getters()
	if err != nil {
		return nil, err
	}
	defer getters.Close()
	getterCount, err := getters.Size()
	if err != nil || getterCount == 0 {
		return nil, err
	}
	for i := uint64(0); i < getterCount; i++ {
		getter, err := getters.At(i)
		if err != nil {
			return nil, err
		}
		parsed.getters = append(parsed.getters, getter)
		ok, err := isSetGetVoltagePort(getter, false)
		if err != nil || !ok {
			return nil, err
		}
	}
	waveforms, err := req.Handle().Waveforms()
	if err != nil {
		return nil, err
	}
	defer waveforms.Close()
	n, err = waveforms.Size()
	if err != nil || n != 1 {
		return nil, err
	}
	waveform, err := waveforms.At(0)
	if err != nil {
		return nil, err
	}
	defer waveform.Close()
	space, err := waveform.Space()
	if err != nil {
		return nil, err
	}
	defer space.Close()
	knobs, err := space.Knobs()
	if err != nil {
		return nil, err
	}
	defer knobs.Close()
	knobCount, err := knobs.Size()
	if err != nil || knobCount == 0 {
		return nil, err
	}
	transforms, err := waveform.Transforms()
	if err != nil {
		return nil, err
	}
	defer transforms.Close()
	n, err = transforms.Size()
	if err != nil || n != knobCount {
		return nil, err
	}
	for i := uint64(0); i < knobCount; i++ {
		setter, err := knobs.At(i)
		if err != nil {
			return nil, err
		}
		parsed.setters = append(parsed.setters, setter)
		ok, err := isSetGetVoltagePort(setter, true)
		if err != nil || !ok {
			return nil, err
		}
		identity, err := porttransform.NewIdentityTransform(setter)
		if err != nil {
			return nil, err
		}
		ok, err = transforms.Contains(identity)
		identity.Close()
		if err != nil || !ok {
			return nil, err
		}
		voltage, fixed, err := fixedSourceVoltage(space, setter)
		if err != nil || !fixed {
			return nil, err
		}
		parsed.voltages = append(parsed.voltages, voltage)
	}
	success = true
	return parsed, nil
}
