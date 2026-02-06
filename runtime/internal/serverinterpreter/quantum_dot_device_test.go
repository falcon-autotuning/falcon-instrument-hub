// Package serverinterpreter provides tests for quantum dot device configuration
// parsing and Lua script generation.
package serverinterpreter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Device Config Loading Tests
// =============================================================================

func TestQuantumDotDeviceConfig_LoadFromFile(t *testing.T) {
	// Get the path to the testdata config file
	configPath := filepath.Join("..", "..", "testdata", "one_charge_sensor_quantum_dot_device.yaml")

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
	configPath := filepath.Join("..", "..", "testdata", "one_charge_sensor_quantum_dot_device.yaml")
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
			"P1",         // Sweep P1
			-1.0, 0.0,    // From -1V to 0V
			101,          // 101 points
			staticVoltages,
			5.0,          // 5ms settling time
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
// Lua Script Generation with Device Config
// =============================================================================

func TestLuaScriptGeneration_FromDeviceConfig(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "one_charge_sensor_quantum_dot_device.yaml")
	config, err := LoadQuantumDotDeviceConfig(configPath)
	require.NoError(t, err)

	setup := NewQuantumDotMeasurementSetup(config, "QDAC1", "DMM1")

	t.Run("generate DC GetSet script for all plungers", func(t *testing.T) {
		tempDir := t.TempDir()
		gen, err := NewScriptGenerator(tempDir)
		require.NoError(t, err)

		// Set all plunger gates
		plungerVoltages := map[string]float64{
			"P1": -0.5,
			"P2": -0.6,
			"P3": -0.4, // Charge sensor plunger
		}

		data := DCGetSetScriptData{
			MeasurementName:    "set_all_plungers",
			SetVoltageRequests: setup.BuildSetVoltageRequests(plungerVoltages),
			GetVoltageRequests: setup.BuildGetVoltageRequests(),
			SettlingTimeMs:     10.0,
		}

		scriptPath, err := gen.GenerateDCGetSetScript(data)
		require.NoError(t, err)

		// Verify script was created
		content, err := os.ReadFile(scriptPath)
		require.NoError(t, err)
		contentStr := string(content)

		// Validate script content
		assert.Contains(t, contentStr, "function main(ctx)")
		assert.Contains(t, contentStr, "ctx:parallel")
		assert.Contains(t, contentStr, "QDAC1.SET_VOLTAGE")
		assert.Contains(t, contentStr, "DMM1.GET_VOLTAGE")
		assert.Contains(t, contentStr, "ctx:sleep(10.000)")
		assert.Contains(t, contentStr, "set_all_plungers")

		t.Logf("Generated DC GetSet script:\n%s", contentStr)
	})

	t.Run("generate 1D plunger sweep script", func(t *testing.T) {
		tempDir := t.TempDir()
		gen, err := NewScriptGenerator(tempDir)
		require.NoError(t, err)

		// Static voltages for barriers
		staticVoltages := map[string]float64{
			"P2": -0.5,
			"B1": -0.8,
			"B2": -0.9,
			"B3": -0.8,
		}

		sweepData, err := setup.Build1DSweepData(
			"P1",
			-1.0, 0.0,
			51,
			staticVoltages,
			2.0,
		)
		require.NoError(t, err)

		scriptPath, err := gen.GenerateSweep1DScript(*sweepData)
		require.NoError(t, err)

		// Verify script was created
		content, err := os.ReadFile(scriptPath)
		require.NoError(t, err)
		contentStr := string(content)

		// Validate script content
		assert.Contains(t, contentStr, "function main(ctx)")
		assert.Contains(t, contentStr, "1D sweep")
		assert.Contains(t, contentStr, "P1")
		assert.Contains(t, contentStr, "-1.0000")
		assert.Contains(t, contentStr, "0.0000")
		assert.Contains(t, contentStr, "num_points = 51")
		assert.Contains(t, contentStr, "ctx:sleep(2.000)")
		assert.Contains(t, contentStr, "for i = 0, num_points - 1 do")
		assert.Contains(t, contentStr, "DMM1.GET_VOLTAGE")

		t.Logf("Generated 1D sweep script:\n%s", contentStr)
	})

	t.Run("generate pinch-off measurement script", func(t *testing.T) {
		tempDir := t.TempDir()
		gen, err := NewScriptGenerator(tempDir)
		require.NoError(t, err)

		// Pinch-off: Sweep barrier gate, measuring source-drain current
		// All plungers and other barriers set to allow conduction initially
		staticVoltages := map[string]float64{
			"P1": 0.0,
			"P2": 0.0,
			"B1": -0.3,
			"B3": -0.3,
		}

		sweepData, err := setup.Build1DSweepData(
			"B2",          // Sweep B2 (central barrier)
			0.0, -2.0,     // From 0V to -2V (pinching off)
			101,
			staticVoltages,
			5.0,
		)
		require.NoError(t, err)
		sweepData.MeasurementName = "B2_pinchoff"

		scriptPath, err := gen.GenerateSweep1DScript(*sweepData)
		require.NoError(t, err)

		content, err := os.ReadFile(scriptPath)
		require.NoError(t, err)
		contentStr := string(content)

		assert.Contains(t, contentStr, "B2")
		assert.Contains(t, contentStr, "pinchoff")
		assert.Contains(t, contentStr, "0.0000")
		assert.Contains(t, contentStr, "-2.0000")

		t.Logf("Generated pinch-off script:\n%s", contentStr)
	})
}

