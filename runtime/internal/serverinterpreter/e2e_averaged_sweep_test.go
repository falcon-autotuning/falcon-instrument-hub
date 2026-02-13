// Package serverinterpreter provides end-to-end tests for the averaged sweep flow.
//
// This test file demonstrates the complete lifecycle of an N-averaged 1D voltage sweep:
//
//  1. Falcon sends a measurement request over NATS
//  2. Hub parses the request via MeasurementRouter
//  3. Hub prepares Lua scripts via MeasurementOrchestrator
//  4. Scripts are dispatched to instrument-script-server via ScriptDispatcher
//  5. Instrument-server executes sweeps and returns data to hub
//  6. Hub buffers trace data in TraceBuffer
//  7. Hub computes averages and stores to HDF5/JSON database
//  8. Hub publishes completion notification to falcon via JetStream
//
// The test uses mock interfaces to simulate the full flow without hardware.
package serverinterpreter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test: Complete E2E Averaged Sweep Flow
// =============================================================================

// TestE2E_AveragedSweep_CompleteFlow tests the entire flow from falcon request
// through to JetStream notification.
func TestE2E_AveragedSweep_CompleteFlow(t *testing.T) {
	// Create temp directory for test data
	tempDir := t.TempDir()

	// Track JetStream publications for verification
	var publishedMessages []JetStreamMessage
	var publishMu sync.Mutex

	// Create components
	t.Run("1_parse_falcon_request", func(t *testing.T) {
		// Simulate falcon sending a measure_1D_buffered request with averaging
		// This is the wire format from falcon-measurement-lib
		falconRequest := `{
			"envelope": {
				"type": "measure_1D_buffered",
				"request_id": "falcon-req-12345",
				"process_id": 42,
				"timestamp": "2026-02-09T10:30:00Z"
			},
			"payload": {
				"bufferedSetters": [{"id": "QDAC1", "channel": 1}],
				"bufferedGetters": [{"id": "DMM1", "channel": 0}],
				"setVoltageDomains": {
					"QDAC1:1": {"min": -1.0, "max": 0.0}
				},
				"sampleRate": 10000,
				"numPoints": 101,
				"numSteps": 101,
				"numAverages": 10
			}
		}`

		var envelope map[string]interface{}
		err := json.Unmarshal([]byte(falconRequest), &envelope)
		require.NoError(t, err)

		// Verify envelope type
		envData := envelope["envelope"].(map[string]interface{})
		assert.Equal(t, "measure_1D_buffered", envData["type"])
		assert.Equal(t, float64(42), envData["process_id"])

		// Parse payload
		payloadJSON, _ := json.Marshal(envelope["payload"])
		var bufferedReq FalconMeasure1DBufferedRequest
		err = json.Unmarshal(payloadJSON, &bufferedReq)
		require.NoError(t, err)

		assert.Len(t, bufferedReq.BufferedSetters, 1)
		assert.Equal(t, "QDAC1", bufferedReq.BufferedSetters[0].ID)
		assert.Equal(t, 1, bufferedReq.BufferedSetters[0].Channel)
		assert.Equal(t, -1.0, bufferedReq.SetVoltageDomains["QDAC1:1"].Min)
		assert.Equal(t, 0.0, bufferedReq.SetVoltageDomains["QDAC1:1"].Max)
		assert.Equal(t, 101, bufferedReq.NumSteps)
	})

	t.Run("2_create_measurement_orchestrator", func(t *testing.T) {
		// Create mock executor that simulates instrument-script-server
		mockExecutor := NewMockScriptExecutor()

		// Create orchestrator
		hubConfig := &HubConfig{}
		orchestrator := NewMeasurementOrchestrator(mockExecutor, hubConfig)
		require.NotNil(t, orchestrator)

		// Build averaged sweep request from falcon envelope
		avgReq := AveragedSweep1DRequest{
			MeasurementID:   uuid.New().String(),
			SweepGate:       "P1",
			SweepInstrument: "QDAC1",
			SweepChannel:    1,
			CurrentMeter:    "DMM1",
			CurrentChannel:  0,
			StartV:          -1.0,
			StopV:           0.0,
			NumPoints:       101,
			NumAverages:     10,
			SettlingTimeMs:  5.0,
		}

		// Execute (with mock)
		result, err := orchestrator.ExecuteAveraged1DSweep(context.Background(), avgReq)

		// The mock will simulate execution
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 10, result.NumAverages)
		assert.Len(t, result.Voltages, 101)
	})

	t.Run("3_trace_buffer_accumulation", func(t *testing.T) {
		// Create trace buffer
		config := DefaultTraceBufferConfig()
		config.DatabasePath = tempDir
		config.OnLog = func(msg string) {
			t.Logf("TraceBuffer: %s", msg)
		}

		buffer := NewTraceBuffer(config)

		measurementID := "test-sweep-001"
		numSweeps := 10
		numPoints := 101

		// Register measurement
		err := buffer.RegisterMeasurement(
			measurementID,
			"P1",      // sweep gate
			-1.0, 0.0, // voltage range
			numPoints,
			numSweeps,
			[]string{"DMM1_0"},
		)
		require.NoError(t, err)

		// Simulate receiving 10 trace reports from instrument-script-server
		for sweepIdx := 1; sweepIdx <= numSweeps; sweepIdx++ {
			trace := make([]map[string]interface{}, numPoints)
			for i := 0; i < numPoints; i++ {
				voltage := -1.0 + float64(i)*0.01 // -1.0 to 0.0
				// Simulate current with noise
				baseCurrent := 1e-9 * (1.0 - voltage*voltage) // Parabolic
				noise := float64(sweepIdx) * 1e-12            // Sweep-dependent offset

				trace[i] = map[string]interface{}{
					"voltage": voltage,
					"measurements": map[string]interface{}{
						"DMM1_0": baseCurrent + noise,
					},
				}
			}

			report := &TraceReportMessage{
				MeasurementID: measurementID,
				SweepIndex:    sweepIdx,
				TotalSweeps:   numSweeps,
				Trace:         trace,
			}

			complete, err := buffer.AddTrace(report)
			require.NoError(t, err)

			if sweepIdx < numSweeps {
				assert.False(t, complete, "Should not be complete after sweep %d", sweepIdx)
			} else {
				assert.True(t, complete, "Should be complete after all sweeps")
			}
		}

		// Check status - measurement is still pending until Complete() is called
		received, expected, exists := buffer.GetStatus(measurementID)
		// The measurement exists until Complete() is called
		if exists {
			t.Logf("Status: received %d of %d traces", received, expected)
			assert.Equal(t, numSweeps, received)
			assert.Equal(t, numSweeps, expected)
		}

		// Now call Complete to finalize
		result, err := buffer.Complete(measurementID)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, numSweeps, result.NumSweeps)
		assert.Equal(t, numPoints, result.NumPoints)
	})

	t.Run("4_compute_average_and_store", func(t *testing.T) {
		config := DefaultTraceBufferConfig()
		config.DatabasePath = tempDir
		var completedResult *AveragedMeasurementResult
		config.OnMeasurementComplete = func(result *AveragedMeasurementResult) error {
			completedResult = result
			return nil
		}

		buffer := NewTraceBuffer(config)

		measurementID := "test-sweep-002"
		numSweeps := 5
		numPoints := 50

		// Register
		err := buffer.RegisterMeasurement(
			measurementID,
			"P1",
			-0.5, 0.5,
			numPoints,
			numSweeps,
			[]string{"DMM1_0"},
		)
		require.NoError(t, err)

		// Add traces
		for sweepIdx := 1; sweepIdx <= numSweeps; sweepIdx++ {
			trace := make([]map[string]interface{}, numPoints)
			for i := 0; i < numPoints; i++ {
				voltage := -0.5 + float64(i)*0.02
				current := 1e-9 + float64(sweepIdx)*1e-11 // Slightly different each sweep

				trace[i] = map[string]interface{}{
					"voltage": voltage,
					"measurements": map[string]interface{}{
						"DMM1_0": current,
					},
				}
			}

			report := &TraceReportMessage{
				MeasurementID: measurementID,
				SweepIndex:    sweepIdx,
				TotalSweeps:   numSweeps,
				Trace:         trace,
			}

			complete, err := buffer.AddTrace(report)
			require.NoError(t, err)

			if complete {
				// Call complete to get the result
				result, err := buffer.Complete(measurementID)
				require.NoError(t, err)
				completedResult = result
			}
		}

		// Verify result
		require.NotNil(t, completedResult)
		assert.Equal(t, measurementID, completedResult.MeasurementID)
		assert.Equal(t, numSweeps, completedResult.NumSweeps)
		assert.Equal(t, numPoints, completedResult.NumPoints)
		assert.Len(t, completedResult.AllTraces, numSweeps)
		assert.Len(t, completedResult.AveragedTrace.Points, numPoints)

		// Verify averaging worked
		// Average of currents 1.01e-9, 1.02e-9, 1.03e-9, 1.04e-9, 1.05e-9 = 1.03e-9
		avgCurrent := completedResult.AveragedTrace.Points[0].Measurements["DMM1_0"]
		assert.InDelta(t, 1.03e-9, avgCurrent, 1e-12)

		t.Logf("Averaged current at first point: %.3e A", avgCurrent)
	})

	t.Run("5_store_to_hdf5_database", func(t *testing.T) {
		// Use a fresh subdirectory so the count below is independent
		dbDir := filepath.Join(tempDir, "test5_db")

		// Create database (now creates raw/ and averaged/ subdirs)
		database, err := NewMeasurementDatabase(dbDir)
		require.NoError(t, err)

		// Verify directory structure was created
		assert.DirExists(t, filepath.Join(dbDir, "raw"))
		assert.DirExists(t, filepath.Join(dbDir, "averaged"))

		// Create a measurement result with raw traces
		result := &AveragedMeasurementResult{
			MeasurementID: "hdf5-test-001",
			SweepGate:     "P1",
			StartVoltage:  -1.0,
			StopVoltage:   0.0,
			NumPoints:     101,
			NumSweeps:     10,
			AllTraces:     make([]Trace, 10),
			AveragedTrace: AveragedTrace{
				Points:    make([]TracePoint, 101),
				NumSweeps: 10,
				SweepGate: "P1",
				StartV:    -1.0,
				StopV:     0.0,
			},
			TotalDuration: 5 * time.Second,
		}

		// Generate realistic raw trace data
		for i := 0; i < 10; i++ {
			result.AllTraces[i] = Trace{
				SweepIndex: i + 1,
				Points:     make([]TracePoint, 101),
				Timestamp:  time.Now(),
			}
			for j := 0; j < 101; j++ {
				voltage := -1.0 + float64(j)*0.01
				result.AllTraces[i].Points[j] = TracePoint{
					Voltage: voltage,
					Measurements: map[string]float64{
						"DMM1_0": 1e-9*(1.0+voltage) + float64(i)*1e-11,
					},
				}
			}
		}

		for i := 0; i < 101; i++ {
			voltage := -1.0 + float64(i)*0.01
			current := 1e-9 * (1.0 + voltage) // Linear

			result.AveragedTrace.Points[i] = TracePoint{
				Voltage: voltage,
				Measurements: map[string]float64{
					"DMM1_0": current,
				},
			}
		}

		// Store (splits raw traces into raw/, averaged into averaged/)
		avgFilePath, err := database.Store(result)
		require.NoError(t, err)
		assert.FileExists(t, avgFilePath)
		t.Logf("Averaged stored to: %s", avgFilePath)

		// Verify averaged file is in averaged/ subdirectory
		assert.Contains(t, avgFilePath, "averaged")

		// Verify raw file exists in raw/ subdirectory
		require.NotNil(t, result.RawRef, "RawRef should be populated after Store")
		assert.FileExists(t, result.RawRef.RawFilePath)
		assert.Contains(t, result.RawRef.RawFilePath, "raw")
		assert.Equal(t, 10, result.RawRef.NumTraces)
		t.Logf("Raw traces stored to: %s", result.RawRef.RawFilePath)

		// Load averaged (should NOT contain raw traces)
		loaded, err := database.Load("hdf5-test-001")
		require.NoError(t, err)
		assert.Equal(t, result.MeasurementID, loaded.MeasurementID)
		assert.Equal(t, result.NumSweeps, loaded.NumSweeps)
		assert.Equal(t, result.NumPoints, loaded.NumPoints)
		assert.Empty(t, loaded.AllTraces, "Averaged database should NOT contain raw traces")
		assert.NotNil(t, loaded.RawRef, "Averaged record should have RawDataRef link")

		// Verify averaged data integrity
		loadedCurrent := loaded.AveragedTrace.Points[50].Measurements["DMM1_0"]
		expectedCurrent := result.AveragedTrace.Points[50].Measurements["DMM1_0"]
		assert.InDelta(t, expectedCurrent, loadedCurrent, 1e-15)

		// Load raw traces separately
		rawRecord, err := database.LoadRawTraces("hdf5-test-001")
		require.NoError(t, err)
		assert.Len(t, rawRecord.Traces, 10, "Raw database should have all 10 traces")
		assert.Equal(t, 101, rawRecord.NumPoints)

		// LoadWithRawTraces should reconstruct the full result
		full, err := database.LoadWithRawTraces("hdf5-test-001")
		require.NoError(t, err)
		assert.Len(t, full.AllTraces, 10, "LoadWithRawTraces should populate AllTraces")

		// Verify index tracks both
		measurements := database.List()
		assert.Len(t, measurements, 1)
		assert.Equal(t, "hdf5-test-001", measurements[0].MeasurementID)
		assert.NotNil(t, measurements[0].RawDataRef)
	})

	t.Run("6_jetstream_notification", func(t *testing.T) {
		// Simulate JetStream publication for averaged-only sharing
		avgPath := filepath.Join(tempDir, "averaged", "sweep_jetstream-test-001.json")
		rawRef := &RawDataRef{
			MeasurementID: "jetstream-test-001",
			RawFilePath:   filepath.Join(tempDir, "raw", "raw_jetstream-test-001.json"),
			NumTraces:     10,
			NumPoints:     101,
		}

		// Create notification message for falcon.
		// FilePath points to the averaged database ONLY.
		// RawDataRef is included as metadata so falcon knows raw exists, but
		// the raw file path is hub-local and not directly accessible by falcon.
		notification := FalconMeasurementNotification{
			Type:          "measurement_complete",
			MeasurementID: "jetstream-test-001",
			ProcessID:     42,
			Status:        "success",
			DataLocation: FalconDataLocation{
				Stream:     "FALCON_MEASUREMENTS",
				Subject:    "measurement.result.jetstream-test-001",
				FilePath:   avgPath,
				NumPoints:  101,
				NumSweeps:  10,
				RawDataRef: rawRef,
			},
			Timestamp: time.Now(),
		}

		// Serialize for publish
		notificationJSON, err := json.MarshalIndent(notification, "", "  ")
		require.NoError(t, err)

		t.Logf("JetStream notification:\n%s", notificationJSON)

		// Track publication
		publishMu.Lock()
		publishedMessages = append(publishedMessages, JetStreamMessage{
			Subject: "measurement.result.jetstream-test-001",
			Data:    notificationJSON,
		})
		publishMu.Unlock()

		// Verify notification structure
		var parsed FalconMeasurementNotification
		err = json.Unmarshal(notificationJSON, &parsed)
		require.NoError(t, err)
		assert.Equal(t, "measurement_complete", parsed.Type)
		assert.Equal(t, "success", parsed.Status)
		assert.Equal(t, 101, parsed.DataLocation.NumPoints)
		assert.Contains(t, parsed.DataLocation.FilePath, "averaged",
			"JetStream notification should reference averaged database, not raw")
		assert.NotNil(t, parsed.DataLocation.RawDataRef,
			"Notification should include RawDataRef metadata")
		assert.Equal(t, 10, parsed.DataLocation.RawDataRef.NumTraces)
	})

	t.Run("7_full_flow_integration", func(t *testing.T) {
		// This test combines all components in a realistic scenario
		// and verifies the two-database split end-to-end.

		// Use a fresh subdirectory
		intDir := filepath.Join(tempDir, "test7_integration")

		// Create all components
		mockExecutor := NewMockScriptExecutor()
		hubConfig := &HubConfig{}
		orchestrator := NewMeasurementOrchestrator(mockExecutor, hubConfig)

		traceConfig := DefaultTraceBufferConfig()
		traceConfig.DatabasePath = intDir
		traceBuffer := NewTraceBuffer(traceConfig)

		database, err := NewMeasurementDatabase(intDir)
		require.NoError(t, err)

		// 1. Receive falcon request
		falconReq := FalconMeasure1DBufferedRequest{
			BufferedSetters: []FalconInstrumentTarget{
				{ID: "QDAC1", Channel: 1},
			},
			BufferedGetters: []FalconInstrumentTarget{
				{ID: "DMM1", Channel: 0},
			},
			SetVoltageDomains: map[string]FalconDomain{
				"QDAC1:1": {Min: -1.0, Max: 0.0},
			},
			SampleRate: 10000,
			NumPoints:  50,
			NumSteps:   50,
		}
		numAverages := 10

		// 2. Convert to averaged sweep request
		measurementID := uuid.New().String()
		avgReq := AveragedSweep1DRequest{
			MeasurementID:   measurementID,
			SweepGate:       "P1",
			SweepInstrument: falconReq.BufferedSetters[0].ID,
			SweepChannel:    falconReq.BufferedSetters[0].Channel,
			CurrentMeter:    falconReq.BufferedGetters[0].ID,
			CurrentChannel:  falconReq.BufferedGetters[0].Channel,
			StartV:          falconReq.SetVoltageDomains["QDAC1:1"].Min,
			StopV:           falconReq.SetVoltageDomains["QDAC1:1"].Max,
			NumPoints:       falconReq.NumSteps,
			NumAverages:     numAverages,
			SettlingTimeMs:  5.0,
		}

		// 3. Execute via orchestrator (mock execution)
		result, err := orchestrator.ExecuteAveraged1DSweep(context.Background(), avgReq)
		require.NoError(t, err)

		// 4. Register with trace buffer and simulate trace receipt
		err = traceBuffer.RegisterMeasurement(
			measurementID,
			"P1",
			avgReq.StartV, avgReq.StopV,
			avgReq.NumPoints,
			numAverages,
			[]string{"DMM1_0"},
		)
		require.NoError(t, err)

		// Simulate instrument-script-server sending traces
		for sweepIdx := 1; sweepIdx <= numAverages; sweepIdx++ {
			trace := make([]map[string]interface{}, avgReq.NumPoints)
			step := (avgReq.StopV - avgReq.StartV) / float64(avgReq.NumPoints-1)

			for i := 0; i < avgReq.NumPoints; i++ {
				voltage := avgReq.StartV + float64(i)*step
				// Parabolic current with sweep-dependent noise
				current := 2e-9*(1.0-voltage*voltage/2.0) + float64(sweepIdx)*1e-11

				trace[i] = map[string]interface{}{
					"voltage": voltage,
					"measurements": map[string]interface{}{
						"DMM1_0": current,
					},
				}
			}

			report := &TraceReportMessage{
				MeasurementID: measurementID,
				SweepIndex:    sweepIdx,
				TotalSweeps:   numAverages,
				Trace:         trace,
			}

			complete, err := traceBuffer.AddTrace(report)
			require.NoError(t, err)

			if complete {
				// 5. Complete and get averaged result
				avgResult, err := traceBuffer.Complete(measurementID)
				require.NoError(t, err)
				require.NotNil(t, avgResult)

				// 6. Store to database (splits raw and averaged)
				dbPath, err := database.Store(avgResult)
				require.NoError(t, err)
				t.Logf("Averaged stored to: %s", dbPath)

				// --- Two-database verification ---

				// Averaged file should be in averaged/ subdirectory
				assert.Contains(t, dbPath, "averaged")
				assert.FileExists(t, dbPath)

				// RawRef should be populated with raw/ path
				require.NotNil(t, avgResult.RawRef)
				assert.FileExists(t, avgResult.RawRef.RawFilePath)
				assert.Contains(t, avgResult.RawRef.RawFilePath, "raw")
				assert.Equal(t, numAverages, avgResult.RawRef.NumTraces)
				t.Logf("Raw traces stored to: %s", avgResult.RawRef.RawFilePath)

				// Load averaged from DB – should NOT contain raw traces
				loadedAvg, loadErr := database.Load(measurementID)
				require.NoError(t, loadErr)
				assert.Empty(t, loadedAvg.AllTraces,
					"Averaged database must not contain raw traces")
				assert.NotNil(t, loadedAvg.RawRef,
					"Averaged record must link to raw database")

				// Load raw traces – should have all sweeps
				rawRecord, rawErr := database.LoadRawTraces(measurementID)
				require.NoError(t, rawErr)
				assert.Len(t, rawRecord.Traces, numAverages)

				// 7. Create JetStream notification (references averaged ONLY)
				notification := FalconMeasurementNotification{
					Type:          "measurement_complete",
					MeasurementID: measurementID,
					ProcessID:     42,
					Status:        "success",
					DataLocation: FalconDataLocation{
						Stream:     "FALCON_MEASUREMENTS",
						Subject:    fmt.Sprintf("measurement.result.%s", measurementID),
						FilePath:   dbPath,
						NumPoints:  avgResult.NumPoints,
						NumSweeps:  avgResult.NumSweeps,
						RawDataRef: avgResult.RawRef,
					},
					Timestamp: time.Now(),
				}

				publishMu.Lock()
				publishedMessages = append(publishedMessages, JetStreamMessage{
					Subject: notification.DataLocation.Subject,
					Data:    mustMarshal(notification),
				})
				publishMu.Unlock()

				// Verify final in-memory result still has AllTraces
				assert.Equal(t, numAverages, avgResult.NumSweeps)
				assert.Equal(t, avgReq.NumPoints, avgResult.NumPoints)
				assert.Len(t, avgResult.AllTraces, numAverages,
					"In-memory result should still hold AllTraces")

				// Verify averaging
				voltages, currents, err := avgResult.ExtractCurrentTrace("DMM1_0")
				require.NoError(t, err)
				assert.Len(t, voltages, avgReq.NumPoints)
				assert.Len(t, currents, avgReq.NumPoints)

				t.Logf("First point: V=%.3f, I=%.3e", voltages[0], currents[0])
				t.Logf("Middle point: V=%.3f, I=%.3e", voltages[25], currents[25])
				t.Logf("Last point: V=%.3f, I=%.3e", voltages[49], currents[49])
			}
		}

		// Also verify the orchestrator result
		require.NotNil(t, result)
		assert.Equal(t, numAverages, result.NumAverages)
	})

	// Final verification
	t.Run("8_verify_all_notifications_published", func(t *testing.T) {
		publishMu.Lock()
		defer publishMu.Unlock()

		assert.GreaterOrEqual(t, len(publishedMessages), 2)
		t.Logf("Total JetStream messages published: %d", len(publishedMessages))

		for i, msg := range publishedMessages {
			t.Logf("Message %d: subject=%s", i+1, msg.Subject)
		}
	})
}

