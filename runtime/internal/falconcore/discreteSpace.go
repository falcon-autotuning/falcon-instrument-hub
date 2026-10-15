package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescoupledlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesinstrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axeslabelledcontrolarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesmapstringbool"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretespace"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/domain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/unitspace"
)

type DiscreteSpaceProjector interface {
	GetProjection(
		projection AxesInstrumentPort,
	) (AxesLabelledControlArray, error)

	GetAxis(
		knob InstrumentPort,
	) (int, error)

	GetDomain(
		knob InstrumentPort,
	) (Domain, error)
}

type DiscreteSpace struct {
	Space      UnitSpace
	Axes       AxesCoupledLabelledDomain
	Increasing Increasing

	DiscreteSpaceProjector
}

type DiscreteSpaceReader interface {
	Space() (*unitspace.Handle, error)

	Axes() (*axescoupledlabelleddomain.Handle, error)

	Increasing() (*axesmapstringbool.Handle, error)

	GetAxis(
		*instrumentport.Handle,
	) (int32, error)

	GetDomain(
		*instrumentport.Handle,
	) (*domain.Handle, error)

	GetProjection(
		*axesinstrumentport.Handle,
	) (*axeslabelledcontrolarray.Handle, error)
}

var _ DiscreteSpaceReader = (*discretespace.Handle)(nil)

type falconDiscreteSpaceProjector struct {
	reader DiscreteSpaceReader
}

func (f *falconDiscreteSpaceProjector) GetAxis(
	knob InstrumentPort,
) (int, error) {
	realKnob, err := knob.NewFalconInstrumentPort()
	if err != nil {
		return 0, err
	}
	defer realKnob.Close()

	axis, err := f.reader.GetAxis(realKnob)
	return int(axis), err
}

func (f *falconDiscreteSpaceProjector) GetDomain(
	knob InstrumentPort,
) (Domain, error) {
	realKnob, err := knob.NewFalconInstrumentPort()
	if err != nil {
		return Domain{}, err
	}
	defer realKnob.Close()

	out, err := f.reader.GetDomain(realKnob)
	if err != nil {
		return Domain{}, err
	}
	defer out.Close()

	return DomainFromFalcon(out)
}

func (f *falconDiscreteSpaceProjector) GetProjection(
	projection AxesInstrumentPort,
) (AxesLabelledControlArray, error) {
	realProjection, err := projection.NewFalconAxesInstrumentPort()
	if err != nil {
		return AxesLabelledControlArray{}, err
	}
	defer realProjection.Close()

	out, err := f.reader.GetProjection(
		realProjection,
	)
	if err != nil {
		return AxesLabelledControlArray{}, err
	}
	defer out.Close()

	return AxesLabelledControlArrayFromFalcon(out)
}

func DiscreteSpaceFromFalcon(
	d DiscreteSpaceReader,
) (DiscreteSpace, error) {
	rawSpace, err := d.Space()
	if err != nil {
		return DiscreteSpace{}, err
	}

	space, err := UnitSpaceFromFalcon(rawSpace)
	if err != nil {
		return DiscreteSpace{}, err
	}

	rawAxes, err := d.Axes()
	if err != nil {
		return DiscreteSpace{}, err
	}

	axes, err := AxesCoupledLabelledDomainFromFalcon(
		rawAxes,
	)
	if err != nil {
		return DiscreteSpace{}, err
	}

	rawIncreasing, err := d.Increasing()
	if err != nil {
		return DiscreteSpace{}, err
	}

	increasing, err := IncreasingFromFalcon(
		rawIncreasing,
	)
	if err != nil {
		return DiscreteSpace{}, err
	}

	return DiscreteSpace{
		Space:      space,
		Axes:       axes,
		Increasing: increasing,
		DiscreteSpaceProjector: &falconDiscreteSpaceProjector{
			reader: d,
		},
	}, nil
}

func (d DiscreteSpace) NewFalconDiscreteSpace() (
	*discretespace.Handle,
	error,
) {
	rawSpace, err := d.Space.NewFalconUnitSpace()
	if err != nil {
		return nil, err
	}
	defer rawSpace.Close()

	rawAxes, err := d.Axes.NewFalconAxesCoupledLabelledDomain()
	if err != nil {
		return nil, err
	}
	defer rawAxes.Close()

	rawIncreasing, err := d.Increasing.NewFalconIncreasing()
	if err != nil {
		return nil, err
	}
	defer rawIncreasing.Close()

	return discretespace.New(
		rawSpace,
		rawAxes,
		rawIncreasing,
	)
}
