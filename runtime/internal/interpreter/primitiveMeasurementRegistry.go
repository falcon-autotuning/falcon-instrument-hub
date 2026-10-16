package interpreter

import (
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
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
		Choice  Dimensionality
	}
	PrimitiveMeasurementOutput struct {
		Getters map[Target]RawData
	}
)

type PrimitiveEncoder func(
	input PrimitiveMeasurementInput,
) ([]instrumentserver.MeasureVariable, error)

type PrimitiveDecoder func(
	output []float64,
) (PrimitiveMeasurementOutput, error)

type PrimitiveMeasurement struct {
	script  string
	encoder PrimitiveEncoder
	decoder PrimitiveDecoder
}

type Executor interface {
	Run(
		input PrimitiveMeasurementInput,
		// numberOfPoints gets allocated if needed and cached from a setting read request
		numberOfPoints *int,
	) (PrimitiveMeasurementOutput, error)
}

func BuildMeasurementRegistry(dims []Dimensionality, dispatcher MeasurementRunner) (Executor, error)