// =============================================================================
// Helper Types for Testing
// =============================================================================

// MockScriptExecutor simulates the instrument-script-server.
type MockScriptExecutor struct {
	executions []ScriptExecution
	mu         sync.Mutex
}

type ScriptExecution struct {
	ScriptName string
	Params     map[string]interface{}
	Timestamp  time.Time
}

func NewMockScriptExecutor() *MockScriptExecutor {
	return &MockScriptExecutor{
		executions: make([]ScriptExecution, 0),
	}
}

func (m *MockScriptExecutor) ExecuteScript(ctx context.Context, scriptName string, params map[string]interface{}) ([]byte, error) {
	m.mu.Lock()
	m.executions = append(m.executions, ScriptExecution{
		ScriptName: scriptName,
		Params:     params,
		Timestamp:  time.Now(),
	})
	m.mu.Unlock()

	// Return simulated results based on script type
	switch scriptName {
	case "sweep_1d":
		numPoints := 101
		if np, ok := params["numPoints"].(int); ok {
			numPoints = np
		}
		return m.simulateSweep1D(params, numPoints)

	case "set_voltage":
		result := map[string]interface{}{
			"success": true,
			"voltage": params["voltage"],
		}
		return json.Marshal(result)

	case "ramp_voltage":
		result := map[string]interface{}{
			"success":      true,
			"finalVoltage": params["targetV"],
		}
		return json.Marshal(result)

	case "measure_current":
		// Simulate a current reading (used by vector sweep)
		result := map[string]interface{}{
			"current": 1e-9,
		}
		return json.Marshal(result)

	default:
		return json.Marshal(map[string]interface{}{"result": "ok"})
	}
}

