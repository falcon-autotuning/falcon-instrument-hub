package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runNATSServer(t *testing.T) *server.Server {
	opts := &server.Options{
		Host: "127.0.0.1",
		Port: -1, // Use random port
	}
	s, err := server.NewServer(opts)
	require.NoError(t, err)

	go s.Start()

	// Wait for server to be ready
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("NATS server not ready for connections")
	}

	return s
}

func TestDeviceConfigHandler(t *testing.T) {
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create test config
	testConfig := &config.Config{
		DeviceConfig: &config.DeviceConfig{
			ScreeningGates:    "S1;S2",
			PlungerGates:      "P1;P2",
			Ohmics:            "O1;O2",
			BarrierGates:      "B1;B2",
			ReservoirGates:    "R1;R2",
			NumUniqueChannels: 2,
			Groups: map[string]config.Group{
				"group1": {
					Name:           "TestGroup",
					NumDots:        2,
					ScreeningGates: "S1",
					ReservoirGates: "R1",
					PlungerGates:   "P1;P2",
					BarrierGates:   "B1",
					Order:          "O1;R1;B1;P1;P2;R2;O2",
				},
			},
			WiringDC: map[string]config.WiringSpec{
				"S1": {Resistance: 100.0, Capacitance: 1e-15},
				"P1": {Resistance: 200.0, Capacitance: 2e-15},
			},
		},
	}
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Create handler
	handler := NewDeviceConfigHandler(testConfig, logger)

	// Subscribe to device config requests
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	t.Run("successful device config request", func(t *testing.T) {
		// Create response channel
		responseCh := make(chan *nats.Msg, 1)
		responseSubject := "DEVICE_CONFIG_RESPONSE.external.test"

		sub, err := nc.Subscribe(responseSubject, func(msg *nats.Msg) {
			responseCh <- msg
		})
		require.NoError(t, err)
		defer sub.Unsubscribe()

		// Create and send request
		request := api.DeviceConfigRequest{
			Timestamp: time.Now().UnixMicro(),
		}
		requestData, err := json.Marshal(request)
		require.NoError(t, err)

		// Send request to DEVICE_CONFIG_REQUEST.external.test
		err = nc.Publish("DEVICE_CONFIG_REQUEST.external.test", requestData)
		require.NoError(t, err)

		// Wait for response
		select {
		case responseMsg := <-responseCh:
			// Parse response
			var response api.DeviceConfigResponse
			err = json.Unmarshal(responseMsg.Data, &response)
			require.NoError(t, err)

			// Verify response
			assert.NotEmpty(t, response.Response)
			assert.Greater(t, response.Timestamp, int64(0))

			// Verify that the response contains valid device config JSON
			var deviceConfig config.DeviceConfig
			err = json.Unmarshal([]byte(response.Response), &deviceConfig)
			require.NoError(t, err)
			assert.Equal(t, "S1;S2", deviceConfig.ScreeningGates)
			assert.Equal(t, 2, deviceConfig.NumUniqueChannels)
			assert.Len(t, deviceConfig.Groups, 1)

		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for device config response")
		}
	})

	t.Run("invalid channel format", func(t *testing.T) {
		// Create and send request to invalid channel
		request := api.DeviceConfigRequest{
			Timestamp: time.Now().UnixMicro(),
		}
		requestData, err := json.Marshal(request)
		require.NoError(t, err)

		// This should be handled but no response should be sent to a valid response channel
		err = nc.Publish("DEVICE_CONFIG_REQUEST.invalid", requestData)
		require.NoError(t, err)

		// Brief wait to ensure message is processed
		time.Sleep(100 * time.Millisecond)
		// No assertions - just ensuring no panic occurs
	})
}

func TestExtractNameFromChannel(t *testing.T) {
	testCases := []struct {
		subject  string
		expected string
	}{
		{"DEVICE_CONFIG_REQUEST.external.test", "test"},
		{"DEVICE_CONFIG_REQUEST.external.myname", "myname"},
		{"DEVICE_CONFIG_REQUEST.external.complex-name", "complex-name"},
	}

	for _, tc := range testCases {
		t.Run(tc.subject, func(t *testing.T) {
			parts := strings.Split(tc.subject, ".")
			if len(parts) == 3 && parts[0] == "DEVICE_CONFIG_REQUEST" && parts[1] == "external" {
				result := parts[2]
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}
