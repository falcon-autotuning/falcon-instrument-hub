package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/liststring"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringdouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPortTransformReader struct {
	portHandle *instrumentport.Handle
	portErr    error

	evaluateValue float64
	evaluateErr   error

	arrayHandle *farraydouble.Handle
	arrayErr    error
}

func (m *mockPortTransformReader) Port() (*instrumentport.Handle, error) {
	return m.portHandle, m.portErr
}

func (m *mockPortTransformReader) Labels() (*liststring.Handle, error) {
	return liststring.New([]string{"x"})
}

func (m *mockPortTransformReader) Evaluate(
	args *mapstringdouble.Handle,
	time float64,
) (float64, error) {
	return m.evaluateValue, m.evaluateErr
}

func (m *mockPortTransformReader) EvaluateArraywise(
	args *mapstringdouble.Handle,
	deltaT float64,
	maxTime float64,
) (*farraydouble.Handle, error) {
	return m.arrayHandle, m.arrayErr
}

func TestPortTransformFromFalcon(t *testing.T) {
	port := InstrumentPort{
		InstrumentName: "DAC",
	}

	rawPort, err := port.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort.Close()

	reader := &mockPortTransformReader{
		portHandle: rawPort,
	}

	result, err := PortTransformFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		port,
		result.Port,
	)

	require.NotNil(
		t,
		result.AnalyticEvaluator,
	)
}

func TestPortTransformFromFalcon_PortError(
	t *testing.T,
) {
	expected := errors.New("port failed")

	reader := &mockPortTransformReader{
		portErr: expected,
	}

	result, err := PortTransformFromFalcon(reader)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		PortTransform{},
		result,
	)
}

func TestPortTransformFromFalcon_InvalidPort(
	t *testing.T,
) {
	port := InstrumentPort{
		InstrumentName: "",
	}

	rawPort, err := port.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort.Close()

	reader := &mockPortTransformReader{
		portHandle: rawPort,
	}

	result, err := PortTransformFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		port,
		result.Port,
	)
}

func TestFalconPortTransformEvaluator_Evaluate(
	t *testing.T,
) {
	reader := &mockPortTransformReader{
		evaluateValue: 123.45,
	}

	evaluator := &falconPortTransformEvaluator{
		reader: reader,
	}

	value, err := evaluator.Evaluate(
		map[string]float64{
			"gain": 2,
		},
		1.5,
	)

	require.NoError(t, err)

	assert.InDelta(
		t,
		123.45,
		value,
		1e-12,
	)
}

func TestFalconPortTransformEvaluator_EvaluateError(
	t *testing.T,
) {
	expected := errors.New("evaluate failed")

	reader := &mockPortTransformReader{
		evaluateErr: expected,
	}

	evaluator := &falconPortTransformEvaluator{
		reader: reader,
	}

	value, err := evaluator.Evaluate(
		map[string]float64{
			"gain": 2,
		},
		1.5,
	)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		0.0,
		value,
	)
}

func TestFalconPortTransformEvaluator_EvaluateArraywise(
	t *testing.T,
) {
	expected := FArray{
		Data: []float64{
			1,
			2,
			3,
			4,
		},
		Shape: []uint64{
			2,
			2,
		},
	}

	handle, err := expected.NewFalconFArray()
	require.NoError(t, err)

	reader := &mockPortTransformReader{
		arrayHandle: handle,
	}

	evaluator := &falconPortTransformEvaluator{
		reader: reader,
	}

	result, err := evaluator.EvaluateArraywise(
		map[string]float64{
			"x": 1,
		},
		0.1,
		1.0,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		expected.Data,
		result.Data,
	)

	assert.Equal(
		t,
		expected.Shape,
		result.Shape,
	)
}

func TestFalconPortTransformEvaluator_EvaluateArraywiseError(
	t *testing.T,
) {
	expected := errors.New("arraywise failed")

	reader := &mockPortTransformReader{
		arrayErr: expected,
	}

	evaluator := &falconPortTransformEvaluator{
		reader: reader,
	}

	result, err := evaluator.EvaluateArraywise(
		map[string]float64{
			"x": 1,
		},
		0.1,
		1.0,
	)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		FArray{},
		result,
	)
}

func TestPortTransformFromFalcon_Integration(
	t *testing.T,
) {
	port := InstrumentPort{
		InstrumentName: "DAC",
	}

	rawPort, err := port.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort.Close()

	reader := &mockPortTransformReader{
		portHandle:    rawPort,
		evaluateValue: 55.5,
	}

	transform, err := PortTransformFromFalcon(reader)
	require.NoError(t, err)

	value, err := transform.Evaluate(
		map[string]float64{
			"offset": 1,
		},
		0,
	)

	require.NoError(t, err)

	assert.InDelta(
		t,
		55.5,
		value,
		1e-12,
	)
}
