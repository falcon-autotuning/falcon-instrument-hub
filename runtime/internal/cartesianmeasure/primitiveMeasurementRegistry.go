package cartesianmeasure

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdecode"
)

type (
	DispatchRequest = dispatcher.MeasurementRequest
	DispatchResult  = dispatcher.MeasurementResult
)

type Dimensionality uint8

const (
	Dim0 Dimensionality = iota
	Dim1
	Dim2
)

type (
	Target  instrumenttarget.Target
	RawData []float64
	Input   struct {
		Axis  uint8
		Start float64
		Stop  *float64 // nil means "not provided"
		Num   *int     // nil means "not provided"
	}
	PrimitiveMeasurementInput struct {
		Setters map[Target]Input
		Getters []Target
		// GetterUnits is keyed by the same physical targets as Getters.
		// The decoder converts ISS return values into these Falcon port
		// units.
		GetterUnits map[Target]falconcore.Unit
		Choice      Dimensionality
	}
	PrimitiveMeasurementOutput struct {
		Getters map[Target]RawData
	}
)

type PrimitiveEncoder func(
	input PrimitiveMeasurementInput,
	numberOfPoints int,
) ([]instrumentserver.MeasureVariable, error)

type PrimitiveDecoder func(
	consumer measurementdecode.BufferConsumer,
	result DispatchResult,
	input PrimitiveMeasurementInput,
	numberOfPoints int,
) (PrimitiveMeasurementOutput, error)

type PrimitiveMeasurement struct {
	script  string
	encoder PrimitiveEncoder
	decoder PrimitiveDecoder
}

// Add script-specific encoders and decoders here as their Lua contracts are
// finalized. The names must match the script files searched by the dispatcher.
// A definition without both hooks is rejected by BuildMeasurementRegistry.
var primitiveMeasurements = map[Dimensionality]PrimitiveMeasurement{
	Dim0: {
		script:  "measure_get_set",
		encoder: encodeGetSet,
		decoder: decodePrimitive,
	},
	Dim1: {
		script:  "measure_1D_buffered",
		encoder: encode1DBuffered,
		decoder: decodePrimitive,
	},
	Dim2: {
		script:  "measure_2D_buffered",
		encoder: encode2DBuffered,
		decoder: decodePrimitive,
	},
}

type Executor interface {
	Run(
		input PrimitiveMeasurementInput,
		// A nil numberOfPoints uses one sample per position. Resolve
		// and cache the instrument setting before calling Run when more
		// are required.
		numberOfPoints *int,
	) (PrimitiveMeasurementOutput, error)
}

type measurementRegistry struct {
	measurements map[Dimensionality]PrimitiveMeasurement
	dispatcher   MeasurementRunner
}

var _ Executor = (*measurementRegistry)(nil)

// BuildMeasurementRegistry selects the requested primitive scripts and checks
// that each can encode inputs and decode ISS results. It does not run a script.
func BuildMeasurementRegistry(
	dims []Dimensionality,
	dispatcher MeasurementRunner,
) (Executor, error) {
	return buildMeasurementRegistry(dims, dispatcher, primitiveMeasurements)
}

// buildMeasurementRegistry accepts definitions separately so the registration
// rules can be tested without changing the production script table.
func buildMeasurementRegistry(
	dims []Dimensionality,
	dispatcher MeasurementRunner,
	definitions map[Dimensionality]PrimitiveMeasurement,
) (Executor, error) {
	if dispatcher == nil {
		return nil, fmt.Errorf("measurement dispatcher is nil")
	}
	if len(dims) == 0 {
		return nil, fmt.Errorf("no measurement dimensions requested")
	}

	selected := make(map[Dimensionality]PrimitiveMeasurement, len(dims))
	for _, dim := range dims {
		if _, duplicate := selected[dim]; duplicate {
			return nil, fmt.Errorf(
				"measurement dimension %d is repeated",
				dim,
			)
		}
		measurement, registered := definitions[dim]
		if !registered {
			return nil, fmt.Errorf(
				"measurement dimension %d is not registered",
				dim,
			)
		}
		if measurement.script == "" || measurement.encoder == nil ||
			measurement.decoder == nil {
			return nil, fmt.Errorf(
				"measurement dimension %d has no complete script, encoder, and decoder",
				dim,
			)
		}
		selected[dim] = measurement
	}

	return &measurementRegistry{
		measurements: selected,
		dispatcher:   dispatcher,
	}, nil
}

// Run executes exactly one registered primitive measurement. The decoder may
// consume buffers through consumer; any remaining buffers are released when
// the dispatcher supports measurement buffer cleanup.
func (r *measurementRegistry) Run(
	input PrimitiveMeasurementInput,
	numberOfPoints *int,
) (PrimitiveMeasurementOutput, error) {
	measurement, ok := r.measurements[input.Choice]
	if !ok {
		return PrimitiveMeasurementOutput{}, fmt.Errorf(
			"measurement dimension %d is not available",
			input.Choice,
		)
	}
	points := 1
	if numberOfPoints != nil {
		points = *numberOfPoints
	}
	if points <= 0 {
		return PrimitiveMeasurementOutput{}, fmt.Errorf(
			"number of points must be positive, got %d", points,
		)
	}

	variables, err := measurement.encoder(input, points)
	if err != nil {
		return PrimitiveMeasurementOutput{}, fmt.Errorf(
			"encode dimension %d: %w", input.Choice, err,
		)
	}
	results := r.dispatcher.RunAll([]DispatchRequest{{
		Script:    measurement.script,
		Variables: variables,
	}})
	if releaser, ok := r.dispatcher.(interface{ ReleaseMeasurementBuffers(string) }); ok {
		for _, result := range results {
			defer releaser.ReleaseMeasurementBuffers(result.ID)
		}
	}
	if len(results) != 1 {
		return PrimitiveMeasurementOutput{}, fmt.Errorf(
			"script %q returned %d jobs, want 1",
			measurement.script,
			len(results),
		)
	}
	result := results[0]
	if result.Err != nil {
		return PrimitiveMeasurementOutput{}, result.Err
	}

	consumer, _ := r.dispatcher.(measurementdecode.BufferConsumer)
	output, err := measurement.decoder(consumer, result, input, points)
	if err != nil {
		return PrimitiveMeasurementOutput{}, fmt.Errorf(
			"decode dimension %d: %w", input.Choice, err,
		)
	}
	return output, nil
}
