package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/liststring"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringdouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/pairstringdouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/analyticfunction"
)

// There is no NewAnalyticFunction interface since this is supposed to be consumed entirely on hub side
type AnalyticEvaluator interface {
	Evaluate(
		args map[string]float64,
		time float64,
	) (float64, error)

	EvaluateArraywise(
		args map[string]float64,
		deltaT float64,
		maxTime float64,
	) (FArray, error)
}

type AnalyticFunction struct {
	Variables []string

	AnalyticEvaluator
}

type AnalyticFunctionReader interface {
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

var _ AnalyticFunctionReader = (*analyticfunction.Handle)(nil)

type falconAnalyticEvaluator struct {
	reader AnalyticFunctionReader
}

func buildMapStringDouble(
	args map[string]float64,
) (*mapstringdouble.Handle, error) {
	pairs := make([]*pairstringdouble.Handle, 0, len(args))

	defer func() {
		for _, pair := range pairs {
			pair.Close()
		}
	}()

	for key, value := range args {
		pair, err := pairstringdouble.New(key, value)
		if err != nil {
			return nil, err
		}

		pairs = append(pairs, pair)
	}

	return mapstringdouble.New(pairs)
}

func (f *falconAnalyticEvaluator) Evaluate(
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

func (f *falconAnalyticEvaluator) EvaluateArraywise(
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

func AnalyticFunctionFromFalcon(
	a AnalyticFunctionReader,
) (AnalyticFunction, error) {
	labelsHandle, err := a.Labels()
	if err != nil {
		return AnalyticFunction{}, err
	}
	defer labelsHandle.Close()
	rawlabels, err := labelsHandle.Items()
	if err != nil {
		return AnalyticFunction{}, err
	}

	return AnalyticFunction{
		Variables: rawlabels,
		AnalyticEvaluator: &falconAnalyticEvaluator{
			reader: a,
		},
	}, nil
}
