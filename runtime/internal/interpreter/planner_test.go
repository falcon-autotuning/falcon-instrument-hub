//go:build cgo

package interpreter

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTarget(
	instrument string,
	channel int,
) Target {
	return Target{
		Instrument: instrument,
		Group:      "analog",
		Channel:    channel,
	}
}

func targetA() Target {
	return testTarget("DAC", 0)
}

func targetB() Target {
	return testTarget("DAC", 1)
}

func targetC() Target {
	return testTarget("DAC", 2)
}

type mockExecutor struct {
	inputs []PrimitiveMeasurementInput

	output PrimitiveMeasurementOutput
	err    error
}

func (m *mockExecutor) Run(
	input PrimitiveMeasurementInput,
	numberOfPoints *int,
) (PrimitiveMeasurementOutput, error) {
	m.inputs = append(
		m.inputs,
		input,
	)

	return m.output, m.err
}

var _ Executor = (*mockExecutor)(nil)

func intPtr(v int) *int {
	return &v
}

func TestNativePlan_Execute(t *testing.T) {
	input := PrimitiveMeasurementInput{
		Choice: Dim1,
	}

	expected := PrimitiveMeasurementOutput{
		Getters: map[Target]RawData{
			targetA(): {1, 2, 3},
		},
	}

	exec := &mockExecutor{
		output: expected,
	}

	plan := &NativePlan{
		Input: input,
	}

	out, err := plan.Execute(exec, nil)

	require.NoError(t, err)

	assert.Equal(t, expected, out)

	require.Len(t, exec.inputs, 1)
	assert.Equal(t, input, exec.inputs[0])
}

