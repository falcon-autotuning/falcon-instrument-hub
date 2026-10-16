package cartesianmeasure

import (
	"strings"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestBuildMeasurementRegistryRejectsUnknownDimension(t *testing.T) {
	_, err := BuildMeasurementRegistry(
		[]Dimensionality{3},
		&registryTestRunner{},
	)
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected unknown dimension error, got %v", err)
	}
}

func TestBuildMeasurementRegistryRegistersSchemaCodecs(t *testing.T) {
	_, err := BuildMeasurementRegistry(
		[]Dimensionality{Dim0, Dim1, Dim2},
		&registryTestRunner{},
	)
	if err != nil {
		t.Fatalf("register schema codecs: %v", err)
	}
}

func TestMeasurementRegistryRunsSelectedPrimitive(t *testing.T) {
	target := Target{
		Instrument: "Meter1",
		Group:      "analog",
		Channel:    2,
	}

	runner := &registryTestRunner{
		results: []DispatchResult{
			{
				ID: "measurement-1",
			},
		},
	}

	definitions := map[Dimensionality]PrimitiveMeasurement{
		Dim0: {
			script: "measure_get_set",

			encoder: func(
				input PrimitiveMeasurementInput,
			) ([]instrumentserver.MeasureVariable, error) {
				return []instrumentserver.MeasureVariable{
					{
						Name: "dummy",
						Value: instrumentserver.VariableValue{
							Value: int64(1),
						},
					},
				}, nil
			},

			decoder: func(
				consumer BufferConsumer,
				result DispatchResult,
				input PrimitiveMeasurementInput,
				numberOfPoints NumberOfPoints,
			) (
				PrimitiveMeasurementOutput,
				error,
			) {
				if consumer != nil {
					t.Fatalf(
						"unexpected consumer: %v",
						consumer,
					)
				}

				if result.ID != "measurement-1" {
					t.Fatalf(
						"result id = %q",
						result.ID,
					)
				}

				if numberOfPoints[target] != 3 {
					t.Fatalf(
						"points = %d, want 3",
						numberOfPoints[target],
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
	require.NoError(t, err)

	output, err := executor.Run(
		PrimitiveMeasurementInput{
			Choice: Dim0,
			Getters: []Target{
				target,
			},
		},
		NumberOfPoints{
			target: 3,
		},
	)
	require.NoError(t, err)

	assert.Equal(
		t,
		RawData{0.1, 0.2, 0.3},
		output.Getters[target],
	)

	require.Len(
		t,
		runner.requests,
		1,
	)

	assert.Equal(
		t,
		"measure_get_set",
		runner.requests[0].Script,
	)

	assert.Equal(
		t,
		[]string{"measurement-1"},
		runner.released,
	)
}

func TestBuildMeasurementRegistry_NilDispatcher(
	t *testing.T,
) {
	_, err := BuildMeasurementRegistry(
		[]Dimensionality{Dim0},
		nil,
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"dispatcher is nil",
	)
}

func TestBuildMeasurementRegistry_NoDimensions(
	t *testing.T,
) {
	_, err := BuildMeasurementRegistry(
		nil,
		&registryTestRunner{},
	)

	require.Error(t, err)
}

func TestBuildMeasurementRegistry_DuplicateDimension(
	t *testing.T,
) {
	_, err := BuildMeasurementRegistry(
		[]Dimensionality{
			Dim0,
			Dim0,
		},
		&registryTestRunner{},
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"repeated",
	)
}

func TestMeasurementRegistry_RejectsZeroPoints(
	t *testing.T,
) {
	target := Target{
		Instrument: "Meter",
		Group:      "analog",
		Channel:    1,
	}

	executor, err := buildMeasurementRegistry(
		[]Dimensionality{Dim0},
		&registryTestRunner{},
		map[Dimensionality]PrimitiveMeasurement{
			Dim0: {
				script: "x",
				encoder: func(
					PrimitiveMeasurementInput,
				) ([]instrumentserver.MeasureVariable, error) {
					return nil, nil
				},
				decoder: func(
					BufferConsumer,
					DispatchResult,
					PrimitiveMeasurementInput,
					NumberOfPoints,
				) (
					PrimitiveMeasurementOutput,
					error,
				) {
					return PrimitiveMeasurementOutput{}, nil
				},
			},
		},
	)

	require.NoError(t, err)

	_, err = executor.Run(
		PrimitiveMeasurementInput{
			Choice: Dim0,
			Getters: []Target{
				target,
			},
		},
		NumberOfPoints{
			target: 0,
		},
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"must be positive",
	)
}
