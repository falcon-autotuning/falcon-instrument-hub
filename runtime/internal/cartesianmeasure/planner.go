package cartesianmeasure

import (
	"fmt"
	"maps"
	"slices"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
)

type Shape []int64

type Plan interface {
	Execute(
		executor Executor,
		numberOfPoints NumberOfPoints,
	) (PrimitiveMeasurementOutput, error)

	Shape() Shape
}
type GlobalAxis uint8

type FixedAxes map[GlobalAxis]float64

// The leaf nodes of the plan
type NativePlan struct {
	Input     PrimitiveMeasurementInput
	FixedAxes FixedAxes
}

func (p *NativePlan) Execute(
	executor Executor,
	numberOfPoints NumberOfPoints,
) (
	PrimitiveMeasurementOutput,
	error,
) {
	return executor.Run(p.Input, numberOfPoints)
}

func (p *NativePlan) Shape() Shape {
	var out Shape

	counts := make(map[uint8]int)
	for _, setter := range p.Input.Setters {
		if setter.Num == nil {
			continue
		}
		counts[setter.Axis] = *setter.Num
	}
	for axis := uint8(0); axis < uint8(len(counts)); axis++ {
		out = append(out, int64(counts[axis]))
	}

	return out
}

// Appends the newData onto the totalData
func mergePrimitiveMeasurementOutputs(
	total PrimitiveMeasurementOutput,
	newData PrimitiveMeasurementOutput,
) PrimitiveMeasurementOutput {
	if total.Getters == nil {
		total.Getters = make(map[Target]RawData)
	}

	for key, value := range newData.Getters {
		total.Getters[key] = append(
			total.Getters[key],
			value...,
		)
	}

	return total
}

type NestedPlan struct {
	Children []Plan
	shape    Shape
}

func (p *NestedPlan) Shape() Shape {
	return p.shape
}

func (p *NestedPlan) Execute(
	executor Executor,
	numberOfPoints NumberOfPoints,
) (
	PrimitiveMeasurementOutput,
	error,
) {
	var out PrimitiveMeasurementOutput

	for _, child := range p.Children {

		result, err := child.Execute(executor, numberOfPoints)
		if err != nil {
			return PrimitiveMeasurementOutput{}, err
		}

		out = mergePrimitiveMeasurementOutputs(
			out,
			result,
		)
	}

	return out, nil
}

func Supports(
	dimensions []Dimensionality,
	d Dimensionality,
) bool {
	return slices.Contains(dimensions, d)
}

func dimensionality(
	space falconcore.DiscreteSpace,
) Dimensionality {
	return Dimensionality(
		len(space.Axes),
	)
}

func shapeFromSpace(
	space *falconcore.UnitSpace,
	dim Dimensionality,
) (Shape, error) {
	_, err := space.Compile()
	if err != nil {
		return Shape{}, err
	}
	axes := make(falconcore.AxesInt, 0, dim)
	for i := range dim {
		axes = append(axes, int(i))
	}
	arrays, err := space.CreateArray(axes)
	var shape Shape
	for _, array := range arrays {
		shape = append(shape, int64(len(array.Data)))
	}
	return shape, nil
}

func convertToTarget(
	port falconcore.InstrumentPort,
	ports *config.ConnectedPorts,
) (Target, error) {
	if ports == nil {
		return Target{}, fmt.Errorf(
			"connected ports not configured",
		)
	}

	cp, found := settingrouter.ResolvePort(port, ports)
	if !found {
		return Target{}, fmt.Errorf("Could not find the port")
	}
	return Target{
			Instrument: string(cp.InstrumentName),
			Channel:    cp.Channel,
			Group:      cp.ChannelGroup,
		},
		nil
}

func buildGetters(
	getters falconcore.Ports,
	ports *config.ConnectedPorts,
) (
	[]Target,
	map[Target]falconcore.InstrumentPort,
	error,
) {
	targets := make(
		[]Target,
		0,
		len(getters),
	)

	portMap := make(
		map[Target]falconcore.InstrumentPort,
		len(getters),
	)
	for _, port := range getters {
		target, err := convertToTarget(
			port,
			ports,
		)
		if err != nil {
			return nil, nil, err
		}
		targets = append(
			targets,
			target,
		)
		portMap[target] = port
	}

	return targets, portMap, nil
}

func inputFromDomain(
	domain falconcore.Domain,
	increasing bool,
	num int,
	axis uint8,
) Input {
	start := domain.LesserBound
	stop := domain.GreaterBound

	if !increasing {
		start, stop = stop, start
	}

	return Input{
		Axis:  axis,
		Start: start,
		Stop:  &stop,
		Num:   &num,
	}
}

