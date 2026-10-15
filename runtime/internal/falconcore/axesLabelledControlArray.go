package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledcontrolarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axeslabelledcontrolarray"
)

type AxesLabelledControlArray []LabelledControlArray

func (a AxesLabelledControlArray) NewFalconAxesLabelledControlArray() (*axeslabelledcontrolarray.Handle, error) {
	out := make([]*labelledcontrolarray.Handle, 0, len(a))
	for _, arr := range a {
		realarr, err := arr.NewFalconLabelledControlArray()
		if err != nil {
			return nil, err
		}
		defer realarr.Close()
		out = append(out, realarr)
	}
	return axeslabelledcontrolarray.New(out)
}

type AxesLabelledControlArrayReader interface {
	Items() ([]*labelledcontrolarray.Handle, error)
}

var _ AxesLabelledControlArrayReader = (*axeslabelledcontrolarray.Handle)(nil)

func AxesLabelledControlArrayFromFalcon(a AxesLabelledControlArrayReader) (AxesLabelledControlArray, error) {
	list, err := a.Items()
	if err != nil {
		return AxesLabelledControlArray{}, err
	}
	inner := make(AxesLabelledControlArray, 0, len(list))
	for _, arr := range list {
		carr, err := LabelledControlArrayFromFalcon(arr)
		if err != nil {
			return AxesLabelledControlArray{}, err
		}
		inner = append(inner, carr)
		arr.Close()
	}
	return inner, nil
}
