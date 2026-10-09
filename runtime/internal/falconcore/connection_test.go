package falconcore

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeConnectionReader struct {
	name    string
	nameErr error

	plunger    bool
	plungerErr error

	barrier    bool
	barrierErr error

	reservoir    bool
	reservoirErr error

	screening    bool
	screeningErr error

	ohmic    bool
	ohmicErr error
}

func (f *fakeConnectionReader) Name() (string, error) {
	return f.name, f.nameErr
}

func (f *fakeConnectionReader) IsPlungerGate() (bool, error) {
	return f.plunger, f.plungerErr
}

func (f *fakeConnectionReader) IsBarrierGate() (bool, error) {
	return f.barrier, f.barrierErr
}

func (f *fakeConnectionReader) IsScreeningGate() (bool, error) {
	return f.screening, f.screeningErr
}

func (f *fakeConnectionReader) IsReservoirGate() (bool, error) {
	return f.reservoir, f.reservoirErr
}

func (f *fakeConnectionReader) IsOhmic() (bool, error) {
	return f.ohmic, f.ohmicErr
}

var _ ConnectionReader = (*fakeConnectionReader)(nil)

func TestConnection_NewFalconConnection(t *testing.T) {
	tests := []struct {
		name string
		conn Connection
	}{
		{
			name: "BarrierGate",
			conn: Connection{
				Name: "B1",
				Type: BarrierGate,
			},
		},
		{
			name: "PlungerGate",
			conn: Connection{
				Name: "P1",
				Type: PlungerGate,
			},
		},
		{
			name: "ReservoirGate",
			conn: Connection{
				Name: "R1",
				Type: ReservoirGate,
			},
		},
		{
			name: "ScreeningGate",
			conn: Connection{
				Name: "S1",
				Type: ScreeningGate,
			},
		},
		{
			name: "Ohmic",
			conn: Connection{
				Name: "O1",
				Type: Ohmic,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handle, err := tt.conn.NewFalconConnection()

			require.NoError(t, err)
			require.NotNil(t, handle)

			assert.NoError(t, handle.Close())
		})
	}
}

func TestConnection_NewFalconConnection_InvalidType(t *testing.T) {
	handle, err := Connection{
		Name: "bad",
		Type: ConnectionType(999),
	}.NewFalconConnection()

	require.Error(t, err)
	assert.Nil(t, handle)
	assert.Contains(t, err.Error(), "unsupported connection type")
}

func TestConnectionFromFalcon_PlungerGate(t *testing.T) {
	conn, err := ConnectionFromFalcon(
		&fakeConnectionReader{
			name:    "P1",
			plunger: true,
		},
	)

	require.NoError(t, err)

	assert.Equal(t,
		Connection{
			Name: ConnectionName("P1"),
			Type: PlungerGate,
		},
		conn,
	)
}

func TestConnectionFromFalcon_BarrierGate(t *testing.T) {
	conn, err := ConnectionFromFalcon(
		&fakeConnectionReader{
			name:    "B1",
			barrier: true,
		},
	)

	require.NoError(t, err)
	assert.Equal(t, BarrierGate, conn.Type)
}

func TestConnectionFromFalcon_ReservoirGate(t *testing.T) {
	conn, err := ConnectionFromFalcon(
		&fakeConnectionReader{
			name:      "R1",
			reservoir: true,
		},
	)

	require.NoError(t, err)
	assert.Equal(t, ReservoirGate, conn.Type)
}

func TestConnectionFromFalcon_ScreeningGate(t *testing.T) {
	conn, err := ConnectionFromFalcon(
		&fakeConnectionReader{
			name:      "S1",
			screening: true,
		},
	)

	require.NoError(t, err)
	assert.Equal(t, ScreeningGate, conn.Type)
}

func TestConnectionFromFalcon_Ohmic(t *testing.T) {
	conn, err := ConnectionFromFalcon(
		&fakeConnectionReader{
			name:  "O1",
			ohmic: true,
		},
	)

	require.NoError(t, err)
	assert.Equal(t, Ohmic, conn.Type)
}

func TestConnectionFromFalcon_NameError(t *testing.T) {
	conn, err := ConnectionFromFalcon(
		&fakeConnectionReader{
			nameErr: errors.New("name error"),
		},
	)

	require.Error(t, err)
	assert.Equal(t, Connection{}, conn)
}

func TestConnectionFromFalcon_NoMatchingType(t *testing.T) {
	conn, err := ConnectionFromFalcon(
		&fakeConnectionReader{
			name: "X1",
		},
	)

	require.Error(t, err)
	assert.Equal(t, Connection{}, conn)
	assert.Contains(t, err.Error(), "unable to select connection type")
}
