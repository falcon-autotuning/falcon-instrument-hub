package cartesianmeasure

import (
	"reflect"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func variablesByName(vars []instrumentserver.MeasureVariable) map[string]any {
	out := make(map[string]any, len(vars))
	for _, item := range vars {
		out[item.Name] = item.Value.Value
	}
	return out
}

func TestEncodeGetSetPairsTargetsWithVoltages(
	t *testing.T,
) {
	first := Target{
		Instrument: "Source",
		Group:      "analog",
		Channel:    1,
	}
	second := Target{
		Instrument: "Source",
		Group:      "analog",
		Channel:    2,
	}
	getter := Target{
		Instrument: "Meter",
		Group:      "analog",
		Channel:    4,
	}
	vars, err := encodeGetSet(
		PrimitiveMeasurementInput{
			Choice: Dim0,
			Setters: map[Target]Input{
				second: {Start: 2},
				first:  {Start: 1},
			},
			Getters: []Target{
				getter,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	got := variablesByName(vars)
	if len(got) != 3 {
		t.Fatalf("schema variables = %v", got)
	}
	setters := got["setters"].(instrumentserver.InstrumentTargetArray)
	voltages := got["setVoltages"].(instrumentserver.DoubleArray)
	if len(setters) != 2 {
		t.Fatalf("setters = %v", setters)
	}
	if len(voltages) != 2 {
		t.Fatalf(
			"voltages = %v",
			voltages,
		)
	}

	for i, target := range []Target{first, second} {
		encoded, err := serializedTarget(target)
		if err != nil {
			t.Fatal(err)
		}
		if setters[i] != encoded {
			t.Fatalf(
				"setter %d = %q want %q",
				i,
				setters[i],
				encoded,
			)
		}
		if voltages[i] != float64(i+1) {
			t.Fatalf(
				"voltage %d = %v want %v",
				i,
				voltages[i],
				float64(i+1),
			)
		}
	}
	if len(got["getters"].(instrumentserver.InstrumentTargetArray)) != 1 {
		t.Fatalf(
			"getters = %v",
			got["getters"],
		)
	}
}

func TestEncodeBufferedSchemas(t *testing.T) {
	x := Target{Instrument: "Source", Group: "analog", Channel: 1}
	y := Target{Instrument: "Source", Group: "analog", Channel: 2}
	getter := Target{Instrument: "Meter", Group: "analog", Channel: 4}
	xStop, yStop := 1.0, 2.0
	xCount, yCount := 3, 4
	input := PrimitiveMeasurementInput{
		Choice: Dim2,
		Setters: map[Target]Input{
			x: {Axis: 0, Start: 0, Stop: &xStop, Num: &xCount},
			y: {Axis: 1, Start: 0, Stop: &yStop, Num: &yCount},
		},
		Getters: []Target{getter},
	}
	vars, err := encode2DBuffered(input)
	if err != nil {
		t.Fatal(err)
	}
	got := variablesByName(vars)
	if got["numXSteps"] != int64(3) || got["numYSteps"] != int64(4) {
		t.Fatalf("step counts = %v", got)
	}
	if len(got["setXVoltageDomains"].(instrumentserver.InstrumentDomainArray)) != 1 ||
		len(
			got["setYVoltageDomains"].(instrumentserver.InstrumentDomainArray),
		) != 1 {
		t.Fatalf("axis domains = %v", got)
	}
	input.Choice = Dim1
	delete(input.Setters, y)
	vars, err = encode1DBuffered(input)
	if err != nil {
		t.Fatal(err)
	}
	got = variablesByName(vars)
	if got["numSteps"] != int64(3) || len(got) != 4 {
		t.Fatalf("1D schema variables = %v", got)
	}
}

type codecBuffers map[string][]float64

func (b codecBuffers) ConsumeMeasurementSamples(
	id, buffer string,
) ([]float64, error) {
	return append([]float64(nil), b[buffer]...), nil
}

func TestDecodePrimitiveUsesTargetAndShape(t *testing.T) {
	getter := Target{Instrument: "Meter", Group: "analog", Channel: 4}
	stop := 1.0
	steps := 3
	setter := Target{Instrument: "Source", Group: "analog", Channel: 1}
	input := PrimitiveMeasurementInput{
		Choice: Dim1,
		Setters: map[Target]Input{
			setter: {Axis: 0, Start: 0, Stop: &stop, Num: &steps},
		},
		Getters: []Target{getter},
		GetterUnits: map[Target]falconcore.Unit{
			getter: falconcore.Nanoampere,
		},
	}
	result := DispatchResult{
		ID: "measurement-1",
		Results: []instrumentserver.CallResult{{
			Instrument: getter.Instrument,
			Group:      getter.Group,
			Channel:    int64(getter.Channel),
			Verb:       "ACQUIRE",
			Return: []instrumentserver.ReturnValue{
				{
					Name: "samples",
					Unit: "nA",
					Value: instrumentserver.VariableValue{
						Value: instrumentserver.DataBuffer(
							"buffer-1",
						),
					},
				},
			},
		}},
	}
	buffers := codecBuffers{"buffer-1": {1, 2, 3, 4, 5, 6}}
	out, err := decodePrimitive(buffers, result, input,
		NumberOfPoints{
			getter: 2,
		})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.Getters[getter], RawData{1, 2, 3, 4, 5, 6}) {
		t.Fatalf("decoded getter = %v", out.Getters[getter])
	}
	delete(buffers, "buffer-1")
	if _, err := decodePrimitive(buffers, result, input,
		NumberOfPoints{
			getter: 2,
		}); err == nil {
		t.Fatal("expected a shape error for missing samples")
	}
}

func TestDecodePrimitiveZeroAndTwoDimensions(t *testing.T) {
	getter := Target{Instrument: "Meter", Group: "analog", Channel: 1}
	call := instrumentserver.CallResult{
		Instrument: getter.Instrument,
		Group:      getter.Group,
		Channel:    int64(getter.Channel),
		Verb:       "READ",
		Return: []instrumentserver.ReturnValue{{
			Name: "reading", Unit: "nA",
			Value: instrumentserver.VariableValue{Value: 7.0},
		}},
	}
	zero := PrimitiveMeasurementInput{
		Choice:  Dim0,
		Getters: []Target{getter},
		GetterUnits: map[Target]falconcore.Unit{
			getter: falconcore.Nanoampere,
		},
	}
	out, err := decodePrimitive(
		nil,
		DispatchResult{
			Results: []instrumentserver.CallResult{
				call,
			},
		},
		zero,
		NumberOfPoints{
			getter: 0, // Not queried: decode one sample without changing the count.
		},
	)
	if err != nil || !reflect.DeepEqual(out.Getters[getter], RawData{7}) {
		t.Fatalf("0D output = %v, error = %v", out, err)
	}

	x := Target{Instrument: "Source", Group: "analog", Channel: 1}
	y := Target{Instrument: "Source", Group: "analog", Channel: 2}
	xStop, yStop := 1.0, 2.0
	xCount, yCount := 2, 3
	two := PrimitiveMeasurementInput{
		Choice: Dim2,
		Setters: map[Target]Input{
			x: {Axis: 0, Start: 0, Stop: &xStop, Num: &xCount},
			y: {Axis: 1, Start: 0, Stop: &yStop, Num: &yCount},
		},
		Getters: []Target{getter},
		GetterUnits: map[Target]falconcore.Unit{
			getter: falconcore.Nanoampere,
		},
	}
	call.Return[0].Value.Value = instrumentserver.DataBuffer("buffer-2d")
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	out, err = decodePrimitive(
		codecBuffers{"buffer-2d": values},
		DispatchResult{
			ID: "measurement-2",
			Results: []instrumentserver.CallResult{
				call,
			},
		},
		two,
		NumberOfPoints{
			getter: 2,
		},
	)
	if err != nil ||
		!reflect.DeepEqual([]float64(out.Getters[getter]), values) {
		t.Fatalf("2D output = %v, error = %v", out, err)
	}
}

func TestDecodePrimitive_RejectsNegativePoints(
	t *testing.T,
) {
	getter := Target{
		Instrument: "Meter",
		Group:      "analog",
		Channel:    1,
	}

	_, err := decodePrimitive(
		nil,
		DispatchResult{},
		PrimitiveMeasurementInput{
			Choice: Dim0,
			Getters: []Target{
				getter,
			},
			GetterUnits: map[Target]falconcore.Unit{
				getter: falconcore.Nanoampere,
			},
		},
		NumberOfPoints{
			getter: -1,
		},
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"number of samples cannot be negative",
	)
}

func TestDecodePrimitive_MissingGetterUnit(
	t *testing.T,
) {
	getter := Target{
		Instrument: "Meter",
		Group:      "analog",
		Channel:    1,
	}

	_, err := decodePrimitive(
		nil,
		DispatchResult{},
		PrimitiveMeasurementInput{
			Choice: Dim0,
			Getters: []Target{
				getter,
			},
		},
		NumberOfPoints{
			getter: 1,
		},
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"has no unit",
	)
}

func TestDecodePrimitive_NoGetters(
	t *testing.T,
) {
	_, err := decodePrimitive(
		nil,
		DispatchResult{},
		PrimitiveMeasurementInput{
			Choice:      Dim0,
			GetterUnits: map[Target]falconcore.Unit{},
		},
		NumberOfPoints{},
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"measurement has no getters",
	)
}

func TestDecodePrimitive_InvalidAxisCounts(
	t *testing.T,
) {
	getter := Target{
		Instrument: "Meter",
		Group:      "analog",
		Channel:    1,
	}

	stop := 1.0

	_, err := decodePrimitive(
		nil,
		DispatchResult{},
		PrimitiveMeasurementInput{
			Choice: Dim1,
			Getters: []Target{
				getter,
			},
			GetterUnits: map[Target]falconcore.Unit{
				getter: falconcore.Nanoampere,
			},
			Setters: map[Target]Input{
				{
					Instrument: "Source",
					Group:      "analog",
					Channel:    1,
				}: {
					Axis: 0,
					Stop: &stop,
				},
			},
		},
		NumberOfPoints{
			getter: 1,
		},
	)

	require.Error(t, err)
}
