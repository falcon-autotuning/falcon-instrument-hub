package devicestate

import (
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartup(t *testing.T) {
	globalManager = nil
	defer Close()

	initial := DeviceVoltageStates{
		"P1": {
			Connection: falconcore.Connection{
				Name: "P1",
				Type: falconcore.PlungerGate,
			},
			Quantity: Quantity{
				Value: 1.23,
				Unit:  Volt,
			},
		},
	}

	err := Startup(initial)

	require.NoError(t, err)
	require.NotNil(t, globalManager)

	state := globalManager.Snapshot()

	assert.Equal(t, initial, state)
}

func TestStartup_CopiesInput(t *testing.T) {
	globalManager = nil
	defer Close()

	initial := DeviceVoltageStates{
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
	}

	require.NoError(t, Startup(initial))

	initial["P2"] = DeviceVoltageState{
		Connection: falconcore.Connection{
			Name: "P2",
			Type: falconcore.BarrierGate,
		},
		Quantity: Quantity{
			Value: 2,
			Unit:  Volt,
		},
	}

	snapshot := globalManager.Snapshot()

	_, exists := snapshot["P2"]

	assert.False(t, exists)
	assert.Len(t, snapshot, 1)
}

func TestStartup_AlreadyInitialized(t *testing.T) {
	globalManager = nil
	defer Close()

	require.NoError(
		t,
		Startup(DeviceVoltageStates{}),
	)

	err := Startup(DeviceVoltageStates{})

	require.Error(t, err)
	assert.Equal(
		t,
		"device state manager already initialized",
		err.Error(),
	)
}

func TestManager(t *testing.T) {
	globalManager = nil
	defer Close()

	require.NoError(
		t,
		Startup(DeviceVoltageStates{}),
	)

	manager := Manager()

	require.NotNil(t, manager)
	assert.Same(t, globalManager, manager)
}

func TestManager_Nil(t *testing.T) {
	globalManager = nil

	manager := Manager()

	assert.Nil(t, manager)
}

func TestClose(t *testing.T) {
	globalManager = nil

	require.NoError(
		t,
		Startup(
			DeviceVoltageStates{
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
		),
	)

	require.NotNil(t, globalManager)

	Close()

	assert.Nil(t, globalManager)
}

func TestClose_NilManager(t *testing.T) {
	globalManager = nil

	assert.NotPanics(t, func() {
		Close()
	})
}

func TestClose_ClearsManagerState(t *testing.T) {
	globalManager = nil

	require.NoError(
		t,
		Startup(
			DeviceVoltageStates{
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
		),
	)

	manager := globalManager

	Close()

	assert.True(t, manager.closed)
	assert.Empty(t, manager.state)
	assert.Nil(t, globalManager)
}
