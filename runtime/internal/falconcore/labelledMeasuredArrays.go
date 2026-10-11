package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listlabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"
)

type LabelledMeasuredArrays []LabelledMeasuredArray

func (l LabelledMeasuredArrays) NewFalconLabelledMeasuredArrays() (*labelledarrayslabelledmeasuredarray.Handle, error) {
	out := make([]*labelledmeasuredarray.Handle, 0, len(l))
	for _, arr := range l {
		realarr, err := arr.NewFalconLabelledMeasuredArray()
		if err != nil {
			return nil, err
		}
		defer realarr.Close()
		out = append(out, realarr)
	}
	return labelledarrayslabelledmeasuredarray.New(out)
}

type LabelledMeasuredArraysReader interface {
	Arrays() (*listlabelledmeasuredarray.Handle, error)
}

var _ LabelledMeasuredArraysReader = (*labelledarrayslabelledmeasuredarray.Handle)(nil)

func LabelledMeasuredArraysFromFalcon(l LabelledMeasuredArraysReader) (LabelledMeasuredArrays, error) {
	list, err := l.Arrays()
	if err != nil {
		return LabelledMeasuredArrays{}, err
	}
	defer list.Close()
	reallist, err := list.Items()
	if err != nil {
		return LabelledMeasuredArrays{}, err
	}
	out := make(LabelledMeasuredArrays, 0, len(reallist))
	for _, arr := range reallist {
		marr, err := LabelledMeasuredArrayFromFalcon(arr)
		if err != nil {
			return LabelledMeasuredArrays{}, err
		}
		out = append(out, marr)
		arr.Close()
	}
	return out, nil
}
