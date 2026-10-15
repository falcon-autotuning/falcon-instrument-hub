package falconcore

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockFArrayReader struct {
	data     []float64
	shape    []uint64
	dataErr  error
	shapeErr error
}

func (m *mockFArrayReader) Data() ([]float64, error) {
	return m.data, m.dataErr
}

func (m *mockFArrayReader) Shape() ([]uint64, error) {
	return m.shape, m.shapeErr
}

func TestFArrayFromFalcon(t *testing.T) {
	reader := &mockFArrayReader{
		data:  []float64{1, 2, 3, 4},
		shape: []uint64{2, 2},
	}

	result, err := FArrayFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		[]float64{1, 2, 3, 4},
		result.Data,
	)

	assert.Equal(
		t,
		[]uint64{2, 2},
		result.Shape,
	)
}

func TestFArrayFromFalcon_DataError(t *testing.T) {
	expected := errors.New("data failed")

	reader := &mockFArrayReader{
		dataErr: expected,
	}

	result, err := FArrayFromFalcon(reader)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
	assert.Equal(t, FArray{}, result)
}

func TestFArrayFromFalcon_ShapeError(t *testing.T) {
	expected := errors.New("shape failed")

	reader := &mockFArrayReader{
		data:     []float64{1, 2},
		shapeErr: expected,
	}

	result, err := FArrayFromFalcon(reader)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
	assert.Equal(t, FArray{}, result)
}

func TestFArray_NewFalconFArray(t *testing.T) {
	array := FArray{
		Data: []float64{
			1,
			2,
			3,
			4,
			5,
			6,
		},
		Shape: []uint64{2, 3},
	}

	handle, err := array.NewFalconFArray()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	data, err := handle.Data()
	require.NoError(t, err)

	shape, err := handle.Shape()
	require.NoError(t, err)

	assert.Equal(t, array.Data, data)
	assert.Equal(t, array.Shape, shape)
}

func TestFArray_RoundTrip(t *testing.T) {
	original := FArray{
		Data: []float64{
			1.5,
			2.5,
			3.5,
			4.5,
		},
		Shape: []uint64{2, 2},
	}

	handle, err := original.NewFalconFArray()
	require.NoError(t, err)
	defer handle.Close()

	result, err := FArrayFromFalcon(handle)

	require.NoError(t, err)

	assert.Equal(
		t,
		original.Data,
		result.Data,
	)

	assert.Equal(
		t,
		original.Shape,
		result.Shape,
	)
}
