package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
)

// setupTestInstrumentHandlerForPortRequest creates an instrument handler whose
// PortConnections are populated directly for testing (no API YAML files needed).
func setupTestInstrumentHandlerForPortRequest(
	t *testing.T,
) *instrument.Handler {
	t.Helper()
	tempDir := t.TempDir()

	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() { logger.Close() })

	nc := setupTestNATSServer(t)

	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}

	handler, err := instrument.NewHandler(
		logger,
		nats.DefaultURL,
		nc,
		cfg,
	)
	require.NoError(t, err)

	// Populate port connections directly for testing.
	handler.PortConnections = []ports.ConnectedPort{
		{
			PortName:       "Mock.DAC.analog.knob1",
			DeviceName:     "P1",
			InstrumentName: "dac1",
			ChannelName:    "analog",
			ChannelIndex:   1,
			Role:           "output",
			Unit:           "V",
			Description:    "Test knob 1",
		},
		{
			PortName:       "Mock.DAC.analog.knob2",
			DeviceName:     "P2",
			InstrumentName: "dac1",
			ChannelName:    "analog",
			ChannelIndex:   2,
			Role:           "output",
			Unit:           "V",
			Description:    "Test knob 2",
		},
		{
			PortName:       "Mock.DAC.analog.meter1",
			DeviceName:     "M1",
			InstrumentName: "dac2",
			ChannelName:    "analog",
			ChannelIndex:   1,
			Role:           "input",
			Unit:           "A",
			Description:    "Test meter 1",
		},
		{
			PortName:       "Mock.DAC.analog.meter2",
			DeviceName:     "M2",
			InstrumentName: "dac2",
			ChannelName:    "analog",
			ChannelIndex:   2,
			Role:           "input",
			Unit:           "A",
			Description:    "Test meter 2",
		},
	}

	return handler
}

func TestNewPortRequestHandler(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
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
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
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

func TestPortRequestHandler_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}

	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	// Setup NATS
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Send invalid JSON - should not crash
	err = nc.Publish(PortRequestSubject, []byte("invalid json"))
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
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
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
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}

	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	// Should not error when unsubscribing without subscription
	err = handler.Unsubscribe()
	require.NoError(t, err)
}

// TestPortRequestHandler_CollectPortProperties verifies that CollectPortProperties
// correctly partitions the PortConnections into knobs and meters.
func TestPortRequestHandler_CollectPortProperties(t *testing.T) {
	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)

	knobs, meters := instrumentHandler.CollectPortProperties()

	assert.Len(t, knobs, 2, "expected 2 knobs")
	assert.Len(t, meters, 2, "expected 2 meters")

	for _, k := range knobs {
		assert.True(t, k.IsKnob())
	}
	for _, m := range meters {
		assert.True(t, m.IsMeter())
	}
}

// TestPortRequestHandler_E2E exercises the full PORT_REQUEST → PORT_PAYLOAD
// flow. It requires CGO and the falcon-core library.
func TestPortRequestHandler_E2E(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	instrumentHandler := setupTestInstrumentHandlerForPortRequest(t)
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
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
	sub, err := nc.Subscribe(PortPayloadSubject, func(msg *nats.Msg) {
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

	err = nc.Publish(PortRequestSubject, requestData)
	require.NoError(t, err)

	// Wait for response
	select {
	case response := <-responseReceived:
		assert.Equal(t, request.Timestamp, response.Timestamp)
		assert.Contains(t, response.Knobs, "[")
		assert.Contains(t, response.Knobs, "]")
		assert.Contains(t, response.Meters, "[")
		assert.Contains(t, response.Meters, "]")

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for PORT_PAYLOAD response")
	}
}
