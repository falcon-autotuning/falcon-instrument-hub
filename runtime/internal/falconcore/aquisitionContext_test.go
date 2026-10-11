package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAcquisitionContextReader struct {
	conn           *connection.Handle
	connErr        error
	unit           *symbolunit.Handle
	unitErr        error
	instrumentType instrument.Instrument
	instrumentErr  error

	connectionCalls int
	unitsCalls      int
	typeCalls       int
}

func (m *mockAcquisitionContextReader) Connection() (*connection.Handle, error) {
	m.connectionCalls++
	return m.conn, m.connErr
}

func (m *mockAcquisitionContextReader) Units() (*symbolunit.Handle, error) {
	m.unitsCalls++
	return m.unit, m.unitErr
}

func (m *mockAcquisitionContextReader) InstrumentType() (instrument.Instrument, error) {
	m.typeCalls++
	return m.instrumentType, m.instrumentErr
}

func TestAcquisitionContext_NewFalconAcquisitionContext(t *testing.T) {
	ctx := AcquisitionContext{
		Connection: Connection{
			Name: "P1",
			Type: PlungerGate,
		},
		Unit:           Volt,
		InstrumentType: instrument.Amnmeter,
	}

	handle, err := ctx.NewFalconAcquisitionContext()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestAcquisitionContext_NewFalconAcquisitionContext_ConnectionError(
	t *testing.T,
) {
	ctx := AcquisitionContext{
		Connection: Connection{
			Name: "P1",
			Type: ConnectionType(999),
		},
		Unit:           Volt,
		InstrumentType: instrument.Amnmeter,
	}

	handle, err := ctx.NewFalconAcquisitionContext()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestAcquisitionContext_NewFalconAcquisitionContext_UnitError(
	t *testing.T,
) {
	ctx := AcquisitionContext{
		Connection: Connection{
			Name: "P1",
			Type: PlungerGate,
		},
		Unit:           Unit(999),
		InstrumentType: instrument.Amnmeter,
	}

	handle, err := ctx.NewFalconAcquisitionContext()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestAcquisitionContextFromFalcon_ConnectionError(t *testing.T) {
	expectedErr := errors.New("connection failed")

	reader := &mockAcquisitionContextReader{
		connErr: expectedErr,
	}

	out, err := AcquisitionContextFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, AcquisitionContext{}, out)

	assert.Equal(t, 1, reader.connectionCalls)
	assert.Equal(t, 0, reader.unitsCalls)
	assert.Equal(t, 0, reader.typeCalls)
}

func TestAcquisitionContextFromFalcon_UnitsError(t *testing.T) {
	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer conn.Close()

	expectedErr := errors.New("units failed")

	reader := &mockAcquisitionContextReader{
		conn:    conn,
		unitErr: expectedErr,
	}

	out, err := AcquisitionContextFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, AcquisitionContext{}, out)

	assert.Equal(t, 1, reader.connectionCalls)
	assert.Equal(t, 1, reader.unitsCalls)
	assert.Equal(t, 0, reader.typeCalls)
}

func TestAcquisitionContextFromFalcon_InstrumentTypeError(t *testing.T) {
	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer conn.Close()

	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer unit.Close()

	expectedErr := errors.New("instrument failed")

	reader := &mockAcquisitionContextReader{
		conn:          conn,
		unit:          unit,
		instrumentErr: expectedErr,
	}

	out, err := AcquisitionContextFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)

	assert.Equal(t,
		AcquisitionContext{
			Connection: Connection{
				Name: "P1",
				Type: PlungerGate,
			},
			Unit: Volt,
		},
		out,
	)

	assert.Equal(t, 1, reader.connectionCalls)
	assert.Equal(t, 1, reader.unitsCalls)
	assert.Equal(t, 1, reader.typeCalls)
}

func TestAcquisitionContextFromFalcon_Success(t *testing.T) {
	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer conn.Close()

	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer unit.Close()

	reader := &mockAcquisitionContextReader{
		conn:           conn,
		unit:           unit,
		instrumentType: instrument.Amnmeter,
	}

	out, err := AcquisitionContextFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(t,
		AcquisitionContext{
			Connection: Connection{
				Name: "P1",
				Type: PlungerGate,
			},
			Unit:           Volt,
			InstrumentType: instrument.Amnmeter,
		},
		out,
	)

	assert.Equal(t, 1, reader.connectionCalls)
	assert.Equal(t, 1, reader.unitsCalls)
	assert.Equal(t, 1, reader.typeCalls)
}