func buildSetters(
	waveform falconcore.Waveform,
	fixed FixedAxes,
	ports *config.ConnectedPorts,
) (map[Target]Input, error) {
	out := make(map[Target]Input)
	// Global axes are the axes in the user waveform.
	//
	// Primitive axes are the axes visible to the executor after
	// fixed dimensions have been removed.
	primitiveAxis := uint8(0)

	for globalAxis, coupled := range waveform.Space.Axes {

		// Fixed axis.
		if value, ok := fixed[GlobalAxis(globalAxis)]; ok {

			for _, labelled := range coupled {

				target, err := convertToTarget(
					labelled.InstrumentPort,
					ports,
				)
				if err != nil {
					return nil, err
				}

				out[target] = Input{
					Start: value,
				}
			}

			continue
		}

		// Swept axis.

		arrays, err := waveform.Space.Space.CreateArray(
			falconcore.AxesInt{
				globalAxis,
			},
		)
		if err != nil {
			return nil, err
		}

		if len(arrays) == 0 {
			return nil, fmt.Errorf(
				"axis %d projection returned no arrays",
				globalAxis,
			)
		}

		num := len(arrays[0].Data)

		for _, labelled := range coupled {

			target, err := convertToTarget(
				labelled.InstrumentPort,
				ports,
			)
			if err != nil {
				return nil, err
			}
			incMap := waveform.Space.Increasing[globalAxis]

			increasing, ok := incMap[string(labelled.InstrumentPort.PseudoName.Name)]
			if !ok {
				return nil, fmt.Errorf(
					"missing increasing information for %q",
					labelled.InstrumentPort,
				)
			}

			out[target] = inputFromDomain(
				labelled.Domain,
				increasing,
				num,
				primitiveAxis,
			)
		}

		primitiveAxis++
	}

	return out, nil
}

func buildPrimitiveInput(
	getters falconcore.Ports,
	waveform falconcore.Waveform,
	fixed FixedAxes,
	ports *config.ConnectedPorts,
) (PrimitiveMeasurementInput, error) {
	getterTargets, getterPorts, err := buildGetters(
		getters,
		ports,
	)
	if err != nil {
		return PrimitiveMeasurementInput{}, err
	}

	setterTargets, err := buildSetters(
		waveform,
		fixed,
		ports,
	)
	if err != nil {
		return PrimitiveMeasurementInput{}, err
	}

	var dim Dimensionality

	seen := make(map[uint8]bool)

	for _, setter := range setterTargets {

		if setter.Num == nil {
			continue
		}

		if !seen[setter.Axis] {
			seen[setter.Axis] = true
			dim++
		}
	}

	getterUnits := make(map[Target]falconcore.Unit, len(getterPorts))
	for target, port := range getterPorts {
		getterUnits[target] = port.Unit
	}
	return PrimitiveMeasurementInput{
		Setters:     setterTargets,
		Getters:     getterTargets,
		GetterUnits: getterUnits,
		Choice:      dim,
	}, nil
}

func axisValues(
	space *falconcore.UnitSpace,
	axis int,
) ([]float64, error) {
	arrays, err := space.CreateArray(
		falconcore.AxesInt{axis},
	)
	if err != nil {
		return nil, err
	}

	if len(arrays) != 1 {
		return nil, fmt.Errorf(
			"expected single axis projection, got %d",
			len(arrays),
		)
	}

	return arrays[0].Data, nil
}

func BuildPlan(
	req MeasurementRequest,
	ports *config.ConnectedPorts,
	dimensions []Dimensionality,
) (Plan, error) {
	return buildPlan(
		req,
		ports,
		nil,
		dimensions,
	)
}

// buildPlan recursively decomposes a measurement request into
// executable plans.
//
// If the requested dimensionality is supported by the executor,
// a NativePlan is produced.
//
// Otherwise one waveform axis is fixed and buildPlan is invoked
// recursively for every point along that axis, producing a
// NestedPlan.
//
// Example:
//
//	2D sweep
//	  P1 ∈ [-1,1]
//	  P2 ∈ [0,10]
//
// with only Dim1 support:
//
//	NestedPlan
//	  NativePlan(P1=-1)
//	  NativePlan(P1= 0)
//	  NativePlan(P1=+1)
func buildPlan(
	req MeasurementRequest,
	ports *config.ConnectedPorts,
	fixed FixedAxes,
	dimensions []Dimensionality,
) (Plan, error) {
	if len(req.Waveforms) == 0 {
		return nil, fmt.Errorf(
			"measurement request contains no waveforms",
		)
	}

	waveform := req.Waveforms[0]

	input, err := buildPrimitiveInput(
		req.Getters,
		waveform,
		fixed,
		ports,
	)
	if err != nil {
		return nil, err
	}

	if Supports(dimensions, input.Choice) {
		return &NativePlan{
			Input:     input,
			FixedAxes: fixed,
		}, nil
	}

	var axisToReduce GlobalAxis
	var found bool

	for i := range waveform.Space.Axes {
		if _, ok := fixed[GlobalAxis(i)]; !ok {
			axisToReduce = GlobalAxis(i)
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf(
			"unable to further decompose measurement",
		)
	}

	values, err := axisValues(
		&waveform.Space.Space,
		int(axisToReduce),
	)
	if err != nil {
		return nil, err
	}

	children := make(
		[]Plan,
		0,
		len(values),
	)

	for _, value := range values {
		childFixed := make(
			FixedAxes,
			len(fixed)+1,
		)
		maps.Copy(childFixed, fixed)
		childFixed[axisToReduce] = value
		child, err := buildPlan(
			req,
			ports,
			childFixed,
			dimensions,
		)
		if err != nil {
			return nil, err
		}
		children = append(
			children,
			child,
		)
	}

	shape, err := shapeFromSpace(
		&waveform.Space.Space,
		dimensionality(waveform.Space),
	)
	if err != nil {
		return nil, err
	}

	return &NestedPlan{
		Children: children,
		shape:    shape,
	}, nil
}
