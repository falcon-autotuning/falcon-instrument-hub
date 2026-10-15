package falconcore

import (
	"errors"
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretizer"
)

type DiscretizationType int

const (
	CartesianDiscretization DiscretizationType = iota
	PolarDiscretization
)

type Discretizer struct {
	Type  DiscretizationType
	Delta float64
}

type DiscretizerReader interface {
	Delta() (float64, error)
	IsCartesian() (bool, error)
	IsPolar() (bool, error)
}

var _ DiscretizerReader = (*discretizer.Handle)(nil)

func DiscretizerFromFalcon(
	d DiscretizerReader,
) (Discretizer, error) {
	delta, err := d.Delta()
	if err != nil {
		return Discretizer{}, err
	}

	cartesian, err := d.IsCartesian()
	if err != nil {
		return Discretizer{}, err
	}

	if cartesian {
		return Discretizer{
			Type:  CartesianDiscretization,
			Delta: delta,
		}, nil
	}

	polar, err := d.IsPolar()
	if err != nil {
		return Discretizer{}, err
	}

	if polar {
		return Discretizer{
			Type:  PolarDiscretization,
			Delta: delta,
		}, nil
	}

	return Discretizer{}, errors.New(
		"unknown discretizer type",
	)
}

func (d Discretizer) NewFalconDiscretizer() (
	*discretizer.Handle,
	error,
) {
	switch d.Type {

	case CartesianDiscretization:
		return discretizer.NewCartesianDiscretizer(
			d.Delta,
		)

	case PolarDiscretization:
		return discretizer.NewPolarDiscretizer(
			d.Delta,
		)

	default:
		return nil, fmt.Errorf(
			"unknown discretizer type %d",
			d.Type,
		)
	}
}
