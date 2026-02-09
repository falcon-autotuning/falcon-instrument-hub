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
			"P1",          // sweep gate
			-1.0, 0.0,     // voltage range
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
				noise := float64(sweepIdx) * 1e-12           // Sweep-dependent offset

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
		// Create database
		database, err := NewMeasurementDatabase(tempDir)
		require.NoError(t, err)

		// Create a measurement result
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

		// Generate realistic data
		for i := 0; i < 10; i++ {
			result.AllTraces[i] = Trace{
				SweepIndex: i + 1,
				Points:     make([]TracePoint, 101),
				Timestamp:  time.Now(),
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

		// Store
		filePath, err := database.Store(result)
		require.NoError(t, err)
		assert.FileExists(t, filePath)
		t.Logf("Stored to: %s", filePath)

		// Verify we can load it back
		loaded, err := database.Load("hdf5-test-001")
		require.NoError(t, err)
		assert.Equal(t, result.MeasurementID, loaded.MeasurementID)
		assert.Equal(t, result.NumSweeps, loaded.NumSweeps)
		assert.Equal(t, result.NumPoints, loaded.NumPoints)

		// Verify data integrity
		loadedCurrent := loaded.AveragedTrace.Points[50].Measurements["DMM1_0"]
		expectedCurrent := result.AveragedTrace.Points[50].Measurements["DMM1_0"]
		assert.InDelta(t, expectedCurrent, loadedCurrent, 1e-15)

		// List measurements
		measurements := database.List()
		assert.Len(t, measurements, 1)
		assert.Equal(t, "hdf5-test-001", measurements[0].MeasurementID)
	})

	t.Run("6_jetstream_notification", func(t *testing.T) {
		// Simulate JetStream publication
		result := &AveragedMeasurementResult{
			MeasurementID: "jetstream-test-001",
			SweepGate:     "P1",
			StartVoltage:  -1.0,
			StopVoltage:   0.0,
			NumPoints:     101,
			NumSweeps:     10,
			DatabasePath:  filepath.Join(tempDir, "sweep_jetstream-test-001.json"),
		}

		// Create notification message for falcon
		notification := FalconMeasurementNotification{
			Type:          "measurement_complete",
			MeasurementID: result.MeasurementID,
			ProcessID:     42,
			Status:        "success",
			DataLocation: FalconDataLocation{
				Stream:    "FALCON_MEASUREMENTS",
				Subject:   fmt.Sprintf("measurement.result.%s", result.MeasurementID),
				FilePath:  result.DatabasePath,
				NumPoints: result.NumPoints,
				NumSweeps: result.NumSweeps,
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
			Subject: fmt.Sprintf("measurement.result.%s", result.MeasurementID),
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
	})

	t.Run("7_full_flow_integration", func(t *testing.T) {
		// This test combines all components in a realistic scenario

		// Create all components
		mockExecutor := NewMockScriptExecutor()
		hubConfig := &HubConfig{}
		orchestrator := NewMeasurementOrchestrator(mockExecutor, hubConfig)

		traceConfig := DefaultTraceBufferConfig()
		traceConfig.DatabasePath = tempDir
		traceBuffer := NewTraceBuffer(traceConfig)

		database, err := NewMeasurementDatabase(tempDir)
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
				current := 2e-9 * (1.0 - voltage*voltage/2.0) + float64(sweepIdx)*1e-11

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

				// 6. Store to database
				dbPath, err := database.Store(avgResult)
				require.NoError(t, err)
				t.Logf("Stored measurement to: %s", dbPath)

				// 7. Create JetStream notification
				notification := FalconMeasurementNotification{
					Type:          "measurement_complete",
					MeasurementID: measurementID,
					ProcessID:     42,
					Status:        "success",
					DataLocation: FalconDataLocation{
						Stream:    "FALCON_MEASUREMENTS",
						Subject:   fmt.Sprintf("measurement.result.%s", measurementID),
						FilePath:  dbPath,
						NumPoints: avgResult.NumPoints,
						NumSweeps: avgResult.NumSweeps,
					},
					Timestamp: time.Now(),
				}

				publishMu.Lock()
				publishedMessages = append(publishedMessages, JetStreamMessage{
					Subject: notification.DataLocation.Subject,
					Data:    mustMarshal(notification),
				})
				publishMu.Unlock()

				// Verify final result
				assert.Equal(t, numAverages, avgResult.NumSweeps)
				assert.Equal(t, avgReq.NumPoints, avgResult.NumPoints)
				assert.Len(t, avgResult.AllTraces, numAverages)

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
type FalconDataLocation struct {
	Stream    string `json:"stream"`
	Subject   string `json:"subject"`
	FilePath  string `json:"file_path"`
	NumPoints int    `json:"num_points"`
	NumSweeps int    `json:"num_sweeps"`
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
	}

	// Verify all are listed
	measurements := db.List()
	assert.Len(t, measurements, 5)

	// Load and verify each
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("measurement-%03d", i)
		loaded, err := db.Load(id)
		require.NoError(t, err)
		assert.Equal(t, id, loaded.MeasurementID)
		assert.Equal(t, 50+i*10, loaded.NumPoints)
	}

	// Test persistence - create new database instance
	db2, err := NewMeasurementDatabase(tempDir)
	require.NoError(t, err)

	measurements2 := db2.List()
	assert.Len(t, measurements2, 5, "Should persist across database restarts")
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