func TestNativePlan_Execute_Error(t *testing.T) {
	expectedErr := errors.New("run failed")

	exec := &mockExecutor{
		err: expectedErr,
	}

	plan := &NativePlan{
		Input: PrimitiveMeasurementInput{
			Choice: Dim0,
		},
	}

	_, err := plan.Execute(exec, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
}

func TestNativePlan_Shape_Dim0(t *testing.T) {
	plan := &NativePlan{
		Input: PrimitiveMeasurementInput{
			Setters: map[Target]Input{
				targetA(): {
					Start: 1,
				},
			},
		},
	}

	assert.Empty(t, plan.Shape())
}

func TestNativePlan_Shape_Dim1(t *testing.T) {
	plan := &NativePlan{
		Input: PrimitiveMeasurementInput{
			Setters: map[Target]Input{
				targetA(): {
					Axis:  0,
					Num:   intPtr(101),
					Start: 0,
				},
			},
		},
	}

	assert.Equal(
		t,
		Shape{101},
		plan.Shape(),
	)
}

func TestNativePlan_Shape_CollapsesDuplicateAxes(t *testing.T) {
	plan := &NativePlan{
		Input: PrimitiveMeasurementInput{
			Setters: map[Target]Input{
				targetA(): {
					Axis: 0,
					Num:  intPtr(51),
				},
				targetB(): {
					Axis: 0,
					Num:  intPtr(51),
				},
				targetC(): {
					Axis: 1,
					Num:  intPtr(101),
				},
			},
		},
	}

	assert.ElementsMatch(
		t,
		Shape{51, 101},
		plan.Shape(),
	)
}

func TestMergePrimitiveMeasurementOutputs_Empty(t *testing.T) {
	out := mergePrimitiveMeasurementOutputs(
		PrimitiveMeasurementOutput{},
		PrimitiveMeasurementOutput{},
	)

	assert.Empty(t, out.Getters)
}

func TestMergePrimitiveMeasurementOutputs_NewKey(t *testing.T) {
	total := PrimitiveMeasurementOutput{}

	update := PrimitiveMeasurementOutput{
		Getters: map[Target]RawData{
			targetA(): {1, 2, 3},
		},
	}

	out := mergePrimitiveMeasurementOutputs(
		total,
		update,
	)

	assert.Equal(
		t,
		RawData{1, 2, 3},
		out.Getters[targetA()],
	)
}

func TestMergePrimitiveMeasurementOutputs_Appends(t *testing.T) {
	total := PrimitiveMeasurementOutput{
		Getters: map[Target]RawData{
			targetA(): {1, 2},
		},
	}

	update := PrimitiveMeasurementOutput{
		Getters: map[Target]RawData{
			targetA(): {3, 4},
		},
	}

	out := mergePrimitiveMeasurementOutputs(
		total,
		update,
	)

	assert.Equal(
		t,
		RawData{1, 2, 3, 4},
		out.Getters[targetA()],
	)
}

func TestNestedPlan_Shape(t *testing.T) {
	plan := &NestedPlan{
		shape: Shape{51, 101},
	}

	assert.Equal(
		t,
		Shape{51, 101},
		plan.Shape(),
	)
}

type mockDimensions struct {
	dims []Dimensionality
	err  error
}

func (m *mockDimensions) AvailableDimensions() (
	[]Dimensionality,
	error,
) {
	return m.dims, m.err
}

func TestBuildPlan_NoWaveforms(
	t *testing.T,
) {
	plan, err := BuildPlan(
		MeasurementRequest{},
		nil,
		[]Dimensionality{
			Dim0,
		},
	)

	require.Error(t, err)
	assert.Nil(t, plan)
}

func validCompiledUnitSpace(
	t *testing.T,
	discretizers []falconcore.Discretizer,
	domain falconcore.Domain,
) falconcore.UnitSpace {
	t.Helper()

	raw, err := (falconcore.UnitSpace{
		Axes:   discretizers,
		Domain: domain,
	}).NewFalconUnitSpace()
	require.NoError(t, err)

	out, err := falconcore.UnitSpaceFromFalcon(raw)
	require.NoError(t, err)

	return out
}

func valid1DSweepRequest(
	t *testing.T,
) MeasurementRequest {
	var req MeasurementRequest
	req.Getters = falconcore.Ports{
		{
			DefaultName:    "dumb-long-name",
			InstrumentName: "ADC",
			InstrumentType: instrument.Amnmeter,
			Description:    "test",
			Unit:           falconcore.Ampere,
			Access:         access.Read,
			Scope:          scope.Local,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
			PortType:       porttype.PortTypeMeter,
			PseudoName: falconcore.Connection{
				Name: falconcore.ConnectionName("O1"),
				Type: falconcore.Ohmic,
			},
		},
	}
	req.Waveforms = []falconcore.Waveform{
		{
			Space: falconcore.DiscreteSpace{
				Space: validCompiledUnitSpace(t,
					[]falconcore.Discretizer{
						{
							Type:  falconcore.CartesianDiscretization,
							Delta: 0.5,
						},
					},
					falconcore.Domain{
						LesserBound:  0,
						GreaterBound: 10,
					},
				),
				Axes: falconcore.AxesCoupledLabelledDomain{
					falconcore.CoupledLabelledDomain{
						falconcore.LabelledDomain{
							InstrumentPort: falconcore.InstrumentPort{
								DefaultName:    "other-long-name",
								InstrumentName: "DAC1",
								InstrumentType: instrument.VoltageSource,
								Description:    "test",
								Unit:           falconcore.Volt,
								Access:         access.Readwrite,
								Scope:          scope.Local,
								Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
								PortType:       porttype.PortTypeKnob,
								PseudoName: falconcore.Connection{
									Name: falconcore.ConnectionName("P1"),
									Type: falconcore.PlungerGate,
								},
							},
							Domain: falconcore.Domain{
								LesserBound:  -1,
								GreaterBound: 1,
							},
						},
					},
				},
				Increasing: falconcore.Increasing{
					{
						"P1": true,
					},
				},
			},
		},
	}
	return req
}

func valid1DConnectedPorts() *config.ConnectedPorts {
	meter := config.ConnectedPort{
		PortEntry: config.PortEntry{
			InstrumentName: "ADC",
			ChannelGroup:   "analog",
			Channel:        0,
			InstrumentType: instrument.Amnmeter,
			Role:           porttype.PortTypeMeter,
			Access:         access.Read,
			Scope:          scope.Local,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
			Unit:           falconcore.Ampere,
			Description:    "test meter",
		},
		DeviceName: "O1",
		PortName:   "dumb-long-name",
		Handle: falconcore.Connection{
			Name: "O1",
			Type: falconcore.Ohmic,
		},
	}

	knob := config.ConnectedPort{
		PortEntry: config.PortEntry{
			InstrumentName: "DAC1",
			ChannelGroup:   "analog",
			Channel:        1,
			InstrumentType: instrument.VoltageSource,
			Role:           porttype.PortTypeKnob,
			Access:         access.Readwrite,
			Scope:          scope.Local,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
			Unit:           falconcore.Volt,
			Description:    "test knob",
		},
		DeviceName: "P1",
		PortName:   "other-long-name",
		Handle: falconcore.Connection{
			Name: "P1",
			Type: falconcore.PlungerGate,
		},
	}

	return &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			meter,
			knob,
		},
		Meters: []config.ConnectedPort{
			meter,
		},
		Knobs: []config.ConnectedPort{
			knob,
		},
	}
}

