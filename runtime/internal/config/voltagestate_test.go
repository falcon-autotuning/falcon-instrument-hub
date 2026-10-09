package config

import (
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_String(t *testing.T) {
	tests := []struct {
		unit     Unit
		expected string
	}{
		{Volt, "volt"},
		{Millivolt, "millivolt"},
		{Microvolt, "microvolt"},
		{Unit(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.unit.String())
		})
	}
}

func TestParseUnit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Unit
	}{
		{"VoltLong", "volt", Volt},
		{"VoltShort", "v", Volt},
		{"MillivoltLong", "millivolt", Millivolt},
		{"MillivoltShort", "mv", Millivolt},
		{"MicrovoltLong", "microvolt", Microvolt},
		{"MicrovoltShort", "uv", Microvolt},
		{"MicrovoltUnicode", "µv", Microvolt},
		{"CaseInsensitive", "MV", Millivolt},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unit, err := ParseUnit(tt.input)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, unit)
		})
	}
}

func TestParseUnit_Invalid(t *testing.T) {
	unit, err := ParseUnit("bad-unit")

	require.Error(t, err)
	assert.Equal(t, Volt, unit)
	assert.Contains(t, err.Error(), "unknown unit")
}

func TestVoltageState_FindMatchingDeviceName(t *testing.T) {
	state := VoltageState{
		Connection: "P1",
	}

	conns := []falconcore.Connection{
		{
			Name: "P1",
			Type: falconcore.PlungerGate,
		},
		{
			Name: "P2",
			Type: falconcore.PlungerGate,
		},
	}

	name, err := state.FindMatchingDeviceName(conns)

	require.NoError(t, err)
	assert.Equal(t, falconcore.ConnectionName("P1"), name)
}

func TestVoltageState_FindMatchingDeviceName_NotFound(t *testing.T) {
	state := VoltageState{
		Connection: "Missing",
	}

	conns := []falconcore.Connection{
		{
			Name: "P1",
			Type: falconcore.PlungerGate,
		},
	}

	name, err := state.FindMatchingDeviceName(conns)

	require.Error(t, err)
	assert.Equal(t, falconcore.ConnectionName(""), name)
	assert.Contains(t, err.Error(), "not present in the device config")
}

func TestNewZeroState(t *testing.T) {
	state := NewZeroState("P1")

	assert.Equal(t,
		VoltageState{
			Voltage:    0.0,
			Connection: "P1",
			Unit:       Volt,
		},
		state,
	)
}

func TestVoltageStates_FindMatchingDeviceNames(t *testing.T) {
	states := VoltageStates{
		{Connection: "P1"},
		{Connection: "P2"},
	}

	conns := []falconcore.Connection{
		{Name: "P1"},
		{Name: "P2"},
		{Name: "P3"},
	}

	names, err := states.FindMatchingDeviceNames(conns)

	require.NoError(t, err)
	assert.Equal(t,
		[]falconcore.ConnectionName{
			"P1",
			"P2",
		},
		names,
	)
}

func TestVoltageStates_FindMatchingDeviceNames_Error(t *testing.T) {
	states := VoltageStates{
		{Connection: "P1"},
		{Connection: "Missing"},
	}

	conns := []falconcore.Connection{
		{Name: "P1"},
	}

	names, err := states.FindMatchingDeviceNames(conns)

	require.Error(t, err)
	assert.Nil(t, names)
}

func TestVoltageStates_DefaultToZero_NoMissingConnections(t *testing.T) {
	states := VoltageStates{
		{
			Connection: "P1",
			Voltage:    1.0,
			Unit:       Volt,
		},
		{
			Connection: "P2",
			Voltage:    2.0,
			Unit:       Millivolt,
		},
	}

	conns := falconcore.Connections{
		{Name: "P1"},
		{Name: "P2"},
	}

	err := states.DefaultToZero(conns)

	require.NoError(t, err)
	assert.Len(t, states, 2)
}

func TestVoltageStates_DefaultToZero_AddsMissingConnections(t *testing.T) {
	states := VoltageStates{
		{
			Connection: "P1",
			Voltage:    1.23,
			Unit:       Volt,
		},
	}

	conns := falconcore.Connections{
		{Name: "P1"},
		{Name: "P2"},
		{Name: "P3"},
	}

	err := states.DefaultToZero(conns)

	require.NoError(t, err)

	assert.Len(t, states, 3)

	assert.Contains(t, states,
		VoltageState{
			Voltage:    0,
			Connection: "P2",
			Unit:       Volt,
		},
	)

	assert.Contains(t, states,
		VoltageState{
			Voltage:    0,
			Connection: "P3",
			Unit:       Volt,
		},
	)
}

func TestVoltageStates_DefaultToZero_Error(t *testing.T) {
	states := VoltageStates{
		{
			Connection: "Missing",
		},
	}

	conns := falconcore.Connections{
		{Name: "P1"},
	}

	err := states.DefaultToZero(conns)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not present in the device config")
}