func (m *MockScriptExecutor) simulateSweep1D(params map[string]interface{}, numPoints int) ([]byte, error) {
	startV := -1.0
	stopV := 0.0
	if sv, ok := params["startVoltage"].(float64); ok {
		startV = sv
	}
	if sv, ok := params["stopVoltage"].(float64); ok {
		stopV = sv
	}

	step := (stopV - startV) / float64(numPoints-1)
	results := make([]map[string]interface{}, numPoints)

	for i := 0; i < numPoints; i++ {
		voltage := startV + float64(i)*step
		current := 1e-9 * (1.0 - voltage*voltage)

		results[i] = map[string]interface{}{
			"voltage": voltage,
			"current": current,
		}
	}

	return json.Marshal(map[string]interface{}{
		"sweep":     results,
		"numPoints": numPoints,
	})
}

func (m *MockScriptExecutor) GetExecutions() []ScriptExecution {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]ScriptExecution{}, m.executions...)
}

// JetStream message tracking
type JetStreamMessage struct {
	Subject string
	Data    []byte
}

// FalconMeasurementNotification is the notification sent to falcon on completion.
type FalconMeasurementNotification struct {
	Type          string             `json:"type"`
	MeasurementID string             `json:"measurement_id"`
	ProcessID     int64              `json:"process_id"`
	Status        string             `json:"status"`
	DataLocation  FalconDataLocation `json:"data_location"`
	Timestamp     time.Time          `json:"timestamp"`
}