func TestBuildPlan_NativePlan(
	t *testing.T,
) {
	req := valid1DSweepRequest(t)

	plan, err := BuildPlan(
		req,
		valid1DConnectedPorts(),
		[]Dimensionality{
			Dim1,
		},
	)

	require.NoError(t, err)

	native, ok := plan.(*NativePlan)
	require.True(t, ok)

	assert.Equal(
		t,
		Dim1,
		native.Input.Choice,
	)

	assert.Len(
		t,
		native.Input.Getters,
		1,
	)

	assert.Len(
		t,
		native.Input.Setters,
		1,
	)
}

func TestBuildPlan_NestedPlan(
	t *testing.T,
) {
	req := valid1DSweepRequest(t)

	plan, err := BuildPlan(
		req,
		valid1DConnectedPorts(),
		[]Dimensionality{
			Dim0,
		},
	)

	require.NoError(t, err)

	nested, ok := plan.(*NestedPlan)
	require.True(t, ok)

	require.NotEmpty(
		t,
		nested.Children,
	)

	for _, child := range nested.Children {
		_, ok := child.(*NativePlan)
		assert.True(t, ok)
	}
}

func TestBuildPlan_NestedPlanShape(
	t *testing.T,
) {
	req := valid1DSweepRequest(t)

	plan, err := BuildPlan(
		req,
		valid1DConnectedPorts(),
		[]Dimensionality{
			Dim0,
		},
	)

	require.NoError(t, err)

	assert.NotEmpty(
		t,
		plan.Shape(),
	)
}

func TestBuildPlan_FixedAxesPropagated(
	t *testing.T,
) {
	req := valid1DSweepRequest(t)

	plan, err := BuildPlan(
		req,
		valid1DConnectedPorts(),
		[]Dimensionality{
			Dim0,
		},
	)

	require.NoError(t, err)

	nested := plan.(*NestedPlan)

	require.NotEmpty(
		t,
		nested.Children,
	)

	native := nested.Children[0].(*NativePlan)

	require.Len(
		t,
		native.FixedAxes,
		1,
	)

	_, ok := native.FixedAxes[0]
	assert.True(t, ok)
}

func TestNestedPlan_Execute(
	t *testing.T,
) {
	plan := &NestedPlan{
		Children: []Plan{
			&NativePlan{
				Input: PrimitiveMeasurementInput{},
			},
			&NativePlan{
				Input: PrimitiveMeasurementInput{},
			},
		},
	}

	exec := &mockExecutor{
		output: PrimitiveMeasurementOutput{
			Getters: map[Target]RawData{
				targetA(): {1},
			},
		},
	}

	out, err := plan.Execute(exec, nil)

	require.NoError(t, err)

	assert.Equal(
		t,
		RawData{1, 1},
		out.Getters[targetA()],
	)
}

func TestNestedPlan_Execute_Error(
	t *testing.T,
) {
	expected := errors.New("boom")

	plan := &NestedPlan{
		Children: []Plan{
			&NativePlan{
				Input: PrimitiveMeasurementInput{},
			},
		},
	}

	exec := &mockExecutor{
		err: expected,
	}

	_, err := plan.Execute(exec, nil)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
}

func testKnob(
	defaultName string,
	instrumentName string,
	pseudo string,
) falconcore.InstrumentPort {
	return falconcore.InstrumentPort{
		DefaultName:    falconcore.PortName(defaultName),
		InstrumentName: falconcore.InstrumentName(instrumentName),
		InstrumentType: instrument.VoltageSource,
		Description:    "test",
		Unit:           falconcore.Volt,
		Access:         access.Readwrite,
		Scope:          scope.Local,
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
		PortType:       porttype.PortTypeKnob,
		PseudoName: falconcore.Connection{
			Name: falconcore.ConnectionName(pseudo),
			Type: falconcore.PlungerGate,
		},
	}
}

