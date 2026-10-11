package falconcore

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasuredArray_NewFalconMeasuredArray_Populated(t *testing.T) {
	array := MeasuredArray{
		Data:  []float64{1.1, 2.2, 3.3, 4.4},
		Shape: []int{2, 2},
	}

	handle, err := array.NewFalconMeasuredArray()

	require.NoError(t, err)
	require.NotNil(t, handle)

	data, err := handle.Data()
	require.NoError(t, err)
	assert.Equal(t, []float64{1.1, 2.2, 3.3, 4.4}, data)

	shape, err := handle.Shape()
	require.NoError(t, err)
	assert.Equal(t, []uint64{2, 2}, shape)

	assert.NoError(t, handle.Close())
}

func TestMeasuredArrayFromFalcon_DataError(t *testing.T) {
	expectedErr := errors.New("data failed")

	reader := &mockArrayReader{
		dataErr: expectedErr,
	}

	out, err := MeasuredArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, MeasuredArray{}, out)

	assert.Equal(t, 1, reader.dataCalls)
	assert.Equal(t, 0, reader.shapeCalls)
}

func TestMeasuredArrayFromFalcon_ShapeError(t *testing.T) {
	expectedErr := errors.New("shape failed")

	reader := &mockArrayReader{
		data:     []float64{1, 2, 3},
		shapeErr: expectedErr,
	}

	out, err := MeasuredArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)

	assert.Equal(t, MeasuredArray{
		Data: []float64{1, 2, 3},
	}, out)

	assert.Equal(t, 1, reader.dataCalls)
	assert.Equal(t, 1, reader.shapeCalls)
}

func TestMeasuredArrayFromFalcon_Success(t *testing.T) {
	reader := &mockArrayReader{
		data:  []float64{1, 2, 3, 4},
		shape: []uint64{2, 2},
	}

	out, err := MeasuredArrayFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(t, MeasuredArray{
		Data:  []float64{1, 2, 3, 4},
		Shape: []int{2, 2},
	}, out)

	assert.Equal(t, 1, reader.dataCalls)
	assert.Equal(t, 1, reader.shapeCalls)
}
