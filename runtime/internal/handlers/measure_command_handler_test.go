package handlers

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
)

// MockBusyManager implements BusyManager interface for testing
type MockBusyManager struct {
	isBusy bool
	mutex  sync.RWMutex
}

func (m *MockBusyManager) SetIsBusy(busy bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.isBusy = busy
}

func (m *MockBusyManager) IsBusy() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.isBusy
}

func TestMeasureCommandHandler_HandleMessage(t *testing.T) {
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create temporary directory
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Create mock measurements manager
	measurementManager, err := measurements.NewManager(
		tempDir+"/data",
		tempDir+"/test.db",
	)
	require.NoError(t, err)

	// Create instrument handler with test config
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}
	instrumentHandler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		cfg,
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	// Create mock busy manager
	mockBusyManager := &MockBusyManager{}

	// Create measure command handler
	handler := NewMeasureCommandHandler(
		logger,
		measurementManager,
		instrumentHandler,
		mockBusyManager,
	)

	// Subscribe to handler
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	t.Run("successful_measure_command", func(t *testing.T) {
		// Set up response subscriptions with channels for synchronization
		responseChan := make(chan api.MeasureResponse, 1)
		processRequestChan := make(chan api.ProcessRequest, 1)

		// Subscribe to MEASURE_RESPONSE
		responseSub, err := nc.Subscribe(
			"MEASURE_RESPONSE.external.test",
			func(msg *nats.Msg) {
				var receivedResponse api.MeasureResponse
				if err := json.Unmarshal(msg.Data, &receivedResponse); err == nil {
					select {
					case responseChan <- receivedResponse:
					default:
					}
				}
			},
		)
		require.NoError(t, err)
		defer responseSub.Unsubscribe()

		// Subscribe to PROCESS_REQUEST
		processRequestSub, err := nc.Subscribe(
			"PROCESS_REQUEST.interpreter",
			func(msg *nats.Msg) {
				var receivedProcessRequest api.ProcessRequest
				if err := json.Unmarshal(msg.Data, &receivedProcessRequest); err == nil {
					select {
					case processRequestChan <- receivedProcessRequest:
					default:
					}
				}
			},
		)
		require.NoError(t, err)
		defer processRequestSub.Unsubscribe()

		// Send MEASURE_COMMAND
		measureCommand := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      12345,
			Request:   "test_measurement_request",
		}

		commandData, err := json.Marshal(measureCommand)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.test", commandData)
		require.NoError(t, err)

		// Wait for PROCESS_REQUEST first
		select {
		case receivedProcessRequest := <-processRequestChan:
			assert.Equal(
				t,
				measureCommand.Request,
				receivedProcessRequest.Request,
				"Request should be passed through",
			)
			assert.NotEmpty(
				t,
				receivedProcessRequest.ProcessId,
				"ProcessId should be set",
			)
			assert.NotEmpty(
				t,
				receivedProcessRequest.DataPath,
				"DataPath should be set",
			)
			assert.NotEmpty(
				t,
				receivedProcessRequest.Configurations,
				"Configurations should be set",
			)
		case <-time.After(2 * time.Second):
			t.Fatal("Did not receive PROCESS_REQUEST within timeout")
		}

		// Now send UPLOAD_DATA to complete the flow
		uploadData := api.UploadData{
			Data: "measurement result data",
		}

		uploadDataBytes, err := json.Marshal(uploadData)
		require.NoError(t, err)

		err = nc.Publish("UPLOAD_DATA", uploadDataBytes)
		require.NoError(t, err)

		// Wait for MEASURE_RESPONSE
		select {
		case receivedResponse := <-responseChan:
			assert.Equal(
				t,
				measureCommand.Hash,
				receivedResponse.Hash,
				"Hash should be transferred",
			)
			assert.Equal(
				t,
				uploadData.Data,
				receivedResponse.Response,
				"Response should contain uploaded data",
			)
			assert.Greater(
				t,
				receivedResponse.Timestamp,
				int64(0),
				"Timestamp should be set",
			)
		case <-time.After(2 * time.Second):
			t.Fatal("Did not receive MEASURE_RESPONSE within timeout")
		}
	})

	t.Run("invalid_subject_format", func(t *testing.T) {
		// Send command with invalid subject
		measureCommand := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      67890,
			Request:   "test_request",
		}

		commandData, err := json.Marshal(measureCommand)
		require.NoError(t, err)

		// Send with invalid subject (missing name part)
		err = nc.Publish("MEASURE_COMMAND.external", commandData)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Should be handled gracefully (no crash)
	})

	t.Run("invalid_json", func(t *testing.T) {
		// Send invalid JSON
		err = nc.Publish(
			"MEASURE_COMMAND.external.test",
			[]byte("invalid json"),
		)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Should be handled gracefully (no crash)
	})

	t.Run("empty_request", func(t *testing.T) {
		// Send command with empty request
		measureCommand := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      11111,
			Request:   "", // Empty request
		}

		commandData, err := json.Marshal(measureCommand)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.test", commandData)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Should be handled (empty request is valid)
	})
}

