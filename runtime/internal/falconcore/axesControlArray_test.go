package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/controlarray"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAxesControlArrayReader struct {
	items []*controlarray.Handle
	err   error

	itemsCalls int
}

func (m *mockAxesControlArrayReader) Items() ([]*controlarray.Handle, error) {
	m.itemsCalls++

	if m.err != nil {
		return nil, m.err
	}

	return m.items, nil
}

func TestAxesControlArray_NewFalconAxesControlArray_Error(
	t *testing.T,
) {
	arr := AxesControlArray{
		{
			Data:  []float64{1, 2},
			Shape: []int{-1},
		},
	}

	handle, err := arr.NewFalconAxesControlArray()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestAxesControlArrayFromFalcon_ItemsError(t *testing.T) {
	expectedErr := errors.New("items failed")

	reader := &mockAxesControlArrayReader{
		err: expectedErr,
	}

	out, err := AxesControlArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Empty(t, out)
	assert.Equal(t, 1, reader.itemsCalls)
}

func TestAxesControlArrayFromFalcon_ControlArrayError(
	t *testing.T,
) {
	//
	// Use a handle that will cause ControlArrayFromFalcon
	// to fail. Adjust if your controlarray implementation
	// has a different invalid-state constructor.
	//
	badHandle := &controlarray.Handle{}

	reader := &mockAxesControlArrayReader{
		items: []*controlarray.Handle{badHandle},
	}

	out, err := AxesControlArrayFromFalcon(reader)

	require.Error(t, err)
	assert.Equal(t, AxesControlArray{}, out)

	assert.Equal(t, 1, reader.itemsCalls)
}

func TestAxesControlArrayFromFalcon_Success(t *testing.T) {
	first, err := (ControlArray{
		Data:  []float64{1, 2},
		Shape: []int{2},
	}).NewFalconControlArray()
	require.NoError(t, err)
	defer first.Close()

	second, err := (ControlArray{
		Data:  []float64{3, 4},
		Shape: []int{2},
	}).NewFalconControlArray()
	require.NoError(t, err)
	defer second.Close()

	reader := &mockAxesControlArrayReader{
		items: []*controlarray.Handle{
			first,
			second,
		},
	}

	out, err := AxesControlArrayFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(t,
		AxesControlArray{
			{
				Data:  []float64{1, 2},
				Shape: []int{2},
			},
			{
				Data:  []float64{3, 4},
				Shape: []int{2},
			},
		},
		out,
	)

	assert.Equal(t, 1, reader.itemsCalls)
}

func TestAxesControlArray_NewFalconAxesControlArray_Success(t *testing.T) {
	arr := AxesControlArray{
		{
			Data:  []float64{1, 2},
			Shape: []int{2},
		},
		{
			Data:  []float64{3, 4},
			Shape: []int{2},
		},
	}

	handle, err := arr.NewFalconAxesControlArray()

	require.NoError(t, err)
	require.NotNil(t, handle)

	items, err := handle.Items()
	require.NoError(t, err)

	assert.Len(t, items, 2)

	assert.NoError(t, handle.Close())
}