func valid2DSweepRequest(
	t *testing.T,
) MeasurementRequest {
	return MeasurementRequest{
		Getters: falconcore.Ports{
			{
				DefaultName:    "adc",
				InstrumentName: "ADC",
				InstrumentType: instrument.Amnmeter,
				Unit:           falconcore.Ampere,
				Access:         access.Read,
				Scope:          scope.Local,
				PortType:       porttype.PortTypeMeter,
				PseudoName: falconcore.Connection{
					Name: "O1",
					Type: falconcore.Ohmic,
				},
			},
		},
		Waveforms: []falconcore.Waveform{
			{
				Space: falconcore.DiscreteSpace{
					Space: validCompiledUnitSpace(
						t,
						[]falconcore.Discretizer{
							{
								Type:  falconcore.CartesianDiscretization,
								Delta: 0.1,
							},
							{
								Type:  falconcore.CartesianDiscretization,
								Delta: 0.1,
							},
						},
						falconcore.Domain{
							LesserBound:  0,
							GreaterBound: 2,
						},
					),
					Axes: falconcore.AxesCoupledLabelledDomain{
						{
							{
								InstrumentPort: testKnob(
									"dac1",
									"DAC1",
									"P1",
								),
								Domain: falconcore.Domain{
									LesserBound:  -1,
									GreaterBound: 1,
								},
							},
							{
								InstrumentPort: testKnob(
									"dac2",
									"DAC2",
									"P2",
								),
								Domain: falconcore.Domain{
									LesserBound:  10,
									GreaterBound: 20,
								},
							},
						},
						{
							{
								InstrumentPort: testKnob(
									"dac3",
									"DAC3",
									"P3",
								),
								Domain: falconcore.Domain{
									LesserBound:  50,
									GreaterBound: 60,
								},
							},
						},
					},
					Increasing: falconcore.Increasing{
						{
							"P1": true,
							"P2": true,
						},
						{
							"P3": true,
						},
					},
				},
			},
		},
	}
}

func valid2DConnectedPorts() *config.ConnectedPorts {
	makeKnob := func(
		name string,
		device string,
		channel int,
	) config.ConnectedPort {
		return config.ConnectedPort{
			PortEntry: config.PortEntry{
				InstrumentName: name,
				ChannelGroup:   "analog",
				Channel:        channel,
				InstrumentType: instrument.VoltageSource,
				Role:           porttype.PortTypeKnob,
				Access:         access.Readwrite,
				Scope:          scope.Local,
				Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
				Unit:           falconcore.Volt,
			},
			PortName: config.PortName(
				"dac" + string(rune('0'+channel)),
			),
			DeviceName: falconcore.ConnectionName(device),
			Handle: falconcore.Connection{
				Name: falconcore.ConnectionName(device),
				Type: falconcore.PlungerGate,
			},
		}
	}

	meter := config.ConnectedPort{
		PortEntry: config.PortEntry{
			InstrumentName: "ADC",
			ChannelGroup:   "analog",
			Channel:        0,
			InstrumentType: instrument.Amnmeter,
			Role:           porttype.PortTypeMeter,
			Access:         access.Read,
			Scope:          scope.Local,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
			Unit:           falconcore.Ampere,
		},
		PortName:   "adc",
		DeviceName: "O1",
		Handle: falconcore.Connection{
			Name: "O1",
			Type: falconcore.Ohmic,
		},
	}

	p1 := makeKnob("DAC1", "P1", 1)
	p1.PortName = "dac1"

	p2 := makeKnob("DAC2", "P2", 2)
	p2.PortName = "dac2"

	p3 := makeKnob("DAC3", "P3", 3)
	p3.PortName = "dac3"

	return &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			meter,
			p1,
			p2,
			p3,
		},
		Meters: []config.ConnectedPort{
			meter,
		},
		Knobs: []config.ConnectedPort{
			p1,
			p2,
			p3,
		},
	}
}

func TestBuildPlan_2DTo1D(
	t *testing.T,
) {
	req := valid2DSweepRequest(t)

	plan, err := BuildPlan(
		req,
		valid2DConnectedPorts(),
		[]Dimensionality{
			Dim1,
		},
	)

	require.NoError(t, err)

	nested := plan.(*NestedPlan)

	require.NotEmpty(t, nested.Children)

	for _, child := range nested.Children {
		_, ok := child.(*NativePlan)
		assert.True(t, ok)
	}
}

func TestBuildPlan_RecursiveDecomposition(
	t *testing.T,
) {
	req := valid2DSweepRequest(t)

	plan, err := BuildPlan(
		req,
		valid2DConnectedPorts(),
		[]Dimensionality{
			Dim0,
		},
	)

	require.NoError(t, err)

	level1 := plan.(*NestedPlan)

	require.NotEmpty(
		t,
		level1.Children,
	)

	level2 := level1.Children[0].(*NestedPlan)

	require.NotEmpty(
		t,
		level2.Children,
	)

	leaf := level2.Children[0].(*NativePlan)

	assert.Equal(
		t,
		Dim0,
		leaf.Input.Choice,
	)

	assert.Len(
		t,
		leaf.FixedAxes,
		2,
	)
}