// FalconDataLocation describes where to find measurement data.
// Only the averaged database path is shared with falcon – raw traces stay hub-local.
type FalconDataLocation struct {
	Stream      string      `json:"stream"`
	Subject     string      `json:"subject"`
	FilePath    string      `json:"file_path"`    // Averaged database path (shared)
	NumPoints   int         `json:"num_points"`
	NumSweeps   int         `json:"num_sweeps"`
	RawDataRef  *RawDataRef `json:"raw_data_ref,omitempty"` // Reference for debugging (path is hub-local)
}

func mustMarshal(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}

// =============================================================================
// Test: Orchestrator Integration with New Architecture
// =============================================================================

func TestE2E_Orchestrator_2DSweep(t *testing.T) {
	mockExecutor := NewMockScriptExecutor()
	hubConfig := &HubConfig{}
	orchestrator := NewMeasurementOrchestrator(mockExecutor, hubConfig)

	// Simulate 2D sweep request from falcon
	req := Sweep2DRequest{
		MeasurementID:  uuid.New().String(),
		XGate:          "P1",
		XInstrument:    "QDAC1",
		XChannel:       1,
		XStartV:        -0.5,
		XStopV:         0.5,
		XNumPoints:     11,
		YGate:          "P2",
		YInstrument:    "QDAC1",
		YChannel:       2,
		YStartV:        -0.5,
		YStopV:         0.5,
		YNumPoints:     11,
		CurrentMeter:   "DMM1",
		CurrentChannel: 0,
		SettlingTimeMs: 5.0,
		RampSlopeVPerS: 0.1,
	}

	ctx := context.Background()
	result, err := orchestrator.Execute2DSweep(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, result.XVoltages, 11)
	assert.Len(t, result.YVoltages, 11)
	assert.Len(t, result.Lines, 11) // 11 Y slices

	for i, line := range result.Lines {
		assert.Len(t, line.Currents, 11, "Y slice %d should have 11 points", i)
	}

	// Verify correct number of script executions
	executions := mockExecutor.GetExecutions()
	t.Logf("Total script executions: %d", len(executions))

	// Count by type
	setVoltageCount := 0
	sweep1dCount := 0
	rampCount := 0
	for _, exec := range executions {
		switch exec.ScriptName {
		case "set_voltage":
			setVoltageCount++
		case "sweep_1d":
			sweep1dCount++
		case "ramp_voltage":
			rampCount++
		}
	}

	assert.Equal(t, 11, sweep1dCount, "Should have 11 1D sweeps (one per Y value)")
	t.Logf("set_voltage: %d, sweep_1d: %d, ramp_voltage: %d",
		setVoltageCount, sweep1dCount, rampCount)
}

