package devicestate

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/quantity"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
)

type Quantity struct {
	Value float64
	Unit  Unit
}

func (q Quantity) NewFalconQuantity() (*quantity.Handle, error) {
	unit, err := q.Unit.NewSymbolUnit()
	if err != nil {
		return nil, err
	}
	return quantity.New(q.Value, unit)
}

type QuantityReader interface {
	Value() (float64, error)
	Unit() (*symbolunit.Handle, error)
}

var _ QuantityReader = (*quantity.Handle)(nil)

func QuantityFromFalcon(
	q QuantityReader,
) (Quantity, error) {
	value, err := q.Value()
	if err != nil {
		return Quantity{}, err
	}

	unitHandle, err := q.Unit()
	if err != nil {
		return Quantity{}, err
	}
	unit, err := SymbolFromFalcon(unitHandle)
	if err != nil {
		return Quantity{}, err
	}

	return Quantity{
		Value: value,
		Unit:  unit,
	}, nil
}
