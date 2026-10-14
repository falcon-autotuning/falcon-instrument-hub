package falconcore

import "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesint"

type AxesInt []int

func (a AxesInt) NewFalconAxesInt() (*axesint.Handle, error) {
	if len(a) == 0 {
		return axesint.NewEmpty()
	}
	var inner []int32
	for _, number := range a {
		inner = append(inner, int32(number))
	}
	return axesint.New(inner)
}

type AxesIntReader interface {
	Items() ([]int32, error)
}

var _ AxesIntReader = (*axesint.Handle)(nil)

func AxesIntFromFalcon(a AxesIntReader) (AxesInt, error) {
	list, err := a.Items()
	if err != nil {
		return AxesInt{}, err
	}
	inner := make(AxesInt, len(list))
	for i, number := range list {
		inner[i] = int(number)
	}
	return inner, nil
}