func TestMeasureCommandHandler_WithInstruments(t *testing.T) {
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create temporary directory
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Create mock measurements manager
	measurementManager, err := measurements.NewManager(
		tempDir+"/data",
		tempDir+"/test.db",
	)
	require.NoError(t, err)

	// Create test config with device mappings
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{
			ScreeningGates: "SG1;SG2",
			Ohmics:         "OH1;OH2",
		},
		WireMap: &config.WireMap{
			"dac1.0": "SG1",
			"dac1.1": "OH1",
		},
	}

	// Create instrument handler
	instrumentHandler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		cfg,
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	// Set up mock instruments with ports and configurations
	mockInstrument := &instrument.InstrumentProcess{
		Name:        "dac1",
		Initialized: true,
		Ports: map[string]interface{}{
			"knobs": map[int64]interface{}{
				0: createTestKnobJSON("DAC", "SG1"),
				1: createTestKnobJSON("DAC", "OH1"),
			},
		},
		Configuration: map[string]interface{}{
			"knobs": map[int64]interface{}{
				0: map[string]interface{}{
					"bounds": []float64{-10, 10},
					"unit":   "V",
				},
				1: map[string]interface{}{
					"bounds": []float64{-5, 5},
					"unit":   "V",
				},
			},
		},
	}

	// Add to handler (note: this is direct access for testing)
	instrumentHandler.Instruments["dac1"] = mockInstrument

	// Create mock busy manager
	mockBusyManager := &MockBusyManager{}

	// Create measure command handler
	handler := NewMeasureCommandHandler(
		logger,
		measurementManager,
		instrumentHandler,
		mockBusyManager,
	)

	// Subscribe to handler
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	t.Run("configurations_built_from_instruments", func(t *testing.T) {
		processRequestChan := make(chan api.ProcessRequest, 1)

		// Subscribe to PROCESS_REQUEST to verify configurations
		processRequestSub, err := nc.Subscribe(
			"PROCESS_REQUEST.interpreter",
			func(msg *nats.Msg) {
				var receivedProcessRequest api.ProcessRequest
				if err := json.Unmarshal(msg.Data, &receivedProcessRequest); err == nil {
					select {
					case processRequestChan <- receivedProcessRequest:
					default:
					}
				}
			},
		)
		require.NoError(t, err)
		defer processRequestSub.Unsubscribe()

		// Send MEASURE_COMMAND
		measureCommand := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      98765,
			Request:   "test_with_instruments",
		}

		commandData, err := json.Marshal(measureCommand)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.test", commandData)
		require.NoError(t, err)

		// Wait for PROCESS_REQUEST
		select {
		case receivedProcessRequest := <-processRequestChan:
			assert.NotEmpty(
				t,
				receivedProcessRequest.Configurations,
				"Configurations should not be empty",
			)

			// Parse configurations JSON to verify structure
			var configurations map[string]map[string]interface{}
			err = json.Unmarshal(
				[]byte(receivedProcessRequest.Configurations),
				&configurations,
			)
			assert.NoError(t, err, "Configurations should be valid JSON")

			// Should contain port configurations from mock instruments
			assert.Greater(
				t,
				len(configurations),
				0,
				"Should have port configurations",
			)
		case <-time.After(2 * time.Second):
			t.Fatal("Did not receive PROCESS_REQUEST within timeout")
		}
	})
}

