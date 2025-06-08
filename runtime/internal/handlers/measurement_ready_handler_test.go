package handlers

import (
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

// Test helper to create a test NATS server
func setupTestNATSServer(t *testing.T) *nats.Conn {
	// For testing purposes, we'll use a mock or embedded NATS server
	// In a real test environment, you might want to use nats-server/test
	// package
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS server not available for testing")
	}
	return nc
}

// setupTestInstrumentHandler creates a real instrument handler for testing
func setupTestInstrumentHandler2(t *testing.T) *instrument.Handler {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create test logger with proper file paths
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Setup NATS connection for the instrument handler
	nc := setupTestNATSServer(t)

	// Create test config with the correct structure
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	// Create instrument handler with the correct signature
	handler, err := instrument.NewHandler(
		logger,
		nats.DefaultURL,
		nc,
		cfg,
		"python",
	)
	require.NoError(t, err)

	// Create mock instruments in the handler for testing
	handler.Instruments = map[string]*instrument.InstrumentProcess{
		"instrument1": {
			Name:        "instrument1",
			Initialized: true,
			Ports: map[string]any{
				"knobs": map[int64]any{
					1: createTestPortJSON("port1", "knobs"),
				},
			},
			Configuration: map[string]any{
				"knobs": map[int64]any{
					1: map[string]any{
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
				},
			},
		},
		"instrument2": {
			Name:        "instrument2",
			Initialized: true,
			Ports: map[string]any{
				"knobs": map[int64]any{
					2: createTestPortJSON("port2", "knobs"),
				},
			},
			Configuration: map[string]any{
				"knobs": map[int64]any{
					2: map[string]any{
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
				},
			},
		},
		"getter_instrument": {
			Name:        "getter_instrument",
			Initialized: true,
			Ports: map[string]any{
				"knobs": map[int64]any{
					10: createTestPortJSON("getter_port", "knobs"),
				},
			},
			Configuration: map[string]any{
				"knobs": map[int64]any{
					10: map[string]any{
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
				},
			},
		},
		"setter_instrument": {
			Name:        "setter_instrument",
			Initialized: true,
			Ports: map[string]any{
				"knobs": map[int64]any{
					20: createTestPortJSON("setter_port", "knobs"),
				},
			},
			Configuration: map[string]any{
				"knobs": map[int64]any{
					20: map[string]any{
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
				},
			},
		},
	}

	return handler
}

// createTestPortJSON creates a test port JSON string with the given port name
// and property
func createTestPortJSON(portName, property string) string {
	port := map[string]interface{}{
		"__class__":       "Knob",
		"__module__":      "falcon_core.instrument_interfaces.names.knob",
		"pseudo_name":     portName, // This is key - the port name goes here
		"instrument_type": "DAC",
		"units":           "V",
		"description":     "Test port",
	}
	data, _ := json.Marshal(port)
	return string(data)
}

func TestMeasurementReadyHandler_UnbufferedMeasurement(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewMeasurementReadyHandler(logger, instrumentHandler, cfg)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create unbuffered measurement request
	port1JSON := createTestPortJSON("port1", "knobs")
	port2JSON := createTestPortJSON("port2", "knobs")

	measurementReady := api.MeasurementReady{
		ProcessId: "test-process-1",
		Getters:   []string{port1JSON, port2JSON}, // Use full JSON strings
		Setters:   []string{},                     // Empty setters = unbuffered
	}

	measurementData, err := json.Marshal(measurementReady)
	require.NoError(t, err)

	// Subscribe to GET commands to simulate instrument responses
	getSubPort1, err := nc.Subscribe(
		GetMessage+".instrument1",
		func(msg *nats.Msg) {
			// Simulate instrument1 responding with RETURN_GET
			returnGet := api.ReturnGet{
				Index:     1,
				Property:  "knobs",
				Value:     42.0,
				Timestamp: time.Now().UnixMicro(),
			}
			returnData, _ := json.Marshal(returnGet)
			nc.Publish(ReturnGetMessage+".instrument1", returnData)
		},
	)
	require.NoError(t, err)
	defer getSubPort1.Unsubscribe()

	getSubPort2, err := nc.Subscribe(
		GetMessage+".instrument2",
		func(msg *nats.Msg) {
			// Simulate instrument2 responding with RETURN_GET
			returnGet := api.ReturnGet{
				Index:     2,
				Property:  "knobs",
				Value:     123.5,
				Timestamp: time.Now().UnixMicro(),
			}
			returnData, _ := json.Marshal(returnGet)
			nc.Publish(ReturnGetMessage+".instrument2", returnData)
		},
	)
	require.NoError(t, err)
	defer getSubPort2.Unsubscribe()

	// Subscribe to PROCESS_DATA to capture the final result
	processDataReceived := make(chan api.ProcessData, 1)
	processDataSub, err := nc.Subscribe(
		ProcessDataSubject,
		func(msg *nats.Msg) {
			var processData api.ProcessData
			json.Unmarshal(msg.Data, &processData)
			processDataReceived <- processData
		},
	)
	require.NoError(t, err)
	defer processDataSub.Unsubscribe()

	// Send MEASUREMENT_READY message
	err = nc.Publish(MeasurementReadySubject, measurementData)
	require.NoError(t, err)

	// Wait for PROCESS_DATA response
	select {
	case processData := <-processDataReceived:
		assert.Equal(t, "test-process-1", processData.ProcessId)

		// Parse the data JSON
		var results map[string]interface{}
		err := json.Unmarshal([]byte(processData.Data), &results)
		require.NoError(t, err)

		// Verify both port results are present
		assert.Equal(t, 42.0, results[port1JSON])
		assert.Equal(t, 123.5, results[port2JSON])

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for PROCESS_DATA")
	}
}

func TestMeasurementReadyHandler_BufferedMeasurement(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewMeasurementReadyHandler(logger, instrumentHandler, cfg)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create buffered measurement request
	getterPortJSON := createTestPortJSON("getter_port", "knobs")
	setterPortJSON := createTestPortJSON("setter_port", "knobs")

	measurementReady := api.MeasurementReady{
		ProcessId: "test-process-buffered",
		Getters:   []string{getterPortJSON},
		Setters:   []string{setterPortJSON}, // Non-empty setters = buffered
	}

	measurementData, err := json.Marshal(measurementReady)
	require.NoError(t, err)

	// Track the sequence of operations using atomic operations
	var triggerCount int64
	var getterArmed int64
	var setterTriggered int64

	// Subscribe to TRIGGER commands for getter instrument
	triggerSubGetter, err := nc.Subscribe(
		TriggerMessage+".getter_instrument",
		func(msg *nats.Msg) {
			atomic.AddInt64(&triggerCount, 1)
			atomic.StoreInt64(&getterArmed, 1)

			// Verify this is the first trigger (getter should be armed first)
			assert.Equal(
				t,
				int64(0),
				atomic.LoadInt64(&setterTriggered),
				"Getter should be armed before setter",
			)
		},
	)
	require.NoError(t, err)
	defer triggerSubGetter.Unsubscribe()

	// Subscribe to TRIGGER commands for setter instrument
	triggerSubSetter, err := nc.Subscribe(
		TriggerMessage+".setter_instrument",
		func(msg *nats.Msg) {
			atomic.AddInt64(&triggerCount, 1)
			atomic.StoreInt64(&setterTriggered, 1)

			// Verify getter was armed first
			assert.Equal(
				t,
				int64(1),
				atomic.LoadInt64(&getterArmed),
				"Getter should be armed before setter",
			)

			// Simulate instrument returning buffered data after setter is
			// triggered
			returnData := api.ReturnData{
				Data:     []interface{}{99.9, 88.8},
				Property: "knobs",
				Index:    10, // This must match the getter_instrument index
			}
			returnDataBytes, _ := json.Marshal(returnData)

			// Publish the RETURN_DATA response
			go func() {
				// Small delay to ensure the trigger processing completes
				time.Sleep(10 * time.Millisecond)
				nc.Publish(
					ReturnDataMessage+".getter_instrument",
					returnDataBytes,
				)
			}()
		},
	)
	require.NoError(t, err)
	defer triggerSubSetter.Unsubscribe()

	// Subscribe to PROCESS_DATA to capture the final result
	processDataReceived := make(chan api.ProcessData, 1)
	processDataSub, err := nc.Subscribe(
		ProcessDataSubject,
		func(msg *nats.Msg) {
			var processData api.ProcessData
			json.Unmarshal(msg.Data, &processData)
			processDataReceived <- processData
		},
	)
	require.NoError(t, err)
	defer processDataSub.Unsubscribe()

	// Send MEASUREMENT_READY message
	err = nc.Publish(MeasurementReadySubject, measurementData)
	require.NoError(t, err)

	// Wait for PROCESS_DATA response
	select {
	case processData := <-processDataReceived:
		assert.Equal(t, "test-process-buffered", processData.ProcessId)

		// Parse the data JSON
		var results map[string]interface{}
		err := json.Unmarshal([]byte(processData.Data), &results)
		require.NoError(t, err)

		// Verify the buffered result
		expectedData := []interface{}{99.9, 88.8}
		assert.Equal(t, expectedData, results[getterPortJSON])
		assert.Greater(t, processData.Timestamp, int64(0))

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for PROCESS_DATA")
	}

	// Verify the sequence
	assert.Equal(
		t,
		int64(2),
		atomic.LoadInt64(&triggerCount),
		"Should have triggered both getter and setter",
	)
	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&getterArmed),
		"Getter should have been armed",
	)
	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&setterTriggered),
		"Setter should have been triggered",
	)
}

