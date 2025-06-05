package handlers

import (
	"encoding/json"
	"fmt"
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

func TestPortRequestHandler_HumanReadableNames(t *testing.T) {
	// Setup test configuration
	deviceConfig := &config.DeviceConfig{
		ScreeningGates: "SG1;SG2;SG3",
		BarrierGates:   "BG1;BG2",
		ReservoirGates: "RG1;RG2",
		PlungerGates:   "PG1;PG2",
		Ohmics:         "OH1;OH2",
	}

	wireMap := &config.WireMap{
		"dac1.0":            "SG1",
		"dac1.1":            "BG1",
		"dac1.2":            "PG1",
		"dac2.0":            "OH1",
		"dac2.1":            "OH2",
		"ignored.with.dots": "should_be_ignored",
	}

	cfg := &config.Config{
		DeviceConfig: deviceConfig,
		WireMap:      wireMap,
	}

	// Setup test instruments with ports
	instrumentHandler := setupTestInstrumentHandler()

	// Create port request handler
	logger, err := logging.NewLogger("test")
	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	// Setup NATS connection for testing
	nc, err := nats.Connect(nats.DefaultURL)
	require.NoError(t, err)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Test the port collection
	knobs, meters := handler.collectPortProperties()

	// Verify we got the expected number of ports
	assert.Greater(t, len(knobs), 0, "Should have collected knobs")
	assert.Greater(t, len(meters), 0, "Should have collected meters")

	// Verify knobs contain human-readable names
	var knobObjects []map[string]interface{}
	err = json.Unmarshal([]byte(fmt.Sprintf("[%s]", knobs[0])), &knobObjects)
	require.NoError(t, err)

	// Check that the first knob has been augmented with human-readable name
	firstKnob := knobObjects[0]
	assert.Equal(
		t,
		"SG1",
		firstKnob["pseudo_name"],
		"Knob should have human-readable pseudo_name",
	)
	assert.Contains(
		t,
		firstKnob,
		"device_connection",
		"Knob should have device_connection field",
	)
	assert.Contains(
		t,
		firstKnob,
		"connection_name",
		"Knob should have connection_name field",
	)
	assert.Equal(
		t,
		"ScreeningGate",
		firstKnob["connection_type"],
		"Should have correct connection type",
	)

	// Verify meters contain human-readable names (only Ohmics)
	var meterObjects []map[string]interface{}
	err = json.Unmarshal([]byte(fmt.Sprintf("[%s]", meters[0])), &meterObjects)
	require.NoError(t, err)

	firstMeter := meterObjects[0]
	assert.Equal(
		t,
		"OH1",
		firstMeter["pseudo_name"],
		"Meter should have human-readable pseudo_name",
	)
	assert.Equal(
		t,
		"Ohmic",
		firstMeter["connection_type"],
		"Meter should be Ohmic type",
	)
}

func TestPortRequestHandler_FallbackToInstrumentType(t *testing.T) {
	// Setup configuration with limited mappings
	deviceConfig := &config.DeviceConfig{
		ScreeningGates: "SG1",
		Ohmics:         "OH1",
	}

	wireMap := &config.WireMap{
		"dac1.0": "SG1", // Only map one port
		// dac1.1 will not be mapped, should fallback to instrument type
	}

	cfg := &config.Config{
		DeviceConfig: deviceConfig,
		WireMap:      wireMap,
	}

	instrumentHandler := setupTestInstrumentHandler()
	logger, err := logging.NewLogger("test")
	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	knobs, _ := handler.collectPortProperties()

	// Parse the knobs to check fallback behavior
	require.Greater(t, len(knobs), 1, "Should have multiple knobs for testing")

	// First knob should have human-readable name
	var firstKnobObj map[string]interface{}
	err = json.Unmarshal([]byte(knobs[0]), &firstKnobObj)
	require.NoError(t, err)
	assert.Equal(t, "SG1", firstKnobObj["pseudo_name"])

	// Second knob should fallback to instrument type
	var secondKnobObj map[string]interface{}
	err = json.Unmarshal([]byte(knobs[1]), &secondKnobObj)
	require.NoError(t, err)
	assert.Equal(
		t,
		"DAC",
		secondKnobObj["pseudo_name"],
		"Should fallback to instrument_type",
	)
}

func setupTestInstrumentHandler() *instrument.Handler {
	// Create mock instrument processes with ports
	instrumentHandler := &instrument.Handler{
		Instruments: make(map[string]*instrument.InstrumentProcess),
	}

	// Create test ports for dac1 (knobs) - as JSON strings to match real
	// instrument data
	dac1Ports := map[string]interface{}{
		"knobs": map[int64]interface{}{
			0: createTestKnobJSON("DAC"),
			1: createTestKnobJSON("DAC"),
		},
	}

	// Create test ports for dac2 (meters - ohmics) - as JSON strings
	dac2Ports := map[string]interface{}{
		"meters": map[int64]interface{}{
			0: createTestMeterJSON("DAC"),
			1: createTestMeterJSON("DAC"),
		},
	}

	instrumentHandler.Instruments["dac1"] = &instrument.InstrumentProcess{
		Name:        "dac1",
		Ports:       dac1Ports,
		Initialized: true,
	}

	instrumentHandler.Instruments["dac2"] = &instrument.InstrumentProcess{
		Name:        "dac2",
		Ports:       dac2Ports,
		Initialized: true,
	}

	return instrumentHandler
}

func createTestKnobJSON(instrumentType string) string {
	knob := map[string]interface{}{
		"__class__":       "Knob",
		"__module__":      "falcon_core.instrument_interfaces.names.knob",
		"pseudo_name":     "",
		"instrument_type": instrumentType,
		"units":           "V",
		"description":     "Test knob",
	}
	data, _ := json.Marshal(knob)
	return string(data)
}

func createTestMeterJSON(instrumentType string) string {
	meter := map[string]interface{}{
		"__class__":       "Meter",
		"__module__":      "falcon_core.instrument_interfaces.names.meter",
		"pseudo_name":     "",
		"instrument_type": instrumentType,
		"units":           "A",
		"description":     "Test meter",
	}
	data, _ := json.Marshal(meter)
	return string(data)
}

func TestPortRequestHandler_E2E(t *testing.T) {
	// End-to-end test with actual NATS messaging
	deviceConfig := &config.DeviceConfig{
		ScreeningGates: "SG1;SG2",
		Ohmics:         "OH1",
	}

	wireMap := &config.WireMap{
		"dac1.0": "SG1",
		"dac2.0": "OH1",
	}

	cfg := &config.Config{
		DeviceConfig: deviceConfig,
		WireMap:      wireMap,
	}

	instrumentHandler := setupTestInstrumentHandler()
	logger, err := logging.NewLogger("test")
	handler := NewPortRequestHandler(logger, instrumentHandler, cfg)

	// Setup NATS
	nc, err := nats.Connect(nats.DefaultURL)
	require.NoError(t, err)
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

		// Verify knobs contain human-readable names
		var knobs []string
		err = json.Unmarshal([]byte(response.Knobs), &knobs)
		require.NoError(t, err)
		assert.Greater(t, len(knobs), 0, "Should have knob entries")

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for PORT_PAYLOAD response")
	}
}
