package cartesianmeasure

import (
	"fmt"
	"math"
	"sort"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentdomain"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
)

func variable(name string, value any) instrumentserver.MeasureVariable {
	return instrumentserver.MeasureVariable{
		Name:  name,
		Value: instrumentserver.VariableValue{Value: value},
	}
}

func serializedTarget(target Target) (string, error) {
	return instrumenttarget.Target(target).Serialize()
}

func getterArray(
	getters []Target,
) (instrumentserver.InstrumentTargetArray, error) {
	if len(getters) == 0 {
		return nil, fmt.Errorf("measurement has no getters")
	}
	out := make(instrumentserver.InstrumentTargetArray, 0, len(getters))
	seen := make(map[Target]bool, len(getters))
	for _, getter := range getters {
		if seen[getter] {
			return nil, fmt.Errorf("getter %+v is repeated", getter)
		}
		seen[getter] = true
		encoded, err := serializedTarget(getter)
		if err != nil {
			return nil, fmt.Errorf(
				"serialize getter %+v: %w",
				getter,
				err,
			)
		}
		out = append(out, encoded)
	}
	return out, nil
}

func sortedSetters(
	setters map[Target]Input,
) ([]Target, instrumentserver.InstrumentTargetArray, error) {
	targets := make([]Target, 0, len(setters))
	for target := range setters {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Instrument != targets[j].Instrument {
			return targets[i].Instrument < targets[j].Instrument
		}
		if targets[i].Group != targets[j].Group {
			return targets[i].Group < targets[j].Group
		}
		return targets[i].Channel < targets[j].Channel
	})
	encoded := make(instrumentserver.InstrumentTargetArray, 0, len(targets))
	for _, target := range targets {
		value, err := serializedTarget(target)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"serialize setter %+v: %w",
				target,
				err,
			)
		}
		encoded = append(encoded, value)
	}
	return targets, encoded, nil
}

// axisCounts ensures that coupled setters agree about each sweep's length.
func axisCounts(input PrimitiveMeasurementInput) ([]int, error) {
	counts := make([]int, int(input.Choice))
	for target, setter := range input.Setters {
		if setter.Num == nil {
			if setter.Stop != nil {
				return nil, fmt.Errorf(
					"fixed setter %+v has a stop voltage",
					target,
				)
			}
			continue
		}
		if setter.Stop == nil || setter.Axis >= uint8(len(counts)) ||
			*setter.Num <= 0 {
			return nil, fmt.Errorf(
				"invalid sweep for setter %+v",
				target,
			)
		}
		axis := int(setter.Axis)
		if counts[axis] != 0 && counts[axis] != *setter.Num {
			return nil, fmt.Errorf(
				"axis %d has conflicting step counts",
				axis,
			)
		}
		counts[axis] = *setter.Num
	}
	for axis, count := range counts {
		if count == 0 {
			return nil, fmt.Errorf(
				"axis %d has no swept setter",
				axis,
			)
		}
	}
	return counts, nil
}

func validateEncoder(
	input PrimitiveMeasurementInput,
	dim Dimensionality,
) (instrumentserver.InstrumentTargetArray, []Target, instrumentserver.InstrumentTargetArray, []int, error) {
	if input.Choice != dim {
		return nil, nil, nil, nil, fmt.Errorf(
			"encoder for dimension %d received dimension %d",
			dim,
			input.Choice,
		)
	}
	getters, err := getterArray(input.Getters)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	targets, setters, err := sortedSetters(input.Setters)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	counts, err := axisCounts(input)
	return getters, targets, setters, counts, err
}

// encodeGetSet follows measure_get_set.json. Acquisition settings are read by
// the handler and are deliberately not added to the schema's script arguments.
func encodeGetSet(
	input PrimitiveMeasurementInput,
) ([]instrumentserver.MeasureVariable, error) {
	getters, targets, setters, _, err := validateEncoder(
		input,
		Dim0,
	)
	if err != nil {
		return nil, err
	}
	voltages := make(instrumentserver.DoubleArray, 0, len(targets))
	for _, target := range targets {
		value := input.Setters[target].Start
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf(
				"setter %+v has a non-finite voltage",
				target,
			)
		}
		voltages = append(voltages, value)
	}
	return []instrumentserver.MeasureVariable{
		variable("setVoltages", voltages),
		variable("getters", getters),
		variable("setters", setters),
	}, nil
}

func domainValue(start, stop float64) (string, error) {
	if math.IsNaN(start) || math.IsInf(start, 0) || math.IsNaN(stop) ||
		math.IsInf(stop, 0) {
		return "", fmt.Errorf(
			"domain contains a non-finite voltage",
		)
	}
	domain := instrumentdomain.Domain{Min: start, Max: stop}
	serializedDomain, err := domain.Serialize()
	if err != nil {
		return "", err
	}
	return serializedDomain, nil
}

