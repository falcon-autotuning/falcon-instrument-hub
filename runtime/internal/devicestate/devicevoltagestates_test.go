package devicestate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeviceVoltageStates_NewFalconDeviceVoltageStates_Empty(
	t *testing.T,
) {
	states := DeviceVoltageStates{}

	handle, err := states.NewFalconDeviceVoltageStates()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	size, err := handle.Size()
	require.NoError(t, err)

	assert.Equal(t, uint64(0), size)
}

func TestDeviceVoltageStates_NewFalconDeviceVoltageStates_Single(
	t *testing.T,
) {
	states := DeviceVoltageStates{
		"P1": {
			Connection: Connection{
				Name: "P1",
				Type: PlungerGate,
			},
			Quantity: Quantity{
				Value: 1.5,
				Unit:  Volt,
			},
		},
	}

	handle, err := states.NewFalconDeviceVoltageStates()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	size, err := handle.Size()
	require.NoError(t, err)

	assert.Equal(t, uint64(1), size)
}

func TestDeviceVoltageStates_NewFalconDeviceVoltageStates_InvalidConnection(
	t *testing.T,
) {
	states := DeviceVoltageStates{
		"P1": {
			Connection: Connection{
				Name: "P1",
				Type: ConnectionType(999),
			},
			Quantity: Quantity{
				Value: 1,
				Unit:  Volt,
			},
		},
	}

	handle, err := states.NewFalconDeviceVoltageStates()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestDeviceVoltageStates_NewFalconDeviceVoltageStates_InvalidUnit(
	t *testing.T,
) {
	states := DeviceVoltageStates{
		"P1": {
			Connection: Connection{
				Name: "P1",
				Type: PlungerGate,
			},
			Quantity: Quantity{
				Value: 1,
				Unit:  Unit(999),
			},
		},
	}

	handle, err := states.NewFalconDeviceVoltageStates()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestDeviceVoltageStatesFromFalcon_Empty(t *testing.T) {
	raw, err := DeviceVoltageStates{}.NewFalconDeviceVoltageStates()
	require.NoError(t, err)
	defer raw.Close()

	result, err := DeviceVoltageStatesFromFalcon(raw)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestDeviceVoltageStates_RoundTrip(
	t *testing.T,
) {
	original := DeviceVoltageStates{
		"P1": {
			Connection: Connection{
				Name: "P1",
				Type: PlungerGate,
			},
			Quantity: Quantity{
				Value: 1.25,
				Unit:  Volt,
			},
		},
		"B1": {
			Connection: Connection{
				Name: "B1",
				Type: BarrierGate,
			},
			Quantity: Quantity{
				Value: 500,
				Unit:  Millivolt,
			},
		},
	}

	raw, err := original.NewFalconDeviceVoltageStates()
	require.NoError(t, err)
	defer raw.Close()

	result, err := DeviceVoltageStatesFromFalcon(raw)
	require.NoError(t, err)

	require.Len(t, result, len(original))

	for name, expected := range original {
		actual, ok := result[name]

		require.True(t, ok)

		assert.Equal(
			t,
			expected.Connection,
			actual.Connection,
		)

		assert.Equal(
			t,
			expected.Quantity.Unit,
			actual.Quantity.Unit,
		)

		assert.InDelta(
			t,
			expected.Quantity.Value,
			actual.Quantity.Value,
			1e-12,
		)
	}
}
