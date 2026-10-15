package interpreter

import "github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"

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

type PrimitiveMeasurement struct{}

type internalRegistry map[Dimensionality]PrimitiveMeasurement

func AddDimensionality(measurement PrimitiveMeasurement) error

func AvailableDimensions() ([]Dimensionality, error)