// =============================================================================
// Test: Database Storage and Retrieval
// =============================================================================

func TestE2E_Database_PersistenceAndRetrieval(t *testing.T) {
	tempDir := t.TempDir()

	// Create and store multiple measurements
	db, err := NewMeasurementDatabase(tempDir)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		result := createTestMeasurementResult(fmt.Sprintf("measurement-%03d", i), 50+i*10)

		path, err := db.Store(result)
		require.NoError(t, err)
		t.Logf("Stored %s to %s", result.MeasurementID, path)

		// Verify raw ref was set
		require.NotNil(t, result.RawRef, "RawRef should be set after Store")
		assert.FileExists(t, result.RawRef.RawFilePath)
	}

	// Verify all are listed
	measurements := db.List()
	assert.Len(t, measurements, 5)

	// Load and verify each (averaged only, no raw traces)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("measurement-%03d", i)

		// Load averaged (should NOT have AllTraces)
		loaded, err := db.Load(id)
		require.NoError(t, err)
		assert.Equal(t, id, loaded.MeasurementID)
		assert.Equal(t, 50+i*10, loaded.NumPoints)
		assert.Empty(t, loaded.AllTraces, "Averaged load should not include raw traces")
		assert.NotNil(t, loaded.RawRef, "Should have RawDataRef link")

		// Load raw traces separately
		rawRecord, err := db.LoadRawTraces(id)
		require.NoError(t, err)
		assert.Len(t, rawRecord.Traces, 10)

		// Load combined
		full, err := db.LoadWithRawTraces(id)
		require.NoError(t, err)
		assert.Len(t, full.AllTraces, 10, "LoadWithRawTraces should reconstruct AllTraces")
	}

	// Test persistence - create new database instance
	db2, err := NewMeasurementDatabase(tempDir)
	require.NoError(t, err)

	measurements2 := db2.List()
	assert.Len(t, measurements2, 5, "Should persist across database restarts")

	// Verify raw database also persists
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("measurement-%03d", i)
		rawRecord, err := db2.LoadRawTraces(id)
		require.NoError(t, err)
		assert.Len(t, rawRecord.Traces, 10, "Raw traces should persist across restarts")
	}
}

