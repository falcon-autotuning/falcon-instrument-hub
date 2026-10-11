package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listpairstringstring"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringstring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMetadataReader struct {
	items     *listpairstringstring.Handle
	err       error
	itemCalls int
}

func (m *mockMetadataReader) Items() (*listpairstringstring.Handle, error) {
	m.itemCalls++

	if m.err != nil {
		return nil, m.err
	}

	return m.items, nil
}

func TestMetadata_NewFalconMetadata_Empty(t *testing.T) {
	meta := Metadata{}

	handle, err := meta.NewFalconMetadata()

	require.NoError(t, err)
	require.NotNil(t, handle)

	items, err := handle.Items()
	require.NoError(t, err)

	pairs, err := items.Items()
	require.NoError(t, err)

	assert.Empty(t, pairs)

	assert.NoError(t, items.Close())
	assert.NoError(t, handle.Close())
}

func TestMetadata_NewFalconMetadata_Populated(t *testing.T) {
	meta := Metadata{
		"key1": "value1",
		"key2": "value2",
	}

	handle, err := meta.NewFalconMetadata()

	require.NoError(t, err)
	require.NotNil(t, handle)

	out, err := MetadataFromFalcon(handle)

	require.NoError(t, err)
	assert.Equal(t, meta, out)

	assert.NoError(t, handle.Close())
}

func TestMetadataFromFalcon_ItemsError(t *testing.T) {
	expectedErr := errors.New("items failed")

	reader := &mockMetadataReader{
		err: expectedErr,
	}

	out, err := MetadataFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, Metadata{}, out)

	assert.Equal(t, 1, reader.itemCalls)
}

func TestMetadataFromFalcon_Empty(t *testing.T) {
	handle, err := mapstringstring.NewEmpty()
	require.NoError(t, err)
	defer handle.Close()

	out, err := MetadataFromFalcon(handle)

	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestMetadataFromFalcon_Success(t *testing.T) {
	handle, err := mapstringstring.NewEmpty()
	require.NoError(t, err)
	defer handle.Close()

	require.NoError(t, handle.Insert("A", "1"))
	require.NoError(t, handle.Insert("B", "2"))
	require.NoError(t, handle.Insert("C", "3"))

	out, err := MetadataFromFalcon(handle)

	require.NoError(t, err)

	assert.Equal(t,
		Metadata{
			"A": "1",
			"B": "2",
			"C": "3",
		},
		out,
	)
}
