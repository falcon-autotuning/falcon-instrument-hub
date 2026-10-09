package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listconnection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeConnectionsReader struct {
	list *listconnection.Handle
	err  error
}

func (f *fakeConnectionsReader) Items() (*listconnection.Handle, error) {
	return f.list, f.err
}

var _ ConnectionsReader = (*fakeConnectionsReader)(nil)

func TestConnections_NewFalconConnections_Empty(t *testing.T) {
	handle, err := Connections{}.NewFalconConnections()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestConnections_NewFalconConnections(t *testing.T) {
	expected := Connections{
		{
			Name: "P1",
			Type: PlungerGate,
		},
		{
			Name: "B1",
			Type: BarrierGate,
		},
		{
			Name: "R1",
			Type: ReservoirGate,
		},
		{
			Name: "S1",
			Type: ScreeningGate,
		},
		{
			Name: "O1",
			Type: Ohmic,
		},
	}

	handle, err := expected.NewFalconConnections()

	require.NoError(t, err)
	require.NotNil(t, handle)

	defer func() {
		assert.NoError(t, handle.Close())
	}()

	actual, err := ConnectionsFromFalcon(handle)

	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestConnections_NewFalconConnections_InvalidConnection(t *testing.T) {
	handle, err := Connections{
		{
			Name: "bad",
			Type: ConnectionType(999),
		},
	}.NewFalconConnections()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestConnectionsFromFalcon_Empty(t *testing.T) {
	handle, err := Connections{}.NewFalconConnections()

	require.NoError(t, err)

	defer func() {
		assert.NoError(t, handle.Close())
	}()

	actual, err := ConnectionsFromFalcon(handle)

	require.NoError(t, err)
	assert.Empty(t, actual)
}

func TestConnectionsFromFalcon_ItemsError(t *testing.T) {
	expectedErr := errors.New("items error")

	actual, err := ConnectionsFromFalcon(
		&fakeConnectionsReader{
			err: expectedErr,
		},
	)

	require.Error(t, err)
	assert.Equal(t, Connections{}, actual)
	assert.ErrorIs(t, err, expectedErr)
}
