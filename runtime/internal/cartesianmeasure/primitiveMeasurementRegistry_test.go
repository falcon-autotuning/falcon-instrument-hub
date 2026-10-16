package cartesianmeasure

import (
	"reflect"
	"strings"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdecode"
)

type registryTestRunner struct {
	requests []DispatchRequest
	results  []DispatchResult
	released []string
}

func (r *registryTestRunner) RunAll(
	requests []DispatchRequest,
) []DispatchResult {
	r.requests = append(r.requests, requests...)
	return r.results
}

func (*registryTestRunner) ScriptsPath() string { return "" }

func (r *registryTestRunner) ReleaseMeasurementBuffers(id string) {
	r.released = append(r.released, id)
}

func TestBuildMeasurementRegistryRejectsUnimplementedDimension(t *testing.T) {
	_, err := BuildMeasurementRegistry(
		[]Dimensionality{Dim0},
		&registryTestRunner{},
	)
	if err == nil || !strings.Contains(err.Error(), "no complete script") {
		t.Fatalf("expected incomplete registration error, got %v", err)
	}
}

func TestMeasurementRegistryRunsSelectedPrimitive(t *testing.T) {
	target := Target{Instrument: "Meter1", Group: "analog", Channel: 2}
	runner := &registryTestRunner{
		results: []DispatchResult{{ID: "measurement-1"}},
	}
	definitions := map[Dimensionality]PrimitiveMeasurement{
		Dim0: {
			script: "measure_get_set",
			encoder: func(input PrimitiveMeasurementInput, points int) ([]instrumentserver.MeasureVariable, error) {
				if points != 3 {
					t.Fatalf(
						"encoded point count = %d, want 3",
						points,
					)
				}
				return []instrumentserver.MeasureVariable{
					{
						Name: "numPoints",
						Value: instrumentserver.VariableValue{
							Value: int64(points),
						},
					},
				}, nil
			},
			decoder: func(
				consumer measurementdecode.BufferConsumer,
				result DispatchResult,
				input PrimitiveMeasurementInput,
				points int,
			) (PrimitiveMeasurementOutput, error) {
				if consumer != nil ||
					result.ID != "measurement-1" ||
					points != 3 {
					t.Fatalf(
						"unexpected decoder inputs: consumer=%v result=%v points=%d",
						consumer,
						result,
						points,
					)
				}
				return PrimitiveMeasurementOutput{
					Getters: map[Target]RawData{
						target: {0.1, 0.2, 0.3},
					},
				}, nil
			},
		},
	}
	executor, err := buildMeasurementRegistry(
		[]Dimensionality{Dim0},
		runner,
		definitions,
	)
	if err != nil {
		t.Fatal(err)
	}
	points := 3
	output, err := executor.Run(
		PrimitiveMeasurementInput{Choice: Dim0},
		&points,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := output.Getters[target]; !reflect.DeepEqual(
		got,
		RawData{0.1, 0.2, 0.3},
	) {
		t.Fatalf("readings = %v", got)
	}
	if len(runner.requests) != 1 ||
		runner.requests[0].Script != "measure_get_set" {
		t.Fatalf("dispatched requests = %v", runner.requests)
	}
	if !reflect.DeepEqual(runner.released, []string{"measurement-1"}) {
		t.Fatalf("released buffers = %v", runner.released)
	}
}
