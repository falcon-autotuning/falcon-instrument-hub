package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/port-transforms/porttransform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPortTransformsReader struct {
	itemsHandle *listporttransform.Handle
	itemsErr    error
}

func (m *mockPortTransformsReader) Items() (*listporttransform.Handle, error) {
	return m.itemsHandle, m.itemsErr
}

func TestPortTransformsFromFalcon_Empty(
	t *testing.T,
) {
	list, err := listporttransform.NewEmpty()
	require.NoError(t, err)
	defer list.Close()

	reader := &mockPortTransformsReader{
		itemsHandle: list,
	}

	result, err := PortTransformsFromFalcon(reader)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestPortTransformsFromFalcon_Single(
	t *testing.T,
) {
	port := InstrumentPort{
		InstrumentName: "DAC",
	}

	rawPort, err := port.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort.Close()

	rawTransform, err := porttransform.NewIdentityTransform(
		rawPort,
	)
	require.NoError(t, err)
	defer rawTransform.Close()

	list, err := listporttransform.New(
		[]*porttransform.Handle{
			rawTransform,
		},
	)
	require.NoError(t, err)
	defer list.Close()

	reader := &mockPortTransformsReader{
		itemsHandle: list,
	}

	result, err := PortTransformsFromFalcon(reader)

	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(
		t,
		port,
		result[0].Port,
	)

	assert.NotNil(
		t,
		result[0].AnalyticEvaluator,
	)
}

func TestPortTransformsFromFalcon_Multiple(
	t *testing.T,
) {
	port1 := InstrumentPort{
		InstrumentName: "DAC1",
	}

	port2 := InstrumentPort{
		InstrumentName: "DAC2",
	}

	rawPort1, err := port1.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort1.Close()

	rawPort2, err := port2.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort2.Close()

	transform1, err := porttransform.NewIdentityTransform(
		rawPort1,
	)
	require.NoError(t, err)
	defer transform1.Close()

	transform2, err := porttransform.NewIdentityTransform(
		rawPort2,
	)
	require.NoError(t, err)
	defer transform2.Close()

	list, err := listporttransform.New(
		[]*porttransform.Handle{
			transform1,
			transform2,
		},
	)
	require.NoError(t, err)
	defer list.Close()

	reader := &mockPortTransformsReader{
		itemsHandle: list,
	}

	result, err := PortTransformsFromFalcon(reader)

	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, port1, result[0].Port)
	assert.Equal(t, port2, result[1].Port)
}

func TestPortTransformsFromFalcon_ItemsError(
	t *testing.T,
) {
	expected := errors.New("items failed")

	reader := &mockPortTransformsReader{
		itemsErr: expected,
	}

	result, err := PortTransformsFromFalcon(reader)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Nil(
		t,
		result,
	)
}

func TestPortTransformsFromFalcon_ListItemsError(
	t *testing.T,
) {
	reader := &mockPortTransformsReader{
		itemsHandle: &listporttransform.Handle{},
	}

	result, err := PortTransformsFromFalcon(reader)

	require.Error(t, err)
	assert.Nil(t, result)
}
