package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescontrolarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesdiscretizer"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesint"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretizer"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/domain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/unitspace"
)

type UnitSpaceCompiler interface {
	Compile() (FArray, error)

	CreateArray(
		axes AxesInt,
	) (AxesControlArray, error)
}

type UnitSpace struct {
	Axes   []Discretizer
	Domain Domain

	UnitSpaceCompiler
}

type UnitSpaceReader interface {
	Compile() error

	Space() (*farraydouble.Handle, error)

	CreateArray(
		*axesint.Handle,
	) (*axescontrolarray.Handle, error)

	Items() ([]*discretizer.Handle, error)

	Domain() (*domain.Handle, error)
}

var _ UnitSpaceReader = (*unitspace.Handle)(nil)

type falconUnitSpaceCompiler struct {
	reader UnitSpaceReader
}

func (f *falconUnitSpaceCompiler) Compile() (FArray, error) {
	if err := f.reader.Compile(); err != nil {
		return FArray{}, err
	}

	space, err := f.reader.Space()
	if err != nil {
		return FArray{}, err
	}
	defer space.Close()

	out, err := FArrayFromFalcon(space)
	if err != nil {
		return FArray{}, err
	}

	return out, nil
}

func (f *falconUnitSpaceCompiler) CreateArray(
	axes AxesInt,
) (AxesControlArray, error) {
	rawAxes, err := axes.NewFalconAxesInt()
	if err != nil {
		return AxesControlArray{}, err
	}
	defer rawAxes.Close()

	array, err := f.reader.CreateArray(rawAxes)
	if err != nil {
		return AxesControlArray{}, err
	}
	defer array.Close()

	return AxesControlArrayFromFalcon(array)
}

func UnitSpaceFromFalcon(
	u UnitSpaceReader,
) (UnitSpace, error) {
	rawAxes, err := u.Items()
	if err != nil {
		return UnitSpace{}, err
	}

	discretizers := make(
		[]Discretizer,
		0,
		len(rawAxes),
	)

	for _, axis := range rawAxes {
		d, err := DiscretizerFromFalcon(axis)
		if err != nil {
			return UnitSpace{}, err
		}

		discretizers = append(
			discretizers,
			d,
		)
	}

	rawDomain, err := u.Domain()
	if err != nil {
		return UnitSpace{}, err
	}

	domainValue, err := DomainFromFalcon(rawDomain)
	if err != nil {
		return UnitSpace{}, err
	}

	return UnitSpace{
		Axes:   discretizers,
		Domain: domainValue,
		UnitSpaceCompiler: &falconUnitSpaceCompiler{
			reader: u,
		},
	}, nil
}

func (u UnitSpace) NewFalconUnitSpace() (
	*unitspace.Handle,
	error,
) {
	rawDomain, err := u.Domain.NewFalconDomain()
	if err != nil {
		return nil, err
	}
	defer rawDomain.Close()

	rawAxes := make(
		[]*discretizer.Handle,
		0,
		len(u.Axes),
	)

	for _, axis := range u.Axes {
		d, err := axis.NewFalconDiscretizer()
		if err != nil {
			return nil, err
		}
		defer d.Close()

		rawAxes = append(rawAxes, d)
	}

	realAxes, err := axesdiscretizer.New(
		rawAxes,
	)
	if err != nil {
		return nil, err
	}
	defer realAxes.Close()

	return unitspace.New(
		realAxes,
		rawDomain,
	)
}