func encode1DBuffered(
	input PrimitiveMeasurementInput,
) ([]instrumentserver.MeasureVariable, error) {
	getters, targets, setters, counts, err := validateEncoder(
		input,
		Dim1,
	)
	if err != nil {
		return nil, err
	}
	domains := make(instrumentserver.InstrumentDomainArray, 0, len(targets))
	for _, target := range targets {
		setter := input.Setters[target]
		stop := setter.Start
		if setter.Stop != nil {
			stop = *setter.Stop
		}
		value, err := domainValue(setter.Start, stop)
		if err != nil {
			return nil, fmt.Errorf("setter %+v: %w", target, err)
		}
		domains = append(domains, value)
	}
	return []instrumentserver.MeasureVariable{
		variable("setVoltageDomains", domains),
		variable("getters", getters),
		variable("numSteps", int64(counts[0])),
		variable("setters", setters),
	}, nil
}

func encode2DBuffered(
	input PrimitiveMeasurementInput,
) ([]instrumentserver.MeasureVariable, error) {
	getters, targets, setters, counts, err := validateEncoder(
		input,
		Dim2,
	)
	if err != nil {
		return nil, err
	}
	xDomains := make(instrumentserver.InstrumentDomainArray, 0)
	yDomains := make(instrumentserver.InstrumentDomainArray, 0)
	xTargets := make(instrumentserver.InstrumentTargetArray, 0)
	yTargets := make(instrumentserver.InstrumentTargetArray, 0)
	for i, target := range targets {
		setter := input.Setters[target]
		if setter.Num == nil || setter.Stop == nil {
			return nil, fmt.Errorf(
				"2D schema has no fixed setter field for %+v",
				target,
			)
		}
		value, err := domainValue(setter.Start, *setter.Stop)
		if err != nil {
			return nil, fmt.Errorf("setter %+v: %w", target, err)
		}
		switch setter.Axis {
		case 0:
			xTargets = append(xTargets, setters[i])
			xDomains = append(xDomains, value)
		case 1:
			yTargets = append(yTargets, setters[i])
			yDomains = append(yDomains, value)
		}
	}
	return []instrumentserver.MeasureVariable{
		variable("bufferedXSetters", xTargets),
		variable("bufferedGetters", getters),
		variable("bufferedYSetters", yTargets),
		variable("numXSteps", int64(counts[0])),
		variable("setYVoltageDomains", yDomains),
		variable("setXVoltageDomains", xDomains),
		variable("numYSteps", int64(counts[1])),
	}, nil
}

// decodePrimitive expects contiguous samples per waveform position. The ISS
// script must emit positions in the same axis order as the planner's shape.
func decodePrimitive(
	consumer BufferConsumer,
	result DispatchResult,
	input PrimitiveMeasurementInput,
	points NumberOfPoints,
) (PrimitiveMeasurementOutput, error) {
	for _, target := range input.Getters {
		if points[target] < 0 {
			return PrimitiveMeasurementOutput{}, fmt.Errorf(
				"number of samples cannot be negative",
			)
		}
	}
	counts, err := axisCounts(input)
	if err != nil {
		return PrimitiveMeasurementOutput{}, err
	}
	specs := make(
		[]TargetSeriesSpec,
		0,
		len(input.Getters),
	)
	if len(input.Getters) == 0 {
		return PrimitiveMeasurementOutput{}, fmt.Errorf(
			"measurement has no getters",
		)
	}
	for _, getter := range input.Getters {
		// points defaults to 0 to indicate not read from the server
		numPoints := points[getter]
		if points[getter] == 0 {
			numPoints = 1
		}
		shape := append(counts, numPoints)
		unit, ok := input.GetterUnits[getter]
		if !ok {
			return PrimitiveMeasurementOutput{}, fmt.Errorf(
				"getter %+v has no unit",
				getter,
			)
		}
		specs = append(specs, TargetSeriesSpec{
			Target:      getter,
			Unit:        unit,
			Shape:       shape,
			BuffersOnly: input.Choice != Dim0,
		})
	}
	series, err := DecodeTargets(consumer, result, specs)
	if err != nil {
		return PrimitiveMeasurementOutput{}, err
	}
	out := PrimitiveMeasurementOutput{
		Getters: make(map[Target]RawData, len(series)),
	}
	for _, item := range series {
		out.Getters[Target(item.Target)] = RawData(item.Data)
	}
	return out, nil
}
