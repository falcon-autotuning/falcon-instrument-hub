package falconcore

import "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"

type LabelledMeasuredArray struct {
	AcquisitionContext
	MeasuredArray
}

func (l LabelledMeasuredArray) NewFalconLabelledMeasuredArray() (*labelledmeasuredarray.Handle, error) {
	ctx, err := l.NewFalconAcquisitionContext()
	if err != nil {
		return nil, err
	}
	ctrl, err := l.NewFalconMeasuredArray()
	if err != nil {
		return nil, err
	}
	defer ctx.Close()
	defer ctrl.Close()
	return labelledmeasuredarray.FromMeasuredArray(ctrl, ctx)
}

var _ LabelledArrayReader = (*labelledmeasuredarray.Handle)(nil)

func LabelledMeasuredArrayFromFalcon(l LabelledArrayReader) (LabelledMeasuredArray, error) {
	var out LabelledMeasuredArray
	var err error
	out.MeasuredArray, err = MeasuredArrayFromFalcon(l)
	if err != nil {
		return LabelledMeasuredArray{}, err
	}
	out.AcquisitionContext, err = AcquisitionContextFromFalcon(l)
	if err != nil {
		return LabelledMeasuredArray{}, err
	}
	return out, nil
}