func TestMeasurementReadyHandler_BufferedMeasurement_MultipleSetter_Error(
	t *testing.T,
) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewMeasurementReadyHandler(logger, instrumentHandler, cfg)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create buffered measurement request with multiple setters (should log
	// error)
	getterPortJSON := createTestPortJSON("getter_port", "knobs")
	setterPortJSON := createTestPortJSON("setter_port", "knobs")

	measurementReady := api.MeasurementReady{
		ProcessId: "test-process-multi-setter",
		Getters:   []string{getterPortJSON},
		Setters: []string{
			setterPortJSON,
			setterPortJSON,
		}, // Multiple setters (using same port twice)
	}

	measurementData, err := json.Marshal(measurementReady)
	require.NoError(t, err)

	// Count triggers - should still get both getter and setter triggers
	// even with multiple setters (only first setter should be used)
	var triggerCount int64
	var getterTriggerCount int64
	var setterTriggerCount int64

	// Subscribe to getter triggers
	triggerSubGetter, err := nc.Subscribe(
		TriggerMessage+".getter_instrument",
		func(msg *nats.Msg) {
			atomic.AddInt64(&triggerCount, 1)
			atomic.AddInt64(&getterTriggerCount, 1)
		},
	)
	require.NoError(t, err)
	defer triggerSubGetter.Unsubscribe()

	// Subscribe to setter triggers
	triggerSubSetter, err := nc.Subscribe(
		TriggerMessage+".setter_instrument",
		func(msg *nats.Msg) {
			atomic.AddInt64(&triggerCount, 1)
			atomic.AddInt64(&setterTriggerCount, 1)
		},
	)
	require.NoError(t, err)
	defer triggerSubSetter.Unsubscribe()

	// Send MEASUREMENT_READY message
	err = nc.Publish(MeasurementReadySubject, measurementData)
	require.NoError(t, err)

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Should process normally with first setter only
	// Expect both getter and setter to be triggered (total 2)
	assert.Equal(
		t,
		int64(2),
		atomic.LoadInt64(&triggerCount),
		"Should have triggered both getter and setter instruments",
	)
	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&getterTriggerCount),
		"Should have triggered getter instrument once",
	)
	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&setterTriggerCount),
		"Should have triggered setter instrument once (ignoring duplicate setters)",
	)
}