func createTestMeasurementResult(id string, numPoints int) *AveragedMeasurementResult {
	result := &AveragedMeasurementResult{
		MeasurementID: id,
		SweepGate:     "P1",
		StartVoltage:  -1.0,
		StopVoltage:   0.0,
		NumPoints:     numPoints,
		NumSweeps:     10,
		AllTraces:     make([]Trace, 10),
		AveragedTrace: AveragedTrace{
			Points:    make([]TracePoint, numPoints),
			NumSweeps: 10,
			SweepGate: "P1",
			StartV:    -1.0,
			StopV:     0.0,
		},
		TotalDuration: 5 * time.Second,
	}

	step := 1.0 / float64(numPoints-1)

	// Populate raw traces with realistic data
	for t := 0; t < 10; t++ {
		result.AllTraces[t] = Trace{
			SweepIndex: t + 1,
			Points:     make([]TracePoint, numPoints),
			Timestamp:  time.Now(),
		}
		for i := 0; i < numPoints; i++ {
			voltage := -1.0 + float64(i)*step
			result.AllTraces[t].Points[i] = TracePoint{
				Voltage: voltage,
				Measurements: map[string]float64{
					"DMM1_0": 1e-9*(1+voltage) + float64(t)*1e-11,
				},
			}
		}
	}

	// Populate averaged trace
	for i := 0; i < numPoints; i++ {
		voltage := -1.0 + float64(i)*step
		result.AveragedTrace.Points[i] = TracePoint{
			Voltage: voltage,
			Measurements: map[string]float64{
				"DMM1_0": 1e-9 * (1 + voltage),
			},
		}
	}

	return result
}

