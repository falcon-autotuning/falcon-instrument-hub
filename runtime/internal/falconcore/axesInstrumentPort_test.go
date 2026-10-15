package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAxesInstrumentPortReader struct {
	items    []*instrumentport.Handle
	itemsErr error
}

func (m *mockAxesInstrumentPortReader) Items() ([]*instrumentport.Handle, error) {
	return m.items, m.itemsErr
}

func TestAxesInstrumentPort_NewFalconAxesInstrumentPort_Empty(
	t *testing.T,
) {
	ports := AxesInstrumentPort{}

	handle, err := ports.NewFalconAxesInstrumentPort()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	size, err := handle.Size()
	require.NoError(t, err)

	assert.Equal(t, uint64(0), size)
}

func TestAxesInstrumentPort_NewFalconAxesInstrumentPort_Single(
	t *testing.T,
) {
	ports := AxesInstrumentPort{
		{
			InstrumentName: "DAC",
		},
	}

	handle, err := ports.NewFalconAxesInstrumentPort()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	size, err := handle.Size()
	require.NoError(t, err)

	assert.Equal(t, uint64(1), size)
}

func TestAxesInstrumentPort_NewFalconAxesInstrumentPort_Invalid(
	t *testing.T,
) {
	ports := AxesInstrumentPort{
		{
			InstrumentName: "",
		},
	}

	handle, err := ports.NewFalconAxesInstrumentPort()
	if err != nil {
		assert.Nil(t, handle)
	}
}

func TestAxesInstrumentPortFromFalcon(
	t *testing.T,
) {
	p1 := InstrumentPort{
		InstrumentName: "DAC1",
	}

	p2 := InstrumentPort{
		InstrumentName: "DAC2",
	}

	raw1, err := p1.NewFalconInstrumentPort()
	require.NoError(t, err)

	raw2, err := p2.NewFalconInstrumentPort()
	require.NoError(t, err)

	reader := &mockAxesInstrumentPortReader{
		items: []*instrumentport.Handle{
			raw1,
			raw2,
		},
	}

	result, err := AxesInstrumentPortFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		AxesInstrumentPort{
			p1,
			p2,
		},
		result,
	)
}

func TestAxesInstrumentPortFromFalcon_ItemsError(
	t *testing.T,
) {
	expected := errors.New("items failed")

	reader := &mockAxesInstrumentPortReader{
		itemsErr: expected,
	}

	result, err := AxesInstrumentPortFromFalcon(reader)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Empty(
		t,
		result,
	)
}

func TestAxesInstrumentPort_RoundTrip(
	t *testing.T,
) {
	original := AxesInstrumentPort{
		{
			InstrumentName: "DAC1",
		},
		{
			InstrumentName: "DAC2",
		},
	}

	handle, err := original.NewFalconAxesInstrumentPort()
	require.NoError(t, err)
	defer handle.Close()

	result, err := AxesInstrumentPortFromFalcon(handle)

	require.NoError(t, err)

	assert.Equal(
		t,
		original,
		result,
	)
}
