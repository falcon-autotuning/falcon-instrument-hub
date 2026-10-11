package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/measuredarray"
)

type MeasuredArray struct {
	Data  []float64
	Shape []int
}

func (c MeasuredArray) NewFalconMeasuredArray() (*measuredarray.Handle, error) {
	shape := make([]uint64, len(c.Shape))
	for i, index := range c.Shape {
		shape[i] = uint64(index)
	}
	return measuredarray.FromData(c.Data, shape)
}

var _ ArrayReader = (*measuredarray.Handle)(nil)

func MeasuredArrayFromFalcon(c ArrayReader) (MeasuredArray, error) {
	var out MeasuredArray

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
