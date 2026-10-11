package falconcore

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockArrayReader struct {
	data     []float64
	shape    []uint64
	dataErr  error
	shapeErr error

	dataCalls  int
	shapeCalls int
}

func (m *mockArrayReader) Data() ([]float64, error) {
	m.dataCalls++

	if m.dataErr != nil {
		return nil, m.dataErr
	}

	return m.data, nil
}

func (m *mockArrayReader) Shape() ([]uint64, error) {
	m.shapeCalls++

	if m.shapeErr != nil {
		return nil, m.shapeErr
	}

	return m.shape, nil
}

func TestControlArray_NewFalconControlArray_Populated(t *testing.T) {
	array := ControlArray{
		Data:  []float64{1.1, 2.2, 3.3, 4.4},
		Shape: []int{1, 4},
	}

	handle, err := array.NewFalconControlArray()

	require.NoError(t, err)
	require.NotNil(t, handle)

	data, err := handle.Data()
	require.NoError(t, err)
	assert.Equal(t, []float64{1.1, 2.2, 3.3, 4.4}, data)

	shape, err := handle.Shape()
	require.NoError(t, err)
	assert.Equal(t, []uint64{1, 4}, shape)

	assert.NoError(t, handle.Close())
}

func TestControlArrayFromFalcon_DataError(t *testing.T) {
	expectedErr := errors.New("data failed")

	reader := &mockArrayReader{
		dataErr: expectedErr,
	}

	out, err := ControlArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, ControlArray{}, out)

	assert.Equal(t, 1, reader.dataCalls)
	assert.Equal(t, 0, reader.shapeCalls)
}

func TestControlArrayFromFalcon_ShapeError(t *testing.T) {
	expectedErr := errors.New("shape failed")

	reader := &mockArrayReader{
		data:     []float64{1, 2, 3},
		shapeErr: expectedErr,
	}

	out, err := ControlArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, ControlArray{
		Data: []float64{1, 2, 3},
	}, out)

	assert.Equal(t, 1, reader.dataCalls)
	assert.Equal(t, 1, reader.shapeCalls)
}

func TestControlArrayFromFalcon_Success(t *testing.T) {
	reader := &mockArrayReader{
		data:  []float64{1, 2, 3, 4},
		shape: []uint64{2, 2},
	}

	out, err := ControlArrayFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(t, ControlArray{
		Data:  []float64{1, 2, 3, 4},
		Shape: []int{2, 2},
	}, out)

	assert.Equal(t, 1, reader.dataCalls)
	assert.Equal(t, 1, reader.shapeCalls)
}