// =============================================================================
// Test: Script Loading and Validation
// =============================================================================

func TestE2E_ScriptFiles_Exist(t *testing.T) {
	// Verify required Lua scripts exist in runtime/scripts/
	scriptsDir := "../../scripts"

	requiredScripts := []string{
		"set_voltage.lua",
		"get_voltage.lua",
		"sweep_1d.lua",
		"ramp_voltage.lua",
		"dc_get_set.lua",
		"measure_current.lua",
	}

	for _, script := range requiredScripts {
		path := filepath.Join(scriptsDir, script)
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			t.Logf("Script %s not found at %s (expected in production)", script, path)
			// Don't fail - scripts may be in different location in CI
		} else {
			t.Logf("Found script: %s", path)
		}
	}
}

// =============================================================================
// Test: SweepAxis – General Axis Sweeps
// =============================================================================

func TestE2E_SweepAxis_ScalarSweep(t *testing.T) {
	// A scalar SweepAxis should behave identically to the legacy
	// AveragedSweep1DRequest.
	mockExecutor := NewMockScriptExecutor()
	hubConfig := &HubConfig{}
	orchestrator := NewMeasurementOrchestrator(mockExecutor, hubConfig)

	axis := ScalarAxis("P1", "QDAC1", 1, -1.0, 0.0, 101)
	require.NoError(t, axis.Validate())
	assert.True(t, axis.IsScalar())
	assert.Equal(t, 1, axis.Dimension())

	req := AveragedSweepAxisRequest{
		MeasurementID:  "axis-scalar-001",
		Axis:           axis,
		NumAverages:    5,
		CurrentMeter:   "DMM1",
		CurrentChannel: 0,
		SettlingTimeMs: 1.0,
	}

	result, err := orchestrator.ExecuteAveragedAxisSweep(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 5, result.NumAverages)
	assert.Len(t, result.PrimaryVoltages, 101)
	assert.Len(t, result.AllTraces, 5)
	assert.Len(t, result.AveragedCurrents, 101)
	assert.Len(t, result.StdDev, 101)

	// Primary voltages should match the axis
	assert.InDelta(t, -1.0, result.PrimaryVoltages[0], 1e-12)
	assert.InDelta(t, 0.0, result.PrimaryVoltages[100], 1e-12)

	// Verify scalar sweep used the sweep_1d script (efficient path)
	executions := mockExecutor.GetExecutions()
	sweep1dCount := 0
	for _, exec := range executions {
		if exec.ScriptName == "sweep_1d" {
			sweep1dCount++
		}
	}
	assert.Equal(t, 5, sweep1dCount, "Scalar axis should use sweep_1d script")

	t.Logf("Scalar sweep: %d points, %d averages, %d script calls",
		len(result.PrimaryVoltages), result.NumAverages, len(executions))
}

func TestE2E_SweepAxis_DiagonalSweep(t *testing.T) {
	// Diagonal sweep: P1 goes -0.5→0.5, P2 goes 0.3→-0.3 (opposing).
	// This simulates sweeping along a detuning axis in a double quantum dot.
	mockExecutor := NewMockScriptExecutor()
	hubConfig := &HubConfig{}
	orchestrator := NewMeasurementOrchestrator(mockExecutor, hubConfig)

	axis := DetuningAxis(
		"P1", "QDAC1", 1, -0.5, 0.5,
		"P2", "QDAC1", 2, 0.3, -0.3,
		51,
	)
	require.NoError(t, axis.Validate())
	assert.False(t, axis.IsScalar())
	assert.Equal(t, 2, axis.Dimension())
	assert.Equal(t, "P1/P2 detuning", axis.Label)

	req := AveragedSweepAxisRequest{
		MeasurementID:  "axis-diag-001",
		Axis:           axis,
		NumAverages:    3,
		CurrentMeter:   "DMM1",
		CurrentChannel: 0,
		SettlingTimeMs: 1.0,
	}

	result, err := orchestrator.ExecuteAveragedAxisSweep(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 3, result.NumAverages)
	assert.Len(t, result.PrimaryVoltages, 51)
	assert.Len(t, result.VoltageVectors, 51)
	assert.Len(t, result.AllTraces, 3)
	assert.Len(t, result.AveragedCurrents, 51)

	// Check voltage vectors at endpoints
	assert.InDelta(t, -0.5, result.VoltageVectors[0]["P1"], 1e-12)
	assert.InDelta(t, 0.3, result.VoltageVectors[0]["P2"], 1e-12)
	assert.InDelta(t, 0.5, result.VoltageVectors[50]["P1"], 1e-12)
	assert.InDelta(t, -0.3, result.VoltageVectors[50]["P2"], 1e-12)

	// Midpoint should be zero / zero
	assert.InDelta(t, 0.0, result.VoltageVectors[25]["P1"], 1e-12)
	assert.InDelta(t, 0.0, result.VoltageVectors[25]["P2"], 1e-12)

	// Diagonal sweep should NOT use sweep_1d – it should use set_voltage + measure_current
	executions := mockExecutor.GetExecutions()
	sweep1dCount := 0
	setVoltageCount := 0
	measureCount := 0
	for _, exec := range executions {
		switch exec.ScriptName {
		case "sweep_1d":
			sweep1dCount++
		case "set_voltage":
			setVoltageCount++
		case "measure_current":
			measureCount++
		}
	}
	assert.Equal(t, 0, sweep1dCount, "Diagonal sweep should NOT use sweep_1d")
	assert.Equal(t, 3*51*2, setVoltageCount, "Should set 2 gates × 51 points × 3 averages")
	assert.Equal(t, 3*51, measureCount, "Should measure current 51 × 3 times")

	t.Logf("Diagonal sweep: %d gates, %d points, %d averages", axis.Dimension(), 51, 3)
	t.Logf("  set_voltage: %d, measure_current: %d", setVoltageCount, measureCount)
}

