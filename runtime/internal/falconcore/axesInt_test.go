package falconcore

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAxesIntReader struct {
	items []int32
	err   error

	itemsCalls int
}

func (m *mockAxesIntReader) Items() ([]int32, error) {
	m.itemsCalls++
	if m.err != nil {
		return nil, m.err
	}
	return m.items, nil
}

func TestAxesInt_NewFalconAxesInt_Populated(t *testing.T) {
	a := AxesInt{1, 2, 3, -4}

	handle, err := a.NewFalconAxesInt()

	require.NoError(t, err)
	require.NotNil(t, handle)

	items, err := handle.Items()
	require.NoError(t, err)
	assert.Equal(t, []int32{1, 2, 3, -4}, items)

	assert.NoError(t, handle.Close())
}

func TestAxesIntFromFalcon_ItemsError(t *testing.T) {
	expectedErr := errors.New("items failed")

	reader := &mockAxesIntReader{
		err: expectedErr,
	}

	out, err := AxesIntFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Empty(t, out)
	assert.Equal(t, 1, reader.itemsCalls)
}

func TestAxesIntFromFalcon_Empty(t *testing.T) {
	reader := &mockAxesIntReader{
		items: []int32{},
	}

	out, err := AxesIntFromFalcon(reader)

	require.NoError(t, err)

	// This assertion will FAIL with the current implementation,
	// revealing the bug in AxesIntFromFalcon.
	assert.Equal(t, AxesInt{}, out)

	assert.Equal(t, 1, reader.itemsCalls)
}

func TestAxesIntFromFalcon_Populated(t *testing.T) {
	reader := &mockAxesIntReader{
		items: []int32{1, 2, 3, -4},
	}

	out, err := AxesIntFromFalcon(reader)

	require.NoError(t, err)

	// This assertion will FAIL with the current implementation,
	// because the result becomes:
	// [0 0 0 0 1 2 3 -4]
	assert.Equal(t, AxesInt{1, 2, 3, -4}, out)

	assert.Equal(t, 1, reader.itemsCalls)
}