func TestMeasureCommandHandler_EdgeCases(t *testing.T) {
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create temporary directory
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Create mock measurements manager
	measurementManager, err := measurements.NewManager(
		tempDir+"/data",
		tempDir+"/test.db",
	)
	require.NoError(t, err)

	// Create instrument handler
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}
	instrumentHandler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		cfg,
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	// Create mock busy manager
	mockBusyManager := &MockBusyManager{}

	// Create handler
	handler := NewMeasureCommandHandler(
		logger,
		measurementManager,
		instrumentHandler,
		mockBusyManager,
	)

	t.Run("subscribe_and_unsubscribe", func(t *testing.T) {
		// Test subscription
		err := handler.Subscribe(nc)
		assert.NoError(t, err, "Should subscribe successfully")

		// Test unsubscription
		err = handler.Unsubscribe()
		assert.NoError(t, err, "Should unsubscribe successfully")

		// Test double unsubscribe (should be safe)
		err = handler.Unsubscribe()
		assert.NoError(t, err, "Should handle double unsubscribe gracefully")
	})

	t.Run("large_hash_values", func(t *testing.T) {
		err := handler.Subscribe(nc)
		require.NoError(t, err)
		defer handler.Unsubscribe()

		// Test with large hash value
		measureCommand := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      9223372036854775807, // Max int64
			Request:   "test_large_hash",
		}

		commandData, err := json.Marshal(measureCommand)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.test", commandData)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Should be handled without issues
	})
}

