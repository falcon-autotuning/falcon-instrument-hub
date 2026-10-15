package falconcore

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDiscretizerReader struct {
	delta    float64
	deltaErr error

	cartesian    bool
	cartesianErr error

	polar    bool
	polarErr error
}

func (m *mockDiscretizerReader) Delta() (float64, error) {
	return m.delta, m.deltaErr
}

func (m *mockDiscretizerReader) IsCartesian() (bool, error) {
	return m.cartesian, m.cartesianErr
}

func (m *mockDiscretizerReader) IsPolar() (bool, error) {
	return m.polar, m.polarErr
}

func TestDiscretizerFromFalcon_Cartesian(
	t *testing.T,
) {
	reader := &mockDiscretizerReader{
		delta:     0.25,
		cartesian: true,
	}

	result, err := DiscretizerFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		Discretizer{
			Type:  CartesianDiscretization,
			Delta: 0.25,
		},
		result,
	)
}

func TestDiscretizerFromFalcon_Polar(
	t *testing.T,
) {
	reader := &mockDiscretizerReader{
		delta: 0.5,
		polar: true,
	}

	result, err := DiscretizerFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		Discretizer{
			Type:  PolarDiscretization,
			Delta: 0.5,
		},
		result,
	)
}

func TestDiscretizerFromFalcon_DeltaError(
	t *testing.T,
) {
	expected := errors.New("delta failed")

	reader := &mockDiscretizerReader{
		deltaErr: expected,
	}

	result, err := DiscretizerFromFalcon(reader)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		Discretizer{},
		result,
	)
}

func TestDiscretizerFromFalcon_IsCartesianError(
	t *testing.T,
) {
	expected := errors.New("cartesian failed")

	reader := &mockDiscretizerReader{
		delta:        1,
		cartesianErr: expected,
	}

	result, err := DiscretizerFromFalcon(reader)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		Discretizer{},
		result,
	)
}

func TestDiscretizerFromFalcon_IsPolarError(
	t *testing.T,
) {
	expected := errors.New("polar failed")

	reader := &mockDiscretizerReader{
		delta:    1,
		polarErr: expected,
	}

	result, err := DiscretizerFromFalcon(reader)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		Discretizer{},
		result,
	)
}

func TestDiscretizerFromFalcon_UnknownType(
	t *testing.T,
) {
	reader := &mockDiscretizerReader{
		delta: 1,
	}

	result, err := DiscretizerFromFalcon(reader)

	require.Error(t, err)

	assert.EqualError(
		t,
		err,
		"unknown discretizer type",
	)

	assert.Equal(
		t,
		Discretizer{},
		result,
	)
}

func TestDiscretizer_NewFalconDiscretizer_Cartesian(
	t *testing.T,
) {
	d := Discretizer{
		Type:  CartesianDiscretization,
		Delta: 0.75,
	}

	handle, err := d.NewFalconDiscretizer()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	delta, err := handle.Delta()
	require.NoError(t, err)

	cartesian, err := handle.IsCartesian()
	require.NoError(t, err)

	assert.Equal(t, 0.75, delta)
	assert.True(t, cartesian)
}

func TestDiscretizer_NewFalconDiscretizer_Polar(
	t *testing.T,
) {
	d := Discretizer{
		Type:  PolarDiscretization,
		Delta: 1.25,
	}

	handle, err := d.NewFalconDiscretizer()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	delta, err := handle.Delta()
	require.NoError(t, err)

	polar, err := handle.IsPolar()
	require.NoError(t, err)

	assert.Equal(t, 1.25, delta)
	assert.True(t, polar)
}

func TestDiscretizer_NewFalconDiscretizer_InvalidType(
	t *testing.T,
) {
	d := Discretizer{
		Type:  DiscretizationType(999),
		Delta: 1,
	}

	handle, err := d.NewFalconDiscretizer()

	require.Error(t, err)

	assert.Nil(t, handle)

	assert.EqualError(
		t,
		err,
		"unknown discretizer type 999",
	)
}

func TestDiscretizer_RoundTrip_Cartesian(
	t *testing.T,
) {
	original := Discretizer{
		Type:  CartesianDiscretization,
		Delta: 0.123,
	}

	handle, err := original.NewFalconDiscretizer()
	require.NoError(t, err)
	defer handle.Close()

	result, err := DiscretizerFromFalcon(handle)

	require.NoError(t, err)

	assert.Equal(
		t,
		original,
		result,
	)
}

func TestDiscretizer_RoundTrip_Polar(
	t *testing.T,
) {
	original := Discretizer{
		Type:  PolarDiscretization,
		Delta: 0.456,
	}

	handle, err := original.NewFalconDiscretizer()
	require.NoError(t, err)
	defer handle.Close()

	result, err := DiscretizerFromFalcon(handle)

	require.NoError(t, err)

	assert.Equal(
		t,
		original,
		result,
	)
}
