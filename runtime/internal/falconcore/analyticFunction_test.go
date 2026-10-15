package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/liststring"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringdouble"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAnalyticFunctionReader struct {
	labelsHandle *liststring.Handle
	labelsErr    error

	evaluateValue float64
	evaluateErr   error

	arrayHandle *farraydouble.Handle
	arrayErr    error
}

func (m *mockAnalyticFunctionReader) Labels() (*liststring.Handle, error) {
	return m.labelsHandle, m.labelsErr
}

func (m *mockAnalyticFunctionReader) Evaluate(
	args *mapstringdouble.Handle,
	time float64,
) (float64, error) {
	return m.evaluateValue, m.evaluateErr
}

func (m *mockAnalyticFunctionReader) EvaluateArraywise(
	args *mapstringdouble.Handle,
	deltaT float64,
	maxTime float64,
) (*farraydouble.Handle, error) {
	return m.arrayHandle, m.arrayErr
}

func TestAnalyticFunctionFromFalcon(t *testing.T) {
	labels, err := liststring.New([]string{
		"Amplitude",
		"Offset",
	})
	require.NoError(t, err)
	defer labels.Close()

	reader := &mockAnalyticFunctionReader{
		labelsHandle: labels,
	}

	result, err := AnalyticFunctionFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{"Amplitude", "Offset"},
		result.Variables,
	)

	require.NotNil(
		t,
		result.AnalyticEvaluator,
	)
}

func TestAnalyticFunctionFromFalcon_LabelsError(
	t *testing.T,
) {
	expected := errors.New("labels failed")

	reader := &mockAnalyticFunctionReader{
		labelsErr: expected,
	}

	result, err := AnalyticFunctionFromFalcon(reader)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
	assert.Equal(t, AnalyticFunction{}, result)
}

func TestFalconAnalyticEvaluator_Evaluate(
	t *testing.T,
) {
	reader := &mockAnalyticFunctionReader{
		evaluateValue: 42.5,
	}

	evaluator := &falconAnalyticEvaluator{
		reader: reader,
	}

	value, err := evaluator.Evaluate(
		map[string]float64{
			"Amplitude": 1,
			"Offset":    2,
		},
		3,
	)

	require.NoError(t, err)

	assert.InDelta(
		t,
		42.5,
		value,
		1e-12,
	)
}

func TestFalconAnalyticEvaluator_EvaluateError(
	t *testing.T,
) {
	expected := errors.New("evaluate failed")

	reader := &mockAnalyticFunctionReader{
		evaluateErr: expected,
	}

	evaluator := &falconAnalyticEvaluator{
		reader: reader,
	}

	value, err := evaluator.Evaluate(
		map[string]float64{
			"Amplitude": 1,
		},
		0,
	)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, 0.0, value)
}

func TestFalconAnalyticEvaluator_EvaluateArraywise(
	t *testing.T,
) {
	rawArray := FArray{
		Data: []float64{
			1,
			2,
			3,
			4,
		},
		Shape: []uint64{2, 2},
	}

	handle, err := rawArray.NewFalconFArray()
	require.NoError(t, err)

	reader := &mockAnalyticFunctionReader{
		arrayHandle: handle,
	}

	evaluator := &falconAnalyticEvaluator{
		reader: reader,
	}

	result, err := evaluator.EvaluateArraywise(
		map[string]float64{
			"x": 1,
		},
		0.1,
		1,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		rawArray.Data,
		result.Data,
	)

	assert.Equal(
		t,
		rawArray.Shape,
		result.Shape,
	)
}

func TestFalconAnalyticEvaluator_EvaluateArraywiseError(
	t *testing.T,
) {
	expected := errors.New("arraywise failed")

	reader := &mockAnalyticFunctionReader{
		arrayErr: expected,
	}

	evaluator := &falconAnalyticEvaluator{
		reader: reader,
	}

	result, err := evaluator.EvaluateArraywise(
		map[string]float64{
			"x": 1,
		},
		0.1,
		1,
	)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, FArray{}, result)
}

func TestAnalyticFunctionFromFalcon_Integration(
	t *testing.T,
) {
	labels, err := liststring.New([]string{
		"A",
		"B",
	})
	require.NoError(t, err)
	defer labels.Close()

	reader := &mockAnalyticFunctionReader{
		labelsHandle:  labels,
		evaluateValue: 77,
	}

	fn, err := AnalyticFunctionFromFalcon(reader)

	require.NoError(t, err)

	value, err := fn.Evaluate(
		map[string]float64{
			"A": 1,
			"B": 2,
		},
		5,
	)

	require.NoError(t, err)

	assert.InDelta(
		t,
		77,
		value,
		1e-12,
	)
}
