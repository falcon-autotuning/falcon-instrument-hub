package falconcore

import "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"

type FArray struct {
	Data  []float64
	Shape []uint64
}

type FArrayReader interface {
	Data() ([]float64, error)
	Shape() ([]uint64, error)
}

var _ FArrayReader = (*farraydouble.Handle)(nil)

func FArrayFromFalcon(
	f FArrayReader,
) (FArray, error) {
	data, err := f.Data()
	if err != nil {
		return FArray{}, err
	}

	shape, err := f.Shape()
	if err != nil {
		return FArray{}, err
	}

	return FArray{
		Data:  data,
		Shape: shape,
	}, nil
}

func (f FArray) NewFalconFArray() (*farraydouble.Handle, error) {
	if len(f.Data) == 0 {
		return farraydouble.NewEmpty(f.Shape)
	}

	return farraydouble.FromData(
		f.Data,
		f.Shape,
	)
}
