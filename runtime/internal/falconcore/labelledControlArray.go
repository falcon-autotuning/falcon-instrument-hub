package falconcore

import "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledcontrolarray"

type LabelledControlArray struct {
	AcquisitionContext
	ControlArray
}

func (l LabelledControlArray) NewFalconLabelledControlArray() (*labelledcontrolarray.Handle, error) {
	ctx, err := l.NewFalconAcquisitionContext()
	if err != nil {
		return nil, err
	}
	ctrl, err := l.NewFalconControlArray()
	if err != nil {
		return nil, err
	}
	defer ctx.Close()
	defer ctrl.Close()
	return labelledcontrolarray.FromControlArray(ctrl, ctx)
}

type LabelledArrayReader interface {
	ArrayReader
	AcquisitionContextReader
}

var _ LabelledArrayReader = (*labelledcontrolarray.Handle)(nil)

func LabelledControlArrayFromFalcon(l LabelledArrayReader) (LabelledControlArray, error) {
	var out LabelledControlArray
	var err error
	out.ControlArray, err = ControlArrayFromFalcon(l)
	if err != nil {
		return LabelledControlArray{}, err
	}
	out.AcquisitionContext, err = AcquisitionContextFromFalcon(l)
	if err != nil {
		return LabelledControlArray{}, err
	}
	return out, nil
}
