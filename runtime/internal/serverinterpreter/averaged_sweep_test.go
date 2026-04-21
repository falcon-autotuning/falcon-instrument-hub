// Package serverinterpreter provides tests for averaged sweep functionality.
package serverinterpreter

import (
	"encoding/json"
	filepathpkg "path/filepath"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Hub Config Tests
// =============================================================================

func TestHubConfig_Parse(t *testing.T) {
	configYAML := `
wiremap: C://configs/wiremap.yaml
quantum-dot-config: C://configs/qdot.yaml
inst-config: C://configs/instruments
teal-apis: C://apis/teal
lua-library-types: C://lua/types
user-measurement-luas: C://lua/user
local-database: C://data
nats-url: nats://localhost:4222
instrument-server-port: 5555
`

	t.Run("parse hub config from YAML", func(t *testing.T) {
		config, err := ParseHubConfig([]byte(configYAML))
		require.NoError(t, err)

		assert.Equal(t, "C://configs/wiremap.yaml", config.Wiremap)
		assert.Equal(t, "C://configs/qdot.yaml", config.QuantumDotConfig)
		assert.Equal(t, "C://data", config.LocalDatabase)
		assert.Equal(t, "nats://localhost:4222", config.NATSUrl)
		assert.Equal(t, 5555, config.InstrumentServerPort)
	})

	t.Run("default values", func(t *testing.T) {
		config, err := ParseHubConfig([]byte("local-database: /tmp/data"))
		require.NoError(t, err)

		assert.Equal(t, "nats://localhost:4222", config.GetNATSUrl())
		assert.Equal(t, 5555, config.GetInstrumentServerPort())
	})
}

func TestHubConfig_LoadFromFile(t *testing.T) {
	// Use the actual config file
	configPath := filepath.Join("..", "..", "..", "instrument_hub_config.yaml")

	t.Run("load existing config", func(t *testing.T) {
		config, err := LoadHubConfig(configPath)
		require.NoError(t, err)

		assert.NotEmpty(t, config.LocalDatabase)
		assert.Equal(t, 5555, config.InstrumentServerPort)
	})
}

// =============================================================================
// Trace Buffer Tests
// =============================================================================

func TestTraceBuffer_RegisterAndAdd(t *testing.T) {
	config := DefaultTraceBufferConfig()
	config.OnLog = func(msg string) { t.Log(msg) }
	buffer := NewTraceBuffer(config)

	t.Run("register measurement", func(t *testing.T) {
		err := buffer.RegisterMeasurement(
			"test-123",
			"P1",
			-1.0, 0.0,
			10,
			5, // 5 sweeps
			[]string{"DMM1_0"},
		)
		require.NoError(t, err)

		received, expected, exists := buffer.GetStatus("test-123")
		assert.True(t, exists)
		assert.Equal(t, 0, received)
		assert.Equal(t, 5, expected)
	})

	t.Run("add traces", func(t *testing.T) {
		// Simulate receiving traces
		for i := 1; i <= 5; i++ {
			trace := make([]map[string]interface{}, 10)
			for j := 0; j < 10; j++ {
				voltage := -1.0 + float64(j)*0.1
				trace[j] = map[string]interface{}{
					"voltage": voltage,
					"measurements": map[string]interface{}{
						"DMM1_0": float64(i) * 1e-9, // Different value per sweep
					},
				}
			}

			report := &TraceReportMessage{
				MeasurementID: "test-123",
				SweepIndex:    i,
				TotalSweeps:   5,
				Trace:         trace,
			}

			complete, err := buffer.AddTrace(report)
			require.NoError(t, err)

			if i < 5 {
				assert.False(t, complete)
			} else {
				assert.True(t, complete)
			}
		}

		received, expected, _ := buffer.GetStatus("test-123")
		assert.Equal(t, 5, received)
		assert.Equal(t, 5, expected)
	})

	t.Run("complete and average", func(t *testing.T) {
		result, err := buffer.Complete("test-123")
		require.NoError(t, err)

		assert.Equal(t, "test-123", result.MeasurementID)
		assert.Equal(t, "P1", result.SweepGate)
		assert.Equal(t, 5, result.NumSweeps)
		assert.Len(t, result.AllTraces, 5)
		assert.Len(t, result.AveragedTrace.Points, 10)

		// Verify averaging: (1+2+3+4+5)/5 = 3
		avgCurrent := result.AveragedTrace.Points[0].Measurements["DMM1_0"]
		expectedAvg := 3e-9
		assert.InDelta(t, expectedAvg, avgCurrent, 1e-15)
	})
}

func TestTraceBuffer_ExtractCurrentTrace(t *testing.T) {
	result := &AveragedMeasurementResult{
		MeasurementID: "test",
		AveragedTrace: AveragedTrace{
			Points: []TracePoint{
				{Voltage: -1.0, Measurements: map[string]float64{"DMM1_0": 1e-9}},
				{Voltage: -0.5, Measurements: map[string]float64{"DMM1_0": 2e-9}},
				{Voltage: 0.0, Measurements: map[string]float64{"DMM1_0": 3e-9}},
			},
		},
	}

	voltages, currents, err := result.ExtractCurrentTrace("DMM1_0")
	require.NoError(t, err)

	assert.Equal(t, []float64{-1.0, -0.5, 0.0}, voltages)
	assert.Equal(t, []float64{1e-9, 2e-9, 3e-9}, currents)
}

// =============================================================================
// HDF5/Database Writer Tests
// =============================================================================

func TestMeasurementDatabase_StoreAndLoad(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("store and retrieve measurement", func(t *testing.T) {
		db, err := NewMeasurementDatabase(tempDir)
		require.NoError(t, err)

		result := &AveragedMeasurementResult{
			MeasurementID: "test-measurement-1",
			SweepGate:     "P1",
			StartVoltage:  -1.0,
			StopVoltage:   0.0,
			NumPoints:     10,
			NumSweeps:     5,
			AveragedTrace: AveragedTrace{
				Points: []TracePoint{
					{Voltage: -1.0, Measurements: map[string]float64{"DMM1_0": 1e-9}},
				},
			},
		}

		// Store
		filepath, err := db.Store(result)
		require.NoError(t, err)
		assert.NotEmpty(t, filepath)
		assert.FileExists(t, filepath)
		assert.Equal(t, ".h5", filepathpkg.Ext(filepath))

		// Load
		loaded, err := db.Load("test-measurement-1")
		require.NoError(t, err)

		assert.Equal(t, result.MeasurementID, loaded.MeasurementID)
		assert.Equal(t, result.SweepGate, loaded.SweepGate)
		assert.Equal(t, result.NumPoints, loaded.NumPoints)
		assert.Empty(t, loaded.AllTraces)
	})

	t.Run("list measurements", func(t *testing.T) {
		db, err := NewMeasurementDatabase(tempDir)
		require.NoError(t, err)

		list := db.List()
		assert.Len(t, list, 1)
		assert.Equal(t, "test-measurement-1", list[0].MeasurementID)
	})
}

// =============================================================================
// Averaged Sweep Request Parsing Tests
// =============================================================================

func TestParseAveragedSweepRequest(t *testing.T) {
	t.Run("parse valid request", func(t *testing.T) {
		reqJSON := `{
			"measurement_name": "P1_averaged_sweep",
			"sweep_gate": "P1",
			"start_voltage": -1.0,
			"stop_voltage": 0.0,
			"num_points": 101,
			"num_averages": 10,
			"settling_time_ms": 5.0,
			"static_voltages": {
				"P2": -0.5,
				"B1": -0.8
			}
		}`

		req, err := ParseAveragedSweepRequestJSON(reqJSON)
		require.NoError(t, err)

		assert.Equal(t, "P1_averaged_sweep", req.MeasurementName)
		assert.Equal(t, "P1", req.SweepGate)
		assert.Equal(t, -1.0, req.StartVoltage)
		assert.Equal(t, 0.0, req.StopVoltage)
		assert.Equal(t, 101, req.NumPoints)
		assert.Equal(t, 10, req.NumAverages)
		assert.Equal(t, 5.0, req.SettlingTimeMs)
		assert.Equal(t, -0.5, req.StaticVoltages["P2"])
	})

	t.Run("default num_averages", func(t *testing.T) {
		reqJSON := `{
			"sweep_gate": "P1",
			"num_points": 10
		}`

		req, err := ParseAveragedSweepRequestJSON(reqJSON)
		require.NoError(t, err)
		assert.Equal(t, 1, req.NumAverages)
	})

	t.Run("validation errors", func(t *testing.T) {
		_, err := ParseAveragedSweepRequestJSON(`{"num_points": 10}`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sweep_gate")

		_, err = ParseAveragedSweepRequestJSON(`{"sweep_gate": "P1", "num_points": 0}`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "num_points")
	})
}

// =============================================================================
// End-to-End Workflow Test
// =============================================================================

func TestAveragedSweep_EndToEndWorkflow(t *testing.T) {
	// This test simulates the complete workflow without actual NATS/instrument connections

	tempDir := t.TempDir()

	// Load device config relative to this test file (runtime/internal/serverinterpreter/)
	configPath := filepath.Join("..", "..", "..", "test_data", "dummy_one_charge_sensor_quantum_dot_device.yaml")
	deviceConfig, err := LoadQuantumDotDeviceConfig(configPath)
	require.NoError(t, err)

	setup := NewQuantumDotMeasurementSetup(deviceConfig, "QDAC1", "DMM1")

	// Create components
	bufferConfig := DefaultTraceBufferConfig()
	bufferConfig.OnLog = func(msg string) { t.Log(msg) }
	buffer := NewTraceBuffer(bufferConfig)

	db, err := NewMeasurementDatabase(tempDir)
	require.NoError(t, err)

	t.Run("complete workflow simulation", func(t *testing.T) {
		// 1. Parse request
		reqJSON := `{
			"measurement_name": "P1_coulomb_peaks",
			"sweep_gate": "P1",
			"start_voltage": -0.8,
			"stop_voltage": 0.2,
			"num_points": 100,
			"num_averages": 5,
			"settling_time_ms": 2.0,
			"static_voltages": {
				"P2": -0.5,
				"B1": -1.2,
				"B2": -1.0,
				"B3": -1.2
			}
		}`

		req, err := ParseAveragedSweepRequestJSON(reqJSON)
		require.NoError(t, err)

		measurementID := "workflow-test-123"

		// 2. Build sweep data
		sweepData, err := setup.Build1DSweepData(
			req.SweepGate,
			req.StartVoltage,
			req.StopVoltage,
			req.NumPoints,
			req.StaticVoltages,
			req.SettlingTimeMs,
		)
		require.NoError(t, err)

		// Verify sweep data was built correctly
		assert.Equal(t, req.SweepGate, sweepData.SweepGate)
		assert.Equal(t, req.StartVoltage, sweepData.StartVoltage)

		// 3. Register measurement
		channels := []string{"DMM1_0", "DMM1_1"}
		err = buffer.RegisterMeasurement(
			measurementID,
			req.SweepGate,
			req.StartVoltage,
			req.StopVoltage,
			req.NumPoints,
			req.NumAverages,
			channels,
		)
		require.NoError(t, err)

		// 5. Simulate receiving traces from instrument-script-server
		step := (req.StopVoltage - req.StartVoltage) / float64(req.NumPoints-1)
		for sweepIdx := 1; sweepIdx <= req.NumAverages; sweepIdx++ {
			trace := make([]map[string]interface{}, req.NumPoints)
			for i := 0; i < req.NumPoints; i++ {
				voltage := req.StartVoltage + float64(i)*step
				// Simulate current with some sweep-dependent noise
				baseCurrent := 1e-9 * (1.0 + voltage*voltage)
				noise := float64(sweepIdx) * 1e-11

				trace[i] = map[string]interface{}{
					"voltage": voltage,
					"measurements": map[string]interface{}{
						"DMM1_0": baseCurrent + noise,
						"DMM1_1": baseCurrent*0.1 + noise*0.1,
					},
				}
			}

			report := &TraceReportMessage{
				MeasurementID: measurementID,
				SweepIndex:    sweepIdx,
				TotalSweeps:   req.NumAverages,
				Trace:         trace,
			}

			complete, err := buffer.AddTrace(report)
			require.NoError(t, err)

			if sweepIdx == req.NumAverages {
				assert.True(t, complete)
			}
		}

		// 6. Complete and average
		result, err := buffer.Complete(measurementID)
		require.NoError(t, err)

		assert.Equal(t, measurementID, result.MeasurementID)
		assert.Equal(t, req.NumPoints, result.NumPoints)
		assert.Equal(t, req.NumAverages, result.NumSweeps)
		assert.Len(t, result.AveragedTrace.Points, req.NumPoints)

		// Verify averaging happened
		for _, pt := range result.AveragedTrace.Points {
			assert.Contains(t, pt.Measurements, "DMM1_0")
			assert.Contains(t, pt.Measurements, "DMM1_1")
		}

		// 7. Store to database
		dbPath, err := db.Store(result)
		require.NoError(t, err)
		assert.FileExists(t, dbPath)

		// 8. Verify we can reload
		loaded, err := db.Load(measurementID)
		require.NoError(t, err)
		assert.Equal(t, result.NumSweeps, loaded.NumSweeps)

		// 9. Extract current trace for plotting
		voltages, currents, err := result.ExtractCurrentTrace("DMM1_0")
		require.NoError(t, err)
		assert.Len(t, voltages, req.NumPoints)
		assert.Len(t, currents, req.NumPoints)

		t.Logf("Workflow complete: %d points, %d averages, stored at %s",
			result.NumPoints, result.NumSweeps, dbPath)
	})
}

// =============================================================================
// Performance/Stress Tests
// =============================================================================

func TestTraceBuffer_LargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	config := DefaultTraceBufferConfig()
	buffer := NewTraceBuffer(config)

	numPoints := 1000
	numAverages := 100

	err := buffer.RegisterMeasurement(
		"large-test",
		"P1",
		-1.0, 0.0,
		numPoints,
		numAverages,
		[]string{"DMM1_0"},
	)
	require.NoError(t, err)

	start := time.Now()

	for sweepIdx := 1; sweepIdx <= numAverages; sweepIdx++ {
		trace := make([]map[string]interface{}, numPoints)
		for i := 0; i < numPoints; i++ {
			trace[i] = map[string]interface{}{
				"voltage": float64(i) / float64(numPoints),
				"measurements": map[string]interface{}{
					"DMM1_0": float64(sweepIdx) * 1e-9,
				},
			}
		}

		report := &TraceReportMessage{
			MeasurementID: "large-test",
			SweepIndex:    sweepIdx,
			TotalSweeps:   numAverages,
			Trace:         trace,
		}

		_, err := buffer.AddTrace(report)
		require.NoError(t, err)
	}

	result, err := buffer.Complete("large-test")
	require.NoError(t, err)

	elapsed := time.Since(start)
	t.Logf("Processed %d sweeps × %d points = %d total points in %v",
		numAverages, numPoints, numAverages*numPoints, elapsed)

	assert.Equal(t, numAverages, result.NumSweeps)
	assert.Len(t, result.AveragedTrace.Points, numPoints)
}

// =============================================================================
// JSON Serialization Tests
// =============================================================================

func TestAveragedMeasurementResult_JSON(t *testing.T) {
	result := &AveragedMeasurementResult{
		MeasurementID: "json-test",
		SweepGate:     "P1",
		StartVoltage:  -1.0,
		StopVoltage:   0.0,
		NumPoints:     3,
		NumSweeps:     2,
		AveragedTrace: AveragedTrace{
			Points: []TracePoint{
				{Voltage: -1.0, Measurements: map[string]float64{"DMM1_0": 1e-9}},
				{Voltage: -0.5, Measurements: map[string]float64{"DMM1_0": 2e-9}},
				{Voltage: 0.0, Measurements: map[string]float64{"DMM1_0": 3e-9}},
			},
		},
	}

	jsonStr, err := result.ToJSON()
	require.NoError(t, err)

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &parsed)
	require.NoError(t, err)

	assert.Equal(t, "json-test", parsed["measurement_id"])
	assert.Equal(t, "P1", parsed["sweep_gate"])
}
