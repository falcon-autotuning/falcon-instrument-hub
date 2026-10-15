package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/liststring"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringdouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/port-transforms/porttransform"
)

// There is no NewPortTransform interface since this is supposed to be consumed entirely on hub side
type PortTransform struct {
	Port InstrumentPort

	AnalyticEvaluator
}

type PortTransformReader interface {
	Port() (*instrumentport.Handle, error)

	Labels() (*liststring.Handle, error)

	Evaluate(
		args *mapstringdouble.Handle,
		time float64,
	) (float64, error)

	EvaluateArraywise(
		args *mapstringdouble.Handle,
		deltaT float64,
		maxTime float64,
	) (*farraydouble.Handle, error)
}

var _ PortTransformReader = (*porttransform.Handle)(nil)

type falconPortTransformEvaluator struct {
	reader PortTransformReader
}

func (f *falconPortTransformEvaluator) Evaluate(
	args map[string]float64,
	time float64,
) (float64, error) {
	m, err := buildMapStringDouble(args)
	if err != nil {
		return 0, err
	}
	defer m.Close()

	return f.reader.Evaluate(m, time)
}

func (f *falconPortTransformEvaluator) EvaluateArraywise(
	args map[string]float64,
	deltaT float64,
	maxTime float64,
) (FArray, error) {
	m, err := buildMapStringDouble(args)
	if err != nil {
		return FArray{}, err
	}
	defer m.Close()

	out, err := f.reader.EvaluateArraywise(
		m,
		deltaT,
		maxTime,
	)
	if err != nil {
		return FArray{}, err
	}
	defer out.Close()

	return FArrayFromFalcon(out)
}

func PortTransformFromFalcon(
	p PortTransformReader,
) (PortTransform, error) {
	portHandle, err := p.Port()
	if err != nil {
		return PortTransform{}, err
	}
	defer portHandle.Close()

	port, err := InstrumentPortFromFalcon(portHandle)
	if err != nil {
		return PortTransform{}, err
	}

	return PortTransform{
		Port: port,
		AnalyticEvaluator: &falconPortTransformEvaluator{
			reader: p,
		},
	}, nil
}
