package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringbool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncreasing_NewFalconIncreasing_Empty(
	t *testing.T,
) {
	inc := Increasing{}

	handle, err := inc.NewFalconIncreasing()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	items, err := handle.Items()
	require.NoError(t, err)

	assert.Empty(t, items)
}

func TestIncreasing_NewFalconIncreasing_Single(
	t *testing.T,
) {
	inc := Increasing{
		{
			"x": true,
			"y": false,
		},
	}

	handle, err := inc.NewFalconIncreasing()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	items, err := handle.Items()
	require.NoError(t, err)

	assert.Len(t, items, 1)
}

func TestIncreasingFromFalcon_Empty(
	t *testing.T,
) {
	raw, err := Increasing{}.NewFalconIncreasing()
	require.NoError(t, err)
	defer raw.Close()

	result, err := IncreasingFromFalcon(raw)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestIncreasing_RoundTrip(
	t *testing.T,
) {
	original := Increasing{
		{
			"P1": true,
			"P2": false,
		},
		{
			"B1": true,
			"B2": true,
			"B3": false,
		},
	}

	raw, err := original.NewFalconIncreasing()
	require.NoError(t, err)
	defer raw.Close()

	result, err := IncreasingFromFalcon(raw)

	require.NoError(t, err)
	require.Len(t, result, len(original))

	for idx := range original {
		assert.Equal(
			t,
			original[idx],
			result[idx],
		)
	}
}

type mockIncreasingReader struct {
	items []*mapstringbool.Handle
	err   error
}

func (m *mockIncreasingReader) Items() (
	[]*mapstringbool.Handle,
	error,
) {
	return m.items, m.err
}

func TestIncreasingFromFalcon_ItemsError(
	t *testing.T,
) {
	expectedErr := errors.New("items failed")

	result, err := IncreasingFromFalcon(
		&mockIncreasingReader{
			err: expectedErr,
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
	assert.Nil(t, result)
}