func TestMeasureCommandHandler_UploadData(t *testing.T) {
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create temporary directory
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Create mock measurements manager
	measurementManager, err := measurements.NewManager(
		tempDir+"/data",
		tempDir+"/test.db",
	)
	require.NoError(t, err)

	// Create instrument handler with test config
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}
	instrumentHandler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		cfg,
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	// Create mock busy manager
	mockBusyManager := &MockBusyManager{}

	// Create handler
	handler := NewMeasureCommandHandler(
		logger,
		measurementManager,
		instrumentHandler,
		mockBusyManager,
	)

	// Subscribe to handler
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	t.Run("successful_upload_data_flow", func(t *testing.T) {
		// Set up response subscription
		responseChan := make(chan api.MeasureResponse, 1)
		responseSub, err := nc.Subscribe(
			"MEASURE_RESPONSE.external.upload-test",
			func(msg *nats.Msg) {
				var receivedResponse api.MeasureResponse
				if err := json.Unmarshal(msg.Data, &receivedResponse); err == nil {
					select {
					case responseChan <- receivedResponse:
					default:
					}
				}
			},
		)
		require.NoError(t, err)
		defer responseSub.Unsubscribe()

		// First, send MEASURE_COMMAND to create a pending measurement
		measureCommand := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      99999,
			Request:   "test_upload_request",
		}

		commandData, err := json.Marshal(measureCommand)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.upload-test", commandData)
		require.NoError(t, err)

		// Wait a moment for MEASURE_COMMAND to be processed
		time.Sleep(100 * time.Millisecond)

		// Now send UPLOAD_DATA
		uploadData := api.UploadData{
			Data: "test measurement data from upload",
		}

		uploadDataBytes, err := json.Marshal(uploadData)
		require.NoError(t, err)

		err = nc.Publish("UPLOAD_DATA", uploadDataBytes)
		require.NoError(t, err)

		// Wait for MEASURE_RESPONSE
		select {
		case receivedResponse := <-responseChan:
			assert.Equal(
				t,
				measureCommand.Hash,
				receivedResponse.Hash,
				"Hash should match original command",
			)
			assert.Equal(
				t,
				uploadData.Data,
				receivedResponse.Response,
				"Response should contain uploaded data",
			)
			assert.Greater(
				t,
				receivedResponse.Timestamp,
				int64(0),
				"Timestamp should be set",
			)
		case <-time.After(2 * time.Second):
			t.Fatal("Did not receive MEASURE_RESPONSE within timeout")
		}
	})

	t.Run("upload_data_without_pending_measurement", func(t *testing.T) {
		// Send UPLOAD_DATA without any pending measurements
		uploadData := api.UploadData{
			Data: "orphaned upload data",
		}

		uploadDataBytes, err := json.Marshal(uploadData)
		require.NoError(t, err)

		err = nc.Publish("UPLOAD_DATA", uploadDataBytes)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(200 * time.Millisecond)

		// Should be handled gracefully (no crash, just logged error)
	})

	t.Run("invalid_upload_data_json", func(t *testing.T) {
		// Send invalid JSON for UPLOAD_DATA
		err = nc.Publish("UPLOAD_DATA", []byte("invalid json"))
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Should be handled gracefully (no crash)
	})

	t.Run("empty_upload_data", func(t *testing.T) {
		// Set up response subscription
		responseChan := make(chan api.MeasureResponse, 1)
		responseSub, err := nc.Subscribe(
			"MEASURE_RESPONSE.external.empty-test",
			func(msg *nats.Msg) {
				var receivedResponse api.MeasureResponse
				if err := json.Unmarshal(msg.Data, &receivedResponse); err == nil {
					select {
					case responseChan <- receivedResponse:
					default:
					}
				}
			},
		)
		require.NoError(t, err)
		defer responseSub.Unsubscribe()

		// Send MEASURE_COMMAND first
		measureCommand := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      88888,
			Request:   "test_empty_upload",
		}

		commandData, err := json.Marshal(measureCommand)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.empty-test", commandData)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Send UPLOAD_DATA with empty data
		uploadData := api.UploadData{
			Data: "", // Empty data
		}

		uploadDataBytes, err := json.Marshal(uploadData)
		require.NoError(t, err)

		err = nc.Publish("UPLOAD_DATA", uploadDataBytes)
		require.NoError(t, err)

		// Wait for MEASURE_RESPONSE
		select {
		case receivedResponse := <-responseChan:
			assert.Equal(
				t,
				measureCommand.Hash,
				receivedResponse.Hash,
				"Hash should match",
			)
			assert.Equal(
				t,
				"",
				receivedResponse.Response,
				"Response should be empty string",
			)
		case <-time.After(2 * time.Second):
			t.Fatal("Did not receive MEASURE_RESPONSE within timeout")
		}
	})

	t.Run("large_upload_data", func(t *testing.T) {
		// Set up response subscription
		responseChan := make(chan api.MeasureResponse, 1)
		responseSub, err := nc.Subscribe(
			"MEASURE_RESPONSE.external.large-test",
			func(msg *nats.Msg) {
				var receivedResponse api.MeasureResponse
				if err := json.Unmarshal(msg.Data, &receivedResponse); err == nil {
					select {
					case responseChan <- receivedResponse:
					default:
					}
				}
			},
		)
		require.NoError(t, err)
		defer responseSub.Unsubscribe()

		// Send MEASURE_COMMAND first
		measureCommand := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      77777,
			Request:   "test_large_upload",
		}

		commandData, err := json.Marshal(measureCommand)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.large-test", commandData)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		// Create large data payload
		largeData := string(make([]byte, 10000)) // 10KB of null bytes
		uploadData := api.UploadData{
			Data: largeData,
		}

		uploadDataBytes, err := json.Marshal(uploadData)
		require.NoError(t, err)

		err = nc.Publish("UPLOAD_DATA", uploadDataBytes)
		require.NoError(t, err)

		// Wait for MEASURE_RESPONSE
		select {
		case receivedResponse := <-responseChan:
			assert.Equal(
				t,
				measureCommand.Hash,
				receivedResponse.Hash,
				"Hash should match",
			)
			assert.Equal(
				t,
				largeData,
				receivedResponse.Response,
				"Response should contain large data",
			)
		case <-time.After(3 * time.Second):
			t.Fatal("Did not receive MEASURE_RESPONSE within timeout")
		}
	})
}

