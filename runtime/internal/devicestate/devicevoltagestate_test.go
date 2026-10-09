package devicestate

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDeviceVoltageStateReader struct {
	value    float64
	valueErr error

	unit    *symbolunit.Handle
	unitErr error

	connection    *connection.Handle
	connectionErr error
}

func (f *fakeDeviceVoltageStateReader) Value() (float64, error) {
	return f.value, f.valueErr
}

func (f *fakeDeviceVoltageStateReader) Unit() (*symbolunit.Handle, error) {
	return f.unit, f.unitErr
}

func (f *fakeDeviceVoltageStateReader) Connection() (*connection.Handle, error) {
	return f.connection, f.connectionErr
}

func TestDeviceVoltageState_NewFalconDeviceVoltageState(t *testing.T) {
	conn := falconcore.Connection{
		Name: "P1",
	}

	state := DeviceVoltageState{
		Quantity: Quantity{
			Value: 1.25,
			Unit:  Volt,
		},
		Connection: conn,
	}

	handle, err := state.NewFalconDeviceVoltageState()

	require.NoError(t, err)
	require.NotNil(t, handle)

	defer handle.Close()

	value, err := handle.Value()
	require.NoError(t, err)

	assert.Equal(t, 1.25, value)
}

func TestDeviceVoltageState_NewFalconDeviceVoltageState_BadUnit(t *testing.T) {
	state := DeviceVoltageState{
		Quantity: Quantity{
			Value: 1,
			Unit:  Unit(999999),
		},
		Connection: falconcore.Connection{
			Name: "P1",
		},
	}

	handle, err := state.NewFalconDeviceVoltageState()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestDeviceVoltageStateFromFalcon_Success(t *testing.T) {
	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer unit.Close()

	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer conn.Close()

	reader := &fakeDeviceVoltageStateReader{
		value:      3.14,
		unit:       unit,
		connection: conn,
	}

	actual, err := DeviceVoltageStateFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(t, 3.14, actual.Value)
	assert.Equal(t, Volt, actual.Unit)

	assert.Equal(
		t,
		falconcore.Connection{
			Name: "P1",
			Type: falconcore.PlungerGate,
		},
		actual.Connection,
	)
}

func TestDeviceVoltageStateFromFalcon_ValueError(t *testing.T) {
	reader := &fakeDeviceVoltageStateReader{
		valueErr: errors.New("value failure"),
	}

	actual, err := DeviceVoltageStateFromFalcon(reader)

	require.Error(t, err)
	assert.Equal(t, DeviceVoltageState{}, actual)
}

func TestDeviceVoltageStateFromFalcon_UnitError(t *testing.T) {
	reader := &fakeDeviceVoltageStateReader{
		value:   1.0,
		unitErr: errors.New("unit failure"),
	}

	actual, err := DeviceVoltageStateFromFalcon(reader)

	require.Error(t, err)
	assert.Equal(t, DeviceVoltageState{}, actual)
}

func TestDeviceVoltageStateFromFalcon_ConnectionError(t *testing.T) {
	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer unit.Close()

	reader := &fakeDeviceVoltageStateReader{
		value:         1.0,
		unit:          unit,
		connectionErr: errors.New("connection failure"),
	}

	actual, err := DeviceVoltageStateFromFalcon(reader)

	require.Error(t, err)
	assert.Equal(t, DeviceVoltageState{}, actual)
}
