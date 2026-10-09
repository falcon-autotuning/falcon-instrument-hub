package devicestate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

func TestDeviceStateManager_UpdatePort(t *testing.T) {
	mgr := &DeviceStateManager{
		state: DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
					Type: falconcore.PlungerGate,
				},
				Quantity: Quantity{
					Value: 1,
					Unit:  Volt,
				},
			},
		},
	}

	err := mgr.UpdatePort(
		"P1",
		Quantity{
			Value: 2,
			Unit:  Millivolt,
		},
	)

	require.NoError(t, err)

	actual, ok := mgr.state["P1"]

	require.True(t, ok)

	assert.Equal(
		t,
		falconcore.Connection{
			Name: "P1",
			Type: falconcore.PlungerGate,
		},
		actual.Connection,
	)

	assert.Equal(
		t,
		Quantity{
			Value: 2,
			Unit:  Millivolt,
		},
		actual.Quantity,
	)
}

func TestDeviceStateManager_Port_Found(t *testing.T) {
	expected := Quantity{
		Value: 123,
		Unit:  Volt,
	}

	mgr := &DeviceStateManager{
		state: DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
					Type: falconcore.PlungerGate,
				},
				Quantity: expected,
			},
		},
	}

	actual, ok := mgr.Port("P1")

	require.True(t, ok)
	assert.Equal(t, expected, actual)
}

func TestDeviceStateManager_Port_NotFound(t *testing.T) {
	mgr := &DeviceStateManager{
		state: DeviceVoltageStates{},
	}

	actual, ok := mgr.Port("missing")

	assert.False(t, ok)
	assert.Equal(t, Quantity{}, actual)
}

func TestDeviceStateManager_Snapshot(t *testing.T) {
	mgr := &DeviceStateManager{
		state: DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
					Type: falconcore.PlungerGate,
				},
				Quantity: Quantity{
					Value: 1,
					Unit:  Volt,
				},
			},
		},
	}

	snapshot := mgr.Snapshot()

	require.Len(t, snapshot, 1)

	assert.Equal(
		t,
		mgr.state["P1"],
		snapshot["P1"],
	)
}

func TestDeviceStateManager_Snapshot_IsCopy(t *testing.T) {
	mgr := &DeviceStateManager{
		state: DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
					Type: falconcore.PlungerGate,
				},
				Quantity: Quantity{
					Value: 1,
					Unit:  Volt,
				},
			},
		},
	}

	snapshot := mgr.Snapshot()

	snapshot["P2"] = DeviceVoltageState{
		Connection: falconcore.Connection{
			Name: "P2",
			Type: falconcore.BarrierGate,
		},
		Quantity: Quantity{
			Value: 5,
			Unit:  Volt,
		},
	}

	_, exists := mgr.state["P2"]

	assert.False(t, exists)
	assert.Len(t, mgr.state, 1)
}

func TestDeviceStateManager_Clear(t *testing.T) {
	mgr := &DeviceStateManager{
		state: DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
					Type: falconcore.PlungerGate,
				},
				Quantity: Quantity{
					Value: 1,
					Unit:  Volt,
				},
			},
		},
	}

	mgr.Clear()

	assert.Empty(t, mgr.state)
}

func TestDeviceStateManager_Close(t *testing.T) {
	mgr := &DeviceStateManager{
		state: DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
					Type: falconcore.PlungerGate,
				},
				Quantity: Quantity{
					Value: 1,
					Unit:  Volt,
				},
			},
		},
	}

	mgr.Close()

	assert.True(t, mgr.closed)
	assert.Empty(t, mgr.state)
}

func TestDeviceStateManager_Close_AlreadyClosed(t *testing.T) {
	mgr := &DeviceStateManager{
		state: DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
					Type: falconcore.PlungerGate,
				},
				Quantity: Quantity{
					Value: 1,
					Unit:  Volt,
				},
			},
		},
		closed: true,
	}

	original := len(mgr.state)

	mgr.Close()

	assert.True(t, mgr.closed)
	assert.Equal(t, original, len(mgr.state))
}

func TestDeviceStateManager_UpdatePort_NewPort(t *testing.T) {
	mgr := &DeviceStateManager{
		state: DeviceVoltageStates{},
	}

	err := mgr.UpdatePort(
		"P1",
		Quantity{
			Value: 1,
			Unit:  Volt,
		},
	)

	require.NoError(t, err)

	state := mgr.state["P1"]

	assert.Equal(t, Quantity{
		Value: 1,
		Unit:  Volt,
	}, state.Quantity)

	assert.Equal(t, falconcore.Connection{}, state.Connection)
}