func TestMeasureCommandHandler_IsBusyFlag(t *testing.T) {
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create temporary directory
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Create mock measurements manager
	measurementManager, err := measurements.NewManager(
		tempDir+"/data",
		tempDir+"/test.db",
	)
	require.NoError(t, err)

	// Create instrument handler with test config
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}
	instrumentHandler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		cfg,
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	// Create mock busy manager
	mockBusyManager := &MockBusyManager{}

	// Create handler
	handler := NewMeasureCommandHandler(
		logger,
		measurementManager,
		instrumentHandler,
		mockBusyManager,
	)

	// Subscribe to handler
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	t.Run("isBusy_flag_lifecycle", func(t *testing.T) {
		// Initially, IsBusy should be false
		assert.False(
			t,
			mockBusyManager.IsBusy(),
			"IsBusy should initially be false",
		)

		// Set up response subscriptions
		responseChan := make(chan api.MeasureResponse, 1)

		// Subscribe to MEASURE_RESPONSE
		responseSub, err := nc.Subscribe(
			"MEASURE_RESPONSE.external.busy-test",
			func(msg *nats.Msg) {
				var receivedResponse api.MeasureResponse
				if err := json.Unmarshal(msg.Data, &receivedResponse); err == nil {
					select {
					case responseChan <- receivedResponse:
					default:
					}
				}
			},
		)
		require.NoError(t, err)
		defer responseSub.Unsubscribe()

		// Send MEASURE_COMMAND
		measureCommand := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      123456,
			Request:   "test_busy_flag",
		}

		commandData, err := json.Marshal(measureCommand)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.busy-test", commandData)
		require.NoError(t, err)

		// Wait for command to be processed
		time.Sleep(100 * time.Millisecond)

		// IsBusy should now be true
		assert.True(
			t,
			mockBusyManager.IsBusy(),
			"IsBusy should be true after MEASURE_COMMAND",
		)

		// Send UPLOAD_DATA to complete the measurement
		uploadData := api.UploadData{
			Data: "test busy flag data",
		}

		uploadDataBytes, err := json.Marshal(uploadData)
		require.NoError(t, err)

		err = nc.Publish("UPLOAD_DATA", uploadDataBytes)
		require.NoError(t, err)

		// Wait for MEASURE_RESPONSE
		select {
		case receivedResponse := <-responseChan:
			assert.Equal(t, measureCommand.Hash, receivedResponse.Hash)
			assert.Equal(t, uploadData.Data, receivedResponse.Response)
		case <-time.After(2 * time.Second):
			t.Fatal("Did not receive MEASURE_RESPONSE within timeout")
		}

		// Brief wait to ensure IsBusy flag is reset
		time.Sleep(50 * time.Millisecond)

		// IsBusy should now be false again
		assert.False(
			t,
			mockBusyManager.IsBusy(),
			"IsBusy should be false after MEASURE_RESPONSE",
		)
	})

	t.Run("isBusy_flag_reset_on_error", func(t *testing.T) {
		// Initially, IsBusy should be false
		assert.False(
			t,
			mockBusyManager.IsBusy(),
			"IsBusy should initially be false",
		)

		// Send invalid JSON to trigger error path
		err = nc.Publish(
			"MEASURE_COMMAND.external.error-test",
			[]byte("invalid json"),
		)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// IsBusy should remain false since the command failed to parse
		assert.False(
			t,
			mockBusyManager.IsBusy(),
			"IsBusy should remain false on JSON parse error",
		)

		// Test allocation error by using an invalid measurementManager
		// (This is harder to test without modifying the manager, so we'll test
		// the happy path above)
	})

	t.Run("isBusy_flag_multiple_commands", func(t *testing.T) {
		// Initially, IsBusy should be false
		assert.False(
			t,
			mockBusyManager.IsBusy(),
			"IsBusy should initially be false",
		)

		// Send first MEASURE_COMMAND
		measureCommand1 := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      111111,
			Request:   "first_command",
		}

		commandData1, err := json.Marshal(measureCommand1)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.multi-test", commandData1)
		require.NoError(t, err)

		// Wait for first command to be processed
		time.Sleep(100 * time.Millisecond)

		// IsBusy should be true
		assert.True(
			t,
			mockBusyManager.IsBusy(),
			"IsBusy should be true after first command",
		)

		// Send second MEASURE_COMMAND while first is still busy
		measureCommand2 := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      222222,
			Request:   "second_command",
		}

		commandData2, err := json.Marshal(measureCommand2)
		require.NoError(t, err)

		err = nc.Publish("MEASURE_COMMAND.external.multi-test", commandData2)
		require.NoError(t, err)

		// Wait for second command to be processed
		time.Sleep(100 * time.Millisecond)

		// IsBusy should still be true (second command should also set it to
		// true)
		assert.True(
			t,
			mockBusyManager.IsBusy(),
			"IsBusy should remain true with multiple commands",
		)

		// Complete measurements by sending UPLOAD_DATA twice
		for i := 0; i < 2; i++ {
			uploadData := api.UploadData{
				Data: fmt.Sprintf("upload_data_%d", i),
			}

			uploadDataBytes, err := json.Marshal(uploadData)
			require.NoError(t, err)

			err = nc.Publish("UPLOAD_DATA", uploadDataBytes)
			require.NoError(t, err)

			time.Sleep(50 * time.Millisecond)
		}

		// Wait for all processing to complete
		time.Sleep(200 * time.Millisecond)

		// IsBusy should be false after the last response is sent
		assert.False(
			t,
			mockBusyManager.IsBusy(),
			"IsBusy should be false after all measurements complete",
		)
	})
}