func TestMeasurementReadyHandler_CachingBehavior(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewMeasurementReadyHandler(logger, instrumentHandler, cfg)

	// Create the port JSON that would be used in a real request
	port1JSON := createTestPortJSON("port1", "knobs")

	// First call - should hit the instrument handler
	result1, err1 := handler.getCachedPortConfiguration(port1JSON)
	require.NoError(t, err1)
	assert.Equal(t, "instrument1", result1.Instrument)
	assert.Equal(t, int64(1), result1.Index)
	assert.Equal(t, []string{"knobs"}, result1.Properties)

	// Second call - should use cache
	result2, err2 := handler.getCachedPortConfiguration(port1JSON)
	require.NoError(t, err2)
	assert.Equal(t, result1, result2)
}

func TestMeasurementReadyHandler_InvalidConfiguration(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewMeasurementReadyHandler(logger, instrumentHandler, cfg)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create measurement request with invalid port
	invalidPortJSON := createTestPortJSON("invalid_port", "knobs")

	measurementReady := api.MeasurementReady{
		ProcessId: "test-process-invalid",
		Getters:   []string{invalidPortJSON},
		Setters:   []string{},
	}

	measurementData, err := json.Marshal(measurementReady)
	require.NoError(t, err)

	// Send MEASUREMENT_READY message
	err = nc.Publish(MeasurementReadySubject, measurementData)
	require.NoError(t, err)

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Test should complete without crashing (error handling verification)
}