// =============================================================================
// Script Validation Tests
// =============================================================================

func TestLuaScriptValidation(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "one_charge_sensor_quantum_dot_device.yaml")
	config, err := LoadQuantumDotDeviceConfig(configPath)
	require.NoError(t, err)

	setup := NewQuantumDotMeasurementSetup(config, "QDAC1", "DMM1")
	tempDir := t.TempDir()
	gen, err := NewScriptGenerator(tempDir)
	require.NoError(t, err)

	t.Run("generated script has valid Lua structure", func(t *testing.T) {
		sweepData, err := setup.Build1DSweepData(
			"P1", -1.0, 0.0, 10,
			map[string]float64{"B1": -1.0},
			1.0,
		)
		require.NoError(t, err)

		scriptPath, err := gen.GenerateSweep1DScript(*sweepData)
		require.NoError(t, err)

		content, err := os.ReadFile(scriptPath)
		require.NoError(t, err)
		contentStr := string(content)

		// Check that main function exists and has proper structure
		assert.Contains(t, contentStr, "function main(ctx)")
		assert.Contains(t, contentStr, "return results")

		// Check for proper for loop structure
		assert.Contains(t, contentStr, "for i = 0, num_points - 1 do")

		// Check for required local variable declarations
		assert.Contains(t, contentStr, "local results = {}")
		assert.Contains(t, contentStr, "local start_voltage")
		assert.Contains(t, contentStr, "local voltage = start_voltage")
	})

	t.Run("script handles edge cases", func(t *testing.T) {
		// Single point sweep (effectively just a set/get)
		sweepData, err := setup.Build1DSweepData(
			"P1", 0.0, 0.0, 1,
			map[string]float64{},
			0.0,
		)
		require.NoError(t, err)

		scriptPath, err := gen.GenerateSweep1DScript(*sweepData)
		require.NoError(t, err)

		content, err := os.ReadFile(scriptPath)
		require.NoError(t, err)
		contentStr := string(content)

		assert.Contains(t, contentStr, "num_points = 1")
		// Should not contain sleep call when settling time is 0
		assert.NotContains(t, contentStr, "ctx:sleep")
	})
}

// =============================================================================
// Integration Test: Full Measurement Workflow
// =============================================================================

func TestQuantumDotMeasurementWorkflow(t *testing.T) {
	configPath := filepath.Join("..", "..", "testdata", "one_charge_sensor_quantum_dot_device.yaml")
	config, err := LoadQuantumDotDeviceConfig(configPath)
	require.NoError(t, err)

	setup := NewQuantumDotMeasurementSetup(config, "QDAC1", "DMM1")
	tempDir := t.TempDir()
	gen, err := NewScriptGenerator(tempDir)
	require.NoError(t, err)

	t.Run("complete tuning sequence generates valid scripts", func(t *testing.T) {
		// Step 1: Initialize all gates
		initData := DCGetSetScriptData{
			MeasurementName: "init_all_gates",
			SetVoltageRequests: setup.BuildSetVoltageRequests(map[string]float64{
				"P1": 0.0, "P2": 0.0, "P3": 0.0,
				"B1": -0.5, "B2": -0.5, "B3": -0.5, "B4": -0.5, "B5": -0.5,
				"S1": -0.3, "S2": -0.3, "S3": -0.3,
				"R1": 0.0, "R2": 0.0, "R3": 0.0, "R4": 0.0,
			}),
			GetVoltageRequests: setup.BuildGetVoltageRequests(),
			SettlingTimeMs:     50.0,
		}

		initPath, err := gen.GenerateDCGetSetScript(initData)
		require.NoError(t, err)
		assert.FileExists(t, initPath)

		// Step 2: Barrier pinch-off sweeps
		for _, barrier := range []string{"B1", "B2", "B3"} {
			sweepData, err := setup.Build1DSweepData(
				barrier, 0.0, -2.0, 101,
				map[string]float64{"P1": 0.0, "P2": 0.0},
				5.0,
			)
			require.NoError(t, err)
			sweepData.MeasurementName = barrier + "_pinchoff"

			sweepPath, err := gen.GenerateSweep1DScript(*sweepData)
			require.NoError(t, err)
			assert.FileExists(t, sweepPath)
		}

		// Step 3: Plunger sweep for Coulomb diamonds
		cpSweep, err := setup.Build1DSweepData(
			"P1", -0.8, 0.2, 201,
			map[string]float64{
				"P2": -0.5, "B1": -1.2, "B2": -1.0, "B3": -1.2,
			},
			2.0,
		)
		require.NoError(t, err)
		cpSweep.MeasurementName = "P1_coulomb_peaks"

		cpPath, err := gen.GenerateSweep1DScript(*cpSweep)
		require.NoError(t, err)
		assert.FileExists(t, cpPath)

		// Log all generated scripts
		files, _ := os.ReadDir(tempDir)
		t.Logf("Generated %d scripts:", len(files))
		for _, f := range files {
			t.Logf("  - %s", f.Name())
		}
	})
}
