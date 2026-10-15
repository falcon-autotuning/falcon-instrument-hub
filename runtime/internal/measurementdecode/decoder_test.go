//go:build cgo

package measurementdecode

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
)

type bufferConsumer map[string][]float64

func (c bufferConsumer) ConsumeMeasurementSamples(
	measurementID string,
	bufferID string,
) ([]float64, error) {
	if measurementID != "measurement-1" {
		return nil, fmt.Errorf(
			"unexpected measurement ID %q",
			measurementID,
		)
	}
	values, ok := c[bufferID]
	if !ok {
		return nil, fmt.Errorf("unknown buffer %q", bufferID)
	}
	return append([]float64(nil), values...), nil
}

func TestDecodeSupportedLayouts(t *testing.T) {
	target := Target{Instrument: "Meter1", Group: "analog", Channel: 2}
	call := func(value any) instrumentserver.CallResult {
		return instrumentserver.CallResult{
			Instrument: target.Instrument,
			Group:      target.Group,
			Channel:    target.Channel,
			Verb:       "MEASURE_STREAM",
			Return: []instrumentserver.ReturnValue{
				{
					Name: "voltage",
					Unit: "V",
					Value: instrumentserver.VariableValue{
						Value: value,
					},
				},
			},
		}
	}

	tests := []struct {
		name    string
		calls   []instrumentserver.CallResult
		buffers bufferConsumer
		shape   []int
		want    []float64
	}{
		{
			name:  "single scalar",
			calls: []instrumentserver.CallResult{call(0.25)},
			shape: []int{1},
			want:  []float64{0.25},
		},
		{
			name: "one-sample buffer per sweep point",
			calls: []instrumentserver.CallResult{
				call(instrumentserver.DataBuffer("b1")),
				call(instrumentserver.DataBuffer("b2")),
			},
			buffers: bufferConsumer{"b1": {0.1}, "b2": {0.2}},
			shape:   []int{2},
			want:    []float64{0.1, 0.2},
		},
		{
			name: "one full-sweep buffer",
			calls: []instrumentserver.CallResult{
				call(instrumentserver.DataBuffer("b1")),
			},
			buffers: bufferConsumer{"b1": {0.1, 0.2}},
			shape:   []int{2},
			want:    []float64{0.1, 0.2},
		},
		{
			name: "one packed sweep-by-sample buffer",
			calls: []instrumentserver.CallResult{
				call(instrumentserver.DataBuffer("b1")),
			},
			buffers: bufferConsumer{"b1": {0.1, 0.2, 0.3, 0.4}},
			shape:   []int{2, 2},
			want:    []float64{0.1, 0.2, 0.3, 0.4},
		},
		{
			name: "buffer array",
			calls: []instrumentserver.CallResult{call(
				instrumentserver.DataBufferArray{"b1", "b2"},
			)},
			buffers: bufferConsumer{
				"b1": {0.1, 0.2},
				"b2": {0.3, 0.4},
			},
			shape: []int{2, 2},
			want:  []float64{0.1, 0.2, 0.3, 0.4},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := dispatcher.MeasurementResult{
				ID:      "measurement-1",
				Results: test.calls,
			}
			got, err := Decode(test.buffers, result, []SeriesSpec{{
				Target:     target,
				Verb:       "MEASURE_STREAM",
				ReturnName: "voltage",
				Unit:       falconcore.Volt,
				Shape:      test.shape,
			}})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 {
				t.Fatalf("got %d series, want 1", len(got))
			}
			if !reflect.DeepEqual(got[0].Data, test.want) {
				t.Fatalf(
					"data = %v, want %v",
					got[0].Data,
					test.want,
				)
			}
			if !reflect.DeepEqual(got[0].Shape, test.shape) {
				t.Fatalf(
					"shape = %v, want %v",
					got[0].Shape,
					test.shape,
				)
			}
		})
	}
}

func TestDecodeRejectsSampleCountThatDoesNotMatchShape(t *testing.T) {
	target := Target{Instrument: "Meter1", Group: "analog", Channel: 2}
	result := dispatcher.MeasurementResult{
		ID: "measurement-1",
		Results: []instrumentserver.CallResult{{
			Instrument: target.Instrument,
			Group:      target.Group,
			Channel:    target.Channel,
			Verb:       "MEASURE_STREAM",
			Return: []instrumentserver.ReturnValue{{
				Name: "voltage",
				Unit: "V",
				Value: instrumentserver.VariableValue{
					Value: instrumentserver.DataBuffer(
						"b1",
					),
				},
			}},
		}},
	}

	_, err := Decode(
		bufferConsumer{"b1": {0.1, 0.2, 0.3}},
		result,
		[]SeriesSpec{{
			Target:     target,
			Verb:       "MEASURE_STREAM",
			ReturnName: "voltage",
			Unit:       falconcore.Volt,
			Shape:      []int{2, 2},
		}},
	)
	if err == nil {
		t.Fatal("Decode succeeded with the wrong sample count")
	}
}

func TestDecodeConvertsReturnUnits(t *testing.T) {
	target := Target{Instrument: "Meter1", Group: "analog", Channel: 2}
	result := dispatcher.MeasurementResult{
		ID: "measurement-1",
		Results: []instrumentserver.CallResult{{
			Instrument: target.Instrument,
			Group:      target.Group,
			Channel:    target.Channel,
			Verb:       "GET_DATAPOINT",
			Return: []instrumentserver.ReturnValue{
				{
					Name: "voltage",
					Unit: "mV",
					Value: instrumentserver.VariableValue{
						Value: 250.0,
					},
				},
			},
		}},
	}

	got, err := Decode(nil, result, []SeriesSpec{{
		Target:     target,
		Verb:       "GET_DATAPOINT",
		ReturnName: "voltage",
		Unit:       falconcore.Volt,
		Shape:      []int{1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Data[0] != 0.25 {
		t.Fatalf("converted value = %v V, want 0.25 V", got[0].Data[0])
	}
}