func TestMeasurementReadyHandler_Subscribe_Unsubscribe(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewMeasurementReadyHandler(logger, instrumentHandler, cfg)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	// Test Subscribe
	err = handler.Subscribe(nc)
	assert.NoError(t, err)
	assert.NotNil(t, handler.subscription)
	assert.NotNil(t, handler.returnGetSub)
	assert.NotNil(t, handler.returnDataSub)

	// Test Unsubscribe
	err = handler.Unsubscribe()
	assert.NoError(t, err)
	assert.Nil(t, handler.subscription)
	assert.Nil(t, handler.returnGetSub)
	assert.Nil(t, handler.returnDataSub)
}

func TestMeasurementReadyHandler_NoGetters_Error(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewMeasurementReadyHandler(logger, instrumentHandler, cfg)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create measurement request with no getters
	measurementReady := api.MeasurementReady{
		ProcessId: "test-process-no-getters",
		Getters:   []string{}, // No getters
		Setters:   []string{},
	}

	measurementData, err := json.Marshal(measurementReady)
	require.NoError(t, err)

	// Send MEASUREMENT_READY message
	err = nc.Publish(MeasurementReadySubject, measurementData)
	require.NoError(t, err)

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Test should complete without crashing (error handling verification)
}

func TestMeasurementReadyHandler_BufferedMeasurement_NoSetters_Error(
	t *testing.T,
) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewMeasurementReadyHandler(logger, instrumentHandler, cfg)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Try to force buffered mode but with no setters
	getterPortJSON := createTestPortJSON("getter_port", "knobs")

	measurementReady := api.MeasurementReady{
		ProcessId: "test-process-buffered-no-setters",
		Getters:   []string{getterPortJSON},
		Setters:   []string{}, // This should trigger unbuffered mode, not buffered
	}

	// Subscribe to PROCESS_DATA to capture any responses (there shouldn't be
	// any)
	measurementData := make(chan api.ProcessData, 1)
	measurementDataSub, err := nc.Subscribe(
		ProcessDataSubject,
		func(msg *nats.Msg) {
			var processData api.ProcessData
			if json.Unmarshal(msg.Data, &processData) == nil {
				select {
				case measurementData <- processData:
				default:
					// Channel full, ignore
				}
			}
		},
	)
	require.NoError(t, err)
	defer measurementDataSub.Unsubscribe()

	// Send the measurement request
	measurementReadyBytes, _ := json.Marshal(measurementReady)
	nc.Publish(MeasurementReadySubject, measurementReadyBytes)

	// Wait a bit to see if any PROCESS_DATA is sent (there shouldn't be)
	select {
	case processData := <-measurementData:
		t.Fatalf("Unexpected PROCESS_DATA received: %+v", processData)
	case <-time.After(1 * time.Second):
		// Expected behavior - no PROCESS_DATA should be sent
		t.Log(
			"Correctly did not receive PROCESS_DATA for unbuffered measurement with no setters",
		)
	}
}