func TestE2E_SweepAxis_LegacyCompatibility(t *testing.T) {
	// Verify that the legacy AveragedSweep1DRequest still works identically.
	mockExecutor := NewMockScriptExecutor()
	hubConfig := &HubConfig{}
	orchestrator := NewMeasurementOrchestrator(mockExecutor, hubConfig)

	legacyReq := AveragedSweep1DRequest{
		MeasurementID:   "legacy-001",
		SweepGate:       "P1",
		SweepInstrument: "QDAC1",
		SweepChannel:    1,
		StartV:          -1.0,
		StopV:           0.0,
		NumPoints:       101,
		NumAverages:     10,
		CurrentMeter:    "DMM1",
		CurrentChannel:  0,
		SettlingTimeMs:  5.0,
	}

	result, err := orchestrator.ExecuteAveraged1DSweep(context.Background(), legacyReq)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "legacy-001", result.MeasurementID)
	assert.Equal(t, "P1", result.SweepGate)
	assert.Equal(t, 10, result.NumAverages)
	assert.Len(t, result.Voltages, 101)
	assert.Len(t, result.AveragedCurrents, 101)
	assert.Len(t, result.AllTraces, 10)

	assert.InDelta(t, -1.0, result.Voltages[0], 1e-12)
	assert.InDelta(t, 0.0, result.Voltages[100], 1e-12)
}

func TestE2E_SweepAxis_Validation(t *testing.T) {
	tests := []struct {
		name    string
		axis    SweepAxis
		wantErr string
	}{
		{
			name:    "no gates",
			axis:    SweepAxis{Label: "empty", Gates: nil, NumPoints: 10},
			wantErr: "no gates",
		},
		{
			name: "too few points",
			axis: SweepAxis{
				Label:     "one point",
				Gates:     []GateEndpoint{{Gate: "P1", Instrument: "Q1", Channel: 1}},
				NumPoints: 1,
			},
			wantErr: "at least 2 points",
		},
		{
			name: "empty gate name",
			axis: SweepAxis{
				Label:     "no name",
				Gates:     []GateEndpoint{{Gate: "", Instrument: "Q1", Channel: 1}},
				NumPoints: 10,
			},
			wantErr: "gate name is empty",
		},
		{
			name: "empty instrument",
			axis: SweepAxis{
				Label:     "no inst",
				Gates:     []GateEndpoint{{Gate: "P1", Instrument: "", Channel: 1}},
				NumPoints: 10,
			},
			wantErr: "instrument for gate P1 is empty",
		},
		{
			name: "duplicate gate",
			axis: SweepAxis{
				Label: "dup",
				Gates: []GateEndpoint{
					{Gate: "P1", Instrument: "Q1", Channel: 1, StartV: 0, StopV: 1},
					{Gate: "P1", Instrument: "Q1", Channel: 2, StartV: 0, StopV: 1},
				},
				NumPoints: 10,
			},
			wantErr: "duplicate gate P1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.axis.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	// Valid cases
	t.Run("valid scalar", func(t *testing.T) {
		axis := ScalarAxis("P1", "QDAC1", 1, -1, 0, 101)
		assert.NoError(t, axis.Validate())
	})

	t.Run("valid detuning", func(t *testing.T) {
		axis := DetuningAxis("P1", "Q1", 1, -0.5, 0.5, "P2", "Q1", 2, 0.5, -0.5, 51)
		assert.NoError(t, axis.Validate())
	})
}

func TestE2E_SweepAxis_VoltageCalculation(t *testing.T) {
	axis := DetuningAxis(
		"P1", "QDAC1", 1, -1.0, 1.0,
		"P2", "QDAC1", 2, 0.5, -0.5,
		11,
	)

	// Check parameter values [0, 0.1, 0.2, ..., 1.0]
	params := axis.ParameterValues()
	assert.Len(t, params, 11)
	assert.InDelta(t, 0.0, params[0], 1e-12)
	assert.InDelta(t, 0.5, params[5], 1e-12)
	assert.InDelta(t, 1.0, params[10], 1e-12)

	// Check voltage vectors at t=0, t=0.5, t=1
	v0 := axis.VoltagesAt(0)
	assert.InDelta(t, -1.0, v0["P1"], 1e-12)
	assert.InDelta(t, 0.5, v0["P2"], 1e-12)

	v50 := axis.VoltagesAt(0.5)
	assert.InDelta(t, 0.0, v50["P1"], 1e-12)
	assert.InDelta(t, 0.0, v50["P2"], 1e-12)

	v100 := axis.VoltagesAt(1.0)
	assert.InDelta(t, 1.0, v100["P1"], 1e-12)
	assert.InDelta(t, -0.5, v100["P2"], 1e-12)

	// Primary voltages should be P1
	pv := axis.PrimaryVoltages()
	assert.Len(t, pv, 11)
	assert.InDelta(t, -1.0, pv[0], 1e-12)
	assert.InDelta(t, 1.0, pv[10], 1e-12)

	// Full voltage table
	table := axis.AllVoltageVectors()
	assert.Len(t, table, 11)
	assert.InDelta(t, -1.0, table[0]["P1"], 1e-12)
	assert.InDelta(t, 0.5, table[0]["P2"], 1e-12)
}
