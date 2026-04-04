// Package serverinterpreter provides tests for quantum dot device configuration
// parsing and Lua script generation.
package serverinterpreter

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Device Config Loading Tests
// =============================================================================

func TestQuantumDotDeviceConfig_LoadFromFile(t *testing.T) {
	// Get the absolute path to the config file relative to this test file
	configPath := filepath.Join("..", "..", "..", "test_data", "dummy_one_charge_sensor_quantum_dot_device.yaml")

	t.Run("load device config from YAML file", func(t *testing.T) {
		config, err := LoadQuantumDotDeviceConfig(configPath)
		require.NoError(t, err)
		require.NotNil(t, config)

		// Verify global config
		assert.Equal(t, "S1;S2;S3", config.Global.ScreeningGates)
		assert.Equal(t, "P1;P2;P3", config.Global.PlungerGates)
		assert.Equal(t, "O1;O2;O3;O4", config.Global.Ohmics)
		assert.Equal(t, "B1;B2;B3;B4;B5", config.Global.BarrierGates)
		assert.Equal(t, "R1;R2;R3;R4", config.Global.ReservoirGates)
		assert.Equal(t, 2, config.Global.NumUniqueChannels)
	})

	t.Run("verify groups", func(t *testing.T) {
		config, err := LoadQuantumDotDeviceConfig(configPath)
		require.NoError(t, err)

		// Should have 2 groups
		assert.Len(t, config.Groups, 2)

		// Verify group1 (double dot)
		group1 := config.Groups["group1"]
		assert.Equal(t, "I_O1", group1.Name)
		assert.Equal(t, 2, group1.NumDots)
		assert.Equal(t, "P1;P2", group1.PlungerGates)
		assert.Equal(t, "B1;B2;B3", group1.BarrierGates)

		// Verify group2 (charge sensor)
		group2 := config.Groups["group2"]
		assert.Equal(t, "I_O3", group2.Name)
		assert.Equal(t, 1, group2.NumDots)
		assert.Equal(t, "P3", group2.PlungerGates)
	})

	t.Run("verify wiring DC", func(t *testing.T) {
		config, err := LoadQuantumDotDeviceConfig(configPath)
		require.NoError(t, err)

		// All gates should have wiring
		assert.Len(t, config.WiringDC, 19) // 3S + 3P + 4O + 5B + 4R

		wiring, ok := config.GetWiring("P1")
		require.True(t, ok)
		assert.Equal(t, 1000.0, wiring.Resistance)
		assert.Equal(t, 1e-12, wiring.Capacitance)
	})

	t.Run("gate list helpers", func(t *testing.T) {
		config, err := LoadQuantumDotDeviceConfig(configPath)
		require.NoError(t, err)

		plungers := config.AllPlungerGates()
		assert.Equal(t, []string{"P1", "P2", "P3"}, plungers)

		barriers := config.AllBarrierGates()
		assert.Equal(t, []string{"B1", "B2", "B3", "B4", "B5"}, barriers)

		ohmics := config.AllOhmics()
		assert.Equal(t, []string{"O1", "O2", "O3", "O4"}, ohmics)
	})
}

// =============================================================================
// Measurement Setup Tests
// =============================================================================

func TestQuantumDotMeasurementSetup(t *testing.T) {
	configPath := filepath.Join("..", "..", "..", "test_data", "dummy_one_charge_sensor_quantum_dot_device.yaml")
	config, err := LoadQuantumDotDeviceConfig(configPath)
	require.NoError(t, err)

	t.Run("create measurement setup", func(t *testing.T) {
		setup := NewQuantumDotMeasurementSetup(config, "QDAC1", "DMM1")
		require.NotNil(t, setup)

		// All gates should be mapped
		target, ok := setup.GateMapping.Get("P1")
		assert.True(t, ok)
		assert.Equal(t, "QDAC1", target.Id)
		assert.Greater(t, target.Channel, 0)

		// Measurement channels should be set up (one per group)
		assert.Len(t, setup.MeasurementChannels, 2)

		// Verify both groups are represented (order may vary due to map iteration)
		names := make([]string, len(setup.MeasurementChannels))
		for i, ch := range setup.MeasurementChannels {
			names[i] = ch.Name
		}
		assert.Contains(t, names, "I_O1")
		assert.Contains(t, names, "I_O3")
	})

	t.Run("build set voltage requests", func(t *testing.T) {
		setup := NewQuantumDotMeasurementSetup(config, "QDAC1", "DMM1")

		voltages := map[string]float64{
			"P1": -0.5,
			"P2": -0.6,
			"B1": -1.0,
		}

		requests := setup.BuildSetVoltageRequests(voltages)
		assert.Len(t, requests, 3)

		// Verify the requests are correct
		for _, req := range requests {
			assert.Equal(t, "QDAC1", req.Setter.Id)
			assert.Greater(t, req.Setter.Channel, 0)
		}
	})

	t.Run("build 1D sweep data", func(t *testing.T) {
		setup := NewQuantumDotMeasurementSetup(config, "QDAC1", "DMM1")

		staticVoltages := map[string]float64{
			"P2": -0.5, // Keep P2 fixed
			"B1": -1.0,
			"B2": -1.0,
			"B3": -1.0,
		}

		sweepData, err := setup.Build1DSweepData(
			"P1",      // Sweep P1
			-1.0, 0.0, // From -1V to 0V
			101, // 101 points
			staticVoltages,
			5.0, // 5ms settling time
		)
		require.NoError(t, err)

		assert.Equal(t, "sweep_P1", sweepData.MeasurementName)
		assert.Equal(t, "P1", sweepData.SweepGate)
		assert.Equal(t, -1.0, sweepData.StartVoltage)
		assert.Equal(t, 0.0, sweepData.StopVoltage)
		assert.Equal(t, 101, sweepData.NumPoints)
		assert.Equal(t, 5.0, sweepData.SettlingTimeMs)
		assert.Len(t, sweepData.StaticSetters, 4)
		assert.Len(t, sweepData.GetVoltageRequests, 2) // Both measurement channels
	})
}

// =============================================================================
// Lua Script Generation from Device Config (Deprecated — tests removed)
// =============================================================================

// Tests for ScriptGenerator-based code generation have been removed.
// ScriptGenerator was deprecated in favour of user-provided Lua scripts
// dispatched via MeasurementOrchestrator / ScriptDispatcher.
// See docs/LUA_SCRIPT_AUTHORING.md
