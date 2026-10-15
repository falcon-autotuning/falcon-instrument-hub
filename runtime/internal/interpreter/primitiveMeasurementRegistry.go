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
	Target                    instrumenttarget.Target
	RawData                   []float64
	PrimitiveMeasurementInput struct {
		Setters   []Target
		Getters   map[Target]RawData
		NumPoints int64
		Choice    Dimensionality
	}
	PrimitiveMeasurementOutput struct {
		Setters []map[Target]RawData
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

type internalRegistry map[Dimensionality]PrimitiveMeasurement

func AddDimensionality(measurement PrimitiveMeasurement) error

func AvailableDimensions() ([]Dimensionality, error)

func Run(input PrimitiveMeasurementInput) (PrimitiveMeasurementOutput, error)
