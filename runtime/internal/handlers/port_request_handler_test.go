package handlers

import (
	"encoding/json"
	"path/filepath"
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

// setupTestInstrumentHandler creates a real instrument handler with mock
// instruments for testing
func setupTestInstrumentHandlerForPortRequest(
	t *testing.T,
) *instrument.Handler {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create test logger with proper file paths
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() { logger.Close() })

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
	)
	require.NoError(t, err)

	// Create mock instruments in the handler for testing
	handler.Instruments = map[instrument.Name]*instrument.InstrumentProcess{
		"dac1": {
			Name:        "dac1",
			Initialized: true,
			Ports: map[instrument.PropertyName]map[instrument.Index]instrument.JsonPort{
				"knobs": {
					"0": createTestKnobJSON("DAC", "knob1"),
					"1": createTestKnobJSON("DAC", "knob2"),
				},
			},
			Configuration: map[instrument.PropertyName]map[instrument.Index]instrument.PortConfiguration{
				"knobs": {
					"0": {
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
					"1": {
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
				},
			},
		},
		"dac2": {
			Name:        "dac2",
			Initialized: true,
			Ports: map[instrument.PropertyName]map[instrument.Index]instrument.JsonPort{
				"meters": {
					"0": createTestMeterJSON("DAC", "meter1"),
					"1": createTestMeterJSON("DAC", "meter2"),
				},
			},
			Configuration: map[instrument.PropertyName]map[instrument.Index]instrument.PortConfiguration{
				"meters": {
					"0": map[string]any{
						"bounds": []float64{0, 100},
						"unit":   "A",
					},
					"1": map[string]any{
						"bounds": []float64{0, 100},
						"unit":   "A",
					},
				},
			},
		},
		"ohmic_instrument": {
			Name:        "ohmic_instrument",
			Initialized: true,
			Ports: map[instrument.PropertyName]map[instrument.Index]instrument.JsonPort{
				"meters": {
					"0": createTestOhmicMeterJSON("DAC", "ohmic_meter"),
				},
			},
			Configuration: map[instrument.PropertyName]map[instrument.Index]instrument.PortConfiguration{
				"meters": {
					"0": instrument.PortConfiguration{
						"bounds": []float64{0, 100},
						"unit":   "A",
					},
				},
			},
		},
	}

	return handler
}

// createTestKnobJSON creates a test knob JSON string
func createTestKnobJSON(instrumentType, portName string) instrument.JsonPort {
	knob := map[string]interface{}{
		"__class__":       "Knob",
		"__module__":      "falcon_core.instrument_interfaces.names.knob",
		"pseudo_name":     portName,
		"instrument_type": instrumentType,
		"units":           "V",
		"description":     "Test knob",
	}
	data, _ := json.Marshal(knob)
	return instrument.JsonPort(data)
}

// createTestMeterJSON creates a test meter JSON string
func createTestMeterJSON(instrumentType, portName string) instrument.JsonPort {
	meter := map[string]interface{}{
		"__class__":       "Meter",
		"__module__":      "falcon_core.instrument_interfaces.names.meter",
		"pseudo_name":     portName,
		"instrument_type": instrumentType,
		"units":           "A",
		"description":     "Test meter",
	}
	data, _ := json.Marshal(meter)
	return instrument.JsonPort(data)
}

// createTestOhmicMeterJSON creates a test ohmic meter JSON string
func createTestOhmicMeterJSON(
	instrumentType, portName string,
) instrument.JsonPort {
	meter := map[string]interface{}{
		"__class__":       "Meter",
		"__module__":      "falcon_core.instrument_interfaces.names.meter",
		"pseudo_name":     portName,
		"instrument_type": instrumentType,
		"connection_type": "Ohmic",
		"units":           "A",
		"description":     "Test ohmic meter",
	}
	data, _ := json.Marshal(meter)
	return instrument.JsonPort(data)
}

func TestNewPortRequestHandler(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	assert.NotNil(t, handler)
	assert.Equal(t, logger, handler.logger)
	assert.Equal(t, instrumentHandler, handler.instrumentHandler)
	assert.Equal(t, cfg, handler.config)
}

func TestPortRequestHandler_Subscribe_Unsubscribe(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	// Setup NATS
	nc := setupTestNATSServer(t)
	defer nc.Close()

	// Test Subscribe
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	assert.NotNil(t, handler.subscription)
	assert.Equal(t, nc, handler.nc)

	// Test Unsubscribe
	err = handler.Unsubscribe()
	require.NoError(t, err)
	assert.Nil(t, handler.subscription)
}

func TestPortRequestHandler_E2E(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	// Setup NATS
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Subscribe to response
	responseReceived := make(chan *api.PortPayload, 1)
	sub, err := nc.Subscribe("PORT_PAYLOAD.external.test", func(msg *nats.Msg) {
		var payload api.PortPayload
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			responseReceived <- &payload
		}
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	// Send PORT_REQUEST
	request := api.PortRequest{
		Timestamp: time.Now().UnixMicro(),
	}
	requestData, err := json.Marshal(request)
	require.NoError(t, err)

	err = nc.Publish("PORT_REQUEST.external.test", requestData)
	require.NoError(t, err)

	// Wait for response
	select {
	case response := <-responseReceived:
		assert.NotEmpty(t, response.Knobs, "Should have knobs in response")
		assert.NotEmpty(t, response.Meters, "Should have meters in response")
		assert.Equal(t, request.Timestamp, response.Timestamp)

		// Verify knobs are properly formatted
		assert.Contains(t, response.Knobs, "[")
		assert.Contains(t, response.Knobs, "]")

		// Verify meters are properly formatted
		assert.Contains(t, response.Meters, "[")
		assert.Contains(t, response.Meters, "]")

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for PORT_PAYLOAD response")
	}
}

func TestPortRequestHandler_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	// Setup NATS
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Send invalid JSON - should not crash
	err = nc.Publish("PORT_REQUEST.external.test", []byte("invalid json"))
	require.NoError(t, err)

	// Give it time to process - should not crash
	time.Sleep(100 * time.Millisecond)
}

func TestPortRequestHandler_isOhmicConnection(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	tests := []struct {
		name     string
		portJSON string
		expected bool
	}{
		{
			name:     "Ohmic connection",
			portJSON: `{"connection_type":"Ohmic","other_field":"value"}`,
			expected: true,
		},
		{
			name:     "Non-Ohmic connection",
			portJSON: `{"connection_type":"Capacitive","other_field":"value"}`,
			expected: false,
		},
		{
			name:     "Missing connection_type",
			portJSON: `{"other_field":"value"}`,
			expected: false,
		},
		{
			name:     "Invalid JSON",
			portJSON: `invalid json`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.isOhmicConnection(tt.portJSON)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPortRequestHandler_UnsubscribeWithoutSubscription(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	// Should not error when unsubscribing without subscription
	err = handler.Unsubscribe()
	require.NoError(t, err)
}
