package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/controlarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescontrolarray"
)

type AxesControlArray []ControlArray

func (a AxesControlArray) NewFalconAxesControlArray() (*axescontrolarray.Handle, error) {
	out := make([]*controlarray.Handle, 0, len(a))
	for _, arr := range a {
		realarr, err := arr.NewFalconControlArray()
		if err != nil {
			return nil, err
		}
		defer realarr.Close()
		out = append(out, realarr)
	}
	return axescontrolarray.New(out)
}

type AxesControlArrayReader interface {
	Items() ([]*controlarray.Handle, error)
}

var _ AxesControlArrayReader = (*axescontrolarray.Handle)(nil)

func AxesControlArrayFromFalcon(a AxesControlArrayReader) (AxesControlArray, error) {
	list, err := a.Items()
	if err != nil {
		return AxesControlArray{}, err
	}
	inner := make(AxesControlArray, 0, len(list))
	for _, arr := range list {
		carr, err := ControlArrayFromFalcon(arr)
		if err != nil {
			return AxesControlArray{}, err
		}
		inner = append(inner, carr)
		arr.Close()
	}
	return inner, nil
}
