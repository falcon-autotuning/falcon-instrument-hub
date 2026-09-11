package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
	"github.com/nats-io/nats.go"
)

func testCapabilityHandler(t *testing.T) *CapabilityLookupHandler {
	t.Helper()

	logger, err := logging.NewLogger(t.TempDir())
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}
	t.Cleanup(func() { logger.Close() })

	instrumentHandler := &instrument.Handler{
		PortConnections: []ports.ConnectedPort{
			{
				PortName:       "Mock.Meter1.analog.trigger_level",
				DeviceName:     "O1",
				InstrumentName: "Meter1",
				ChannelName:    "analog",
				ChannelIndex:   1,
				IoTypeName:     "trigger_level",
				InstrumentType: "voltmeter",
				Role:           "setting",
				Unit:           "V",
				Description:    "Trigger level setting for multimeter channel",
			},
			{
				PortName:       "Mock.Meter1.analog.voltage",
				DeviceName:     "O1",
				InstrumentName: "Meter1",
				ChannelName:    "analog",
				ChannelIndex:   1,
				IoTypeName:     "voltage",
				InstrumentType: "voltmeter",
				Role:           "input",
				Unit:           "V",
				Description:    "Measured DC voltage",
			},
		},
	}
	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{
			Ohmics: "O1",
		},
	}

	return NewCapabilityLookupHandler(logger, instrumentHandler, cfg)
}

func TestCapabilityLookupHandlerResolveCapability(t *testing.T) {
	handler := testCapabilityHandler(t)
	request := api.CapabilityRequest{
		DeviceName: "O1",
		Capability: "trigger_level",
		Role:       "setting",
		Timestamp:  42,
	}

	response := handler.resolveCapability(request)
	if response.Error != "" {
		t.Fatalf("CapabilityPayload.Error = %q, want empty", response.Error)
	}
	if response.Timestamp != request.Timestamp {
		t.Fatalf("Timestamp = %d, want %d", response.Timestamp, request.Timestamp)
	}
	if response.PortName != "Mock.Meter1.analog.trigger_level" {
		t.Fatalf("PortName = %q", response.PortName)
	}
	if response.DeviceName != "O1" {
		t.Fatalf("DeviceName = %q", response.DeviceName)
	}
	if response.InstrumentName != "Meter1" {
		t.Fatalf("InstrumentName = %q", response.InstrumentName)
	}
	if response.ChannelName != "analog" {
		t.Fatalf("ChannelName = %q", response.ChannelName)
	}
	if response.ChannelIndex != 1 {
		t.Fatalf("ChannelIndex = %d", response.ChannelIndex)
	}
	if response.Capability != "trigger_level" {
		t.Fatalf("Capability = %q", response.Capability)
	}
	if response.Role != "setting" {
		t.Fatalf("Role = %q", response.Role)
	}
	if response.Unit != "V" {
		t.Fatalf("Unit = %q", response.Unit)
	}
	if response.Port == "" {
		t.Fatal("Port cereal JSON is empty")
	}

	port, err := instrumentport.FromJSON(response.Port)
	if err != nil {
		t.Fatalf("instrumentport.FromJSON returned error: %v", err)
	}
	defer port.Close()
}

func TestCapabilityLookupHandlerResolveCapabilityError(t *testing.T) {
	handler := testCapabilityHandler(t)
	response := handler.resolveCapability(api.CapabilityRequest{
		DeviceName: "O1",
		Capability: "trigger_level",
		Role:       "input",
		Timestamp:  43,
	})

	if response.Timestamp != 43 {
		t.Fatalf("Timestamp = %d, want 43", response.Timestamp)
	}
	if response.DeviceName != "O1" {
		t.Fatalf("DeviceName = %q", response.DeviceName)
	}
	if response.Capability != "trigger_level" {
		t.Fatalf("Capability = %q", response.Capability)
	}
	if response.Role != "input" {
		t.Fatalf("Role = %q", response.Role)
	}
	if response.Error == "" {
		t.Fatal("CapabilityPayload.Error is empty")
	}
	if response.Port != "" {
		t.Fatalf("Port = %q, want empty on error", response.Port)
	}
}

func TestCapabilityLookupHandlerResolveCapabilityValidationError(t *testing.T) {
	handler := testCapabilityHandler(t)
	response := handler.resolveCapability(api.CapabilityRequest{
		Capability: "trigger_level",
		Role:       "setting",
		Timestamp:  44,
	})

	if response.Error != "device_name is required" {
		t.Fatalf("Error = %q, want device_name is required", response.Error)
	}
	if response.Timestamp != 44 {
		t.Fatalf("Timestamp = %d, want 44", response.Timestamp)
	}
}

func TestCapabilityLookupHandlerE2E(t *testing.T) {
	handler := testCapabilityHandler(t)
	nc := setupTestNATSServer(t)
	defer nc.Close()

	if err := handler.Subscribe(nc); err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer handler.Unsubscribe()

	responseReceived := make(chan api.CapabilityPayload, 1)
	sub, err := nc.Subscribe(CapabilityPayloadSubj, func(msg *nats.Msg) {
		var payload api.CapabilityPayload
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			responseReceived <- payload
		}
	})
	if err != nil {
		t.Fatalf("Subscribe response returned error: %v", err)
	}
	defer sub.Unsubscribe()

	request := api.CapabilityRequest{
		DeviceName: "O1",
		Capability: "trigger_level",
		Role:       "setting",
		Timestamp:  time.Now().UnixMicro(),
	}
	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Marshal request returned error: %v", err)
	}
	if err := nc.Publish(CapabilityRequestSubj, requestData); err != nil {
		t.Fatalf("Publish request returned error: %v", err)
	}

	select {
	case response := <-responseReceived:
		if response.Error != "" {
			t.Fatalf("CapabilityPayload.Error = %q, want empty", response.Error)
		}
		if response.Timestamp != request.Timestamp {
			t.Fatalf("Timestamp = %d, want %d", response.Timestamp, request.Timestamp)
		}
		if response.PortName != "Mock.Meter1.analog.trigger_level" {
			t.Fatalf("PortName = %q", response.PortName)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for CAPABILITY_PAYLOAD response")
	}
}

func TestCapabilityLookupHandlerNATSReplySubject(t *testing.T) {
	handler := testCapabilityHandler(t)
	nc := setupTestNATSServer(t)
	defer nc.Close()

	if err := handler.Subscribe(nc); err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	defer handler.Unsubscribe()

	request := api.CapabilityRequest{
		DeviceName: "O1",
		Capability: "trigger_level",
		Role:       "setting",
		Timestamp:  time.Now().UnixMicro(),
	}
	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Marshal request returned error: %v", err)
	}

	msg, err := nc.Request(CapabilityRequestSubj, requestData, 5*time.Second)
	if err != nil {
		t.Fatalf("Request returned error: %v", err)
	}

	var response api.CapabilityPayload
	if err := json.Unmarshal(msg.Data, &response); err != nil {
		t.Fatalf("Unmarshal response returned error: %v", err)
	}
	if response.Error != "" {
		t.Fatalf("CapabilityPayload.Error = %q, want empty", response.Error)
	}
	if !strings.Contains(response.PortName, "trigger_level") {
		t.Fatalf("PortName = %q, want trigger_level", response.PortName)
	}
}
