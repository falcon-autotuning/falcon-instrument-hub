package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/controlarray"
)

type ControlArray struct {
	Data  []float64
	Shape []int
}

func (c ControlArray) NewFalconControlArray() (*controlarray.Handle, error) {
	shape := make([]uint64, len(c.Shape))
	for i, index := range c.Shape {
		shape[i] = uint64(index)
	}
	return controlarray.FromData(c.Data, shape)
}

type ArrayReader interface {
	Data() ([]float64, error)
	Shape() ([]uint64, error)
}

var _ ArrayReader = (*controlarray.Handle)(nil)

func ControlArrayFromFalcon(c ArrayReader) (ControlArray, error) {
	var out ControlArray

	var err error
	out.Data, err = c.Data()
	if err != nil {
		return out, err
	}

	shape, err := c.Shape()
	if err != nil {
		return out, err
	}

	out.Shape = make([]int, len(shape))
	for i, index := range shape {
		out.Shape[i] = int(index)
	}

	return out, nil
}