func TestMeasureCommandHandler_MultipleUploadData(t *testing.T) {
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create temporary directory
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Create mock measurements manager
	measurementManager, err := measurements.NewManager(
		tempDir+"/data",
		tempDir+"/test.db",
	)
	require.NoError(t, err)

	// Create instrument handler
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}
	instrumentHandler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		cfg,
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	// Create mock busy manager
	mockBusyManager := &MockBusyManager{}

	// Create measure command handler
	handler := NewMeasureCommandHandler(
		logger,
		measurementManager,
		instrumentHandler,
		mockBusyManager,
	)

	// Subscribe to handler
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	t.Run("multiple_measurements_with_uploads", func(t *testing.T) {
		numMeasurements := 3
		responseChan := make(chan api.MeasureResponse, numMeasurements)

		// Subscribe to all responses
		responseSub, err := nc.Subscribe(
			"MEASURE_RESPONSE.external.multi-test",
			func(msg *nats.Msg) {
				var receivedResponse api.MeasureResponse
				if err := json.Unmarshal(msg.Data, &receivedResponse); err == nil {
					select {
					case responseChan <- receivedResponse:
					default:
					}
				}
			},
		)
		require.NoError(t, err)
		defer responseSub.Unsubscribe()

		// Send multiple MEASURE_COMMANDs
		expectedHashes := make([]int64, numMeasurements)
		for i := 0; i < numMeasurements; i++ {
			hash := int64(10000 + i)
			expectedHashes[i] = hash

			measureCommand := api.MeasureCommand{
				Timestamp: time.Now().UnixMicro(),
				Hash:      hash,
				Request:   fmt.Sprintf("test_multi_request_%d", i),
			}

			commandData, err := json.Marshal(measureCommand)
			require.NoError(t, err)

			err = nc.Publish("MEASURE_COMMAND.external.multi-test", commandData)
			require.NoError(t, err)

			// Small delay between commands
			time.Sleep(10 * time.Millisecond)
		}

		// Wait for all commands to be processed
		time.Sleep(200 * time.Millisecond)

		// Send corresponding UPLOAD_DATA messages
		for i := 0; i < numMeasurements; i++ {
			uploadData := api.UploadData{
				Data: fmt.Sprintf("upload_data_%d", i),
			}

			uploadDataBytes, err := json.Marshal(uploadData)
			require.NoError(t, err)

			err = nc.Publish("UPLOAD_DATA", uploadDataBytes)
			require.NoError(t, err)

			// Small delay between uploads
			time.Sleep(10 * time.Millisecond)
		}

		// Collect all responses
		receivedHashes := make(map[int64]bool)
		for i := 0; i < numMeasurements; i++ {
			select {
			case response := <-responseChan:
				receivedHashes[response.Hash] = true
				assert.NotEmpty(
					t,
					response.Response,
					"Response should not be empty",
				)
				assert.Greater(
					t,
					response.Timestamp,
					int64(0),
					"Timestamp should be set",
				)
			case <-time.After(5 * time.Second):
				t.Fatalf(
					"Timeout waiting for response %d/%d",
					i+1,
					numMeasurements,
				)
			}
		}

		// Verify all expected hashes were received
		for _, expectedHash := range expectedHashes {
			assert.True(
				t,
				receivedHashes[expectedHash],
				"Should have received response for hash %d",
				expectedHash,
			)
		}
	})

	t.Run("upload_data_timing_with_concurrent_commands", func(t *testing.T) {
		// Test that UPLOAD_DATA correctly correlates with pending measurements
		// even when commands arrive in quick succession

		responseChan := make(chan api.MeasureResponse, 2)
		responseSub, err := nc.Subscribe(
			"MEASURE_RESPONSE.external.timing-test",
			func(msg *nats.Msg) {
				var receivedResponse api.MeasureResponse
				if err := json.Unmarshal(msg.Data, &receivedResponse); err == nil {
					select {
					case responseChan <- receivedResponse:
					default:
					}
				}
			},
		)
		require.NoError(t, err)
		defer responseSub.Unsubscribe()

		// Send two MEASURE_COMMANDs rapidly
		hash1 := int64(20001)
		hash2 := int64(20002)

		measureCommand1 := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      hash1,
			Request:   "concurrent_request_1",
		}

		measureCommand2 := api.MeasureCommand{
			Timestamp: time.Now().UnixMicro(),
			Hash:      hash2,
			Request:   "concurrent_request_2",
		}

		commandData1, err := json.Marshal(measureCommand1)
		require.NoError(t, err)
		commandData2, err := json.Marshal(measureCommand2)
		require.NoError(t, err)

		// Send both commands quickly
		err = nc.Publish("MEASURE_COMMAND.external.timing-test", commandData1)
		require.NoError(t, err)
		err = nc.Publish("MEASURE_COMMAND.external.timing-test", commandData2)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(200 * time.Millisecond)

		// Send UPLOAD_DATA - should correlate with first available pending
		// measurement
		uploadData1 := api.UploadData{
			Data: "first_upload_data",
		}
		uploadDataBytes1, err := json.Marshal(uploadData1)
		require.NoError(t, err)

		err = nc.Publish("UPLOAD_DATA", uploadDataBytes1)
		require.NoError(t, err)

		// Wait and send second upload
		time.Sleep(100 * time.Millisecond)

		uploadData2 := api.UploadData{
			Data: "second_upload_data",
		}
		uploadDataBytes2, err := json.Marshal(uploadData2)
		require.NoError(t, err)

		err = nc.Publish("UPLOAD_DATA", uploadDataBytes2)
		require.NoError(t, err)

		// Collect responses
		receivedResponses := make([]api.MeasureResponse, 0, 2)
		for i := 0; i < 2; i++ {
			select {
			case response := <-responseChan:
				receivedResponses = append(receivedResponses, response)
			case <-time.After(3 * time.Second):
				t.Fatalf("Timeout waiting for response %d", i+1)
			}
		}

		// Verify we got both responses
		assert.Len(t, receivedResponses, 2, "Should receive both responses")

		// Verify hashes match our original commands
		receivedHashes := make([]int64, len(receivedResponses))
		for i, resp := range receivedResponses {
			receivedHashes[i] = resp.Hash
		}

		assert.Contains(t, receivedHashes, hash1, "Should contain hash1")
		assert.Contains(t, receivedHashes, hash2, "Should contain hash2")
	})
}
