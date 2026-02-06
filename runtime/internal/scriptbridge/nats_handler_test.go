package scriptbridge

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runNATSServer starts a test NATS server and returns it.
func runTestNATSServer(t *testing.T) *server.Server {
	opts := &server.Options{
		Host:   "127.0.0.1",
		Port:   -1, // Auto-select port
		NoLog:  true,
		NoSigs: true,
	}

	s, err := server.NewServer(opts)
	require.NoError(t, err)

	go s.Start()

	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server failed to start")
	}

	return s
}

func TestNATSBridgeHandler_SubscribeUnsubscribe(t *testing.T) {
	// Start test NATS server
	natsServer := runTestNATSServer(t)
	defer natsServer.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(natsServer.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create mock instrument server
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	// Parse mock server URL
	host, port := parseMockURL(mock.URL())

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
		ScriptOutputDir:  t.TempDir(),
	}

	handler, err := NewNATSBridgeHandler(config)
	require.NoError(t, err)

	// Test subscription
	err = handler.Subscribe(nc, "TEST_BRIDGE")
	assert.NoError(t, err)

	// Test unsubscription
	err = handler.Unsubscribe()
	assert.NoError(t, err)

	// Test double unsubscribe
	err = handler.Unsubscribe()
	assert.NoError(t, err)
}

func TestNATSBridgeHandler_HandleMessage(t *testing.T) {
	// Start test NATS server
	natsServer := runTestNATSServer(t)
	defer natsServer.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(natsServer.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create mock instrument server
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	host, port := parseMockURL(mock.URL())

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
		ScriptOutputDir:  t.TempDir(),
	}

	handler, err := NewNATSBridgeHandler(config)
	require.NoError(t, err)

	// Subscribe
	err = handler.Subscribe(nc, "TEST_BRIDGE")
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Subscribe to response
	responseChan := make(chan NATSMeasurementResponse, 1)
	responseSub, err := nc.Subscribe("MEASURE_RESPONSE.external", func(msg *nats.Msg) {
		var resp NATSMeasurementResponse
		if err := json.Unmarshal(msg.Data, &resp); err == nil {
			select {
			case responseChan <- resp:
			default:
			}
		}
	})
	require.NoError(t, err)
	defer responseSub.Unsubscribe()

	// Create and send test command
	requestJSON := `{
		"measurementName": "test_from_nats",
		"setters": [{"id": "DAC1", "channel": 0}],
		"setVoltages": {"DAC1": 1.5}
	}`

	command := NATSMeasurementCommand{
		Request:   requestJSON,
		Timestamp: time.Now().UnixMicro(),
		Hash:      12345,
	}

	commandData, err := json.Marshal(command)
	require.NoError(t, err)

	err = nc.Publish("TEST_BRIDGE.test", commandData)
	require.NoError(t, err)

	// Wait for response
	select {
	case resp := <-responseChan:
		assert.Equal(t, command.Hash, resp.Hash)
		assert.NotEmpty(t, resp.Response)

		// Parse the response JSON
		var result ExecutionResult
		err := json.Unmarshal([]byte(resp.Response), &result)
		require.NoError(t, err)
		assert.Equal(t, "completed", result.Status)

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

func TestNATSBridgeHandler_InvalidJSON(t *testing.T) {
	// Start test NATS server
	natsServer := runTestNATSServer(t)
	defer natsServer.Shutdown()

	nc, err := nats.Connect(natsServer.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	host, port := parseMockURL(mock.URL())

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
		ScriptOutputDir:  t.TempDir(),
	}

	handler, err := NewNATSBridgeHandler(config)
	require.NoError(t, err)

	err = handler.Subscribe(nc, "TEST_BRIDGE")
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Send invalid JSON
	err = nc.Publish("TEST_BRIDGE.test", []byte("invalid json"))
	require.NoError(t, err)

	// Wait a bit to ensure no crash
	time.Sleep(100 * time.Millisecond)
}

func TestNATSBridgeHandler_EmptyRequest(t *testing.T) {
	// Start test NATS server
	natsServer := runTestNATSServer(t)
	defer natsServer.Shutdown()

	nc, err := nats.Connect(natsServer.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	host, port := parseMockURL(mock.URL())

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
		ScriptOutputDir:  t.TempDir(),
	}

	handler, err := NewNATSBridgeHandler(config)
	require.NoError(t, err)

	err = handler.Subscribe(nc, "TEST_BRIDGE")
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Send command with empty request
	command := NATSMeasurementCommand{
		Request:   "{}",
		Timestamp: time.Now().UnixMicro(),
		Hash:      99999,
	}

	commandData, err := json.Marshal(command)
	require.NoError(t, err)

	err = nc.Publish("TEST_BRIDGE.test", commandData)
	require.NoError(t, err)

	// Wait a bit to ensure no crash
	time.Sleep(100 * time.Millisecond)
}

// Helper function to parse mock server URL
func parseMockURL(url string) (string, int) {
	// URL is like "http://127.0.0.1:12345"
	url = url[7:] // Remove "http://"
	for i, c := range url {
		if c == ':' {
			host := url[:i]
			portStr := url[i+1:]
			var port int
			for _, d := range portStr {
				if d >= '0' && d <= '9' {
					port = port*10 + int(d-'0')
				}
			}
			return host, port
		}
	}
	return "127.0.0.1", 8555
}

func TestNATSMeasurementCommand_JSON(t *testing.T) {
	cmd := NATSMeasurementCommand{
		Request:   `{"test": "value"}`,
		Timestamp: 1234567890,
		Hash:      42,
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	var parsed NATSMeasurementCommand
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, cmd.Request, parsed.Request)
	assert.Equal(t, cmd.Timestamp, parsed.Timestamp)
	assert.Equal(t, cmd.Hash, parsed.Hash)
}

func TestNATSMeasurementResponse_JSON(t *testing.T) {
	resp := NATSMeasurementResponse{
		Response:  `{"status": "completed"}`,
		Timestamp: 1234567890,
		Hash:      42,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed NATSMeasurementResponse
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, resp.Response, parsed.Response)
	assert.Equal(t, resp.Timestamp, parsed.Timestamp)
	assert.Equal(t, resp.Hash, parsed.Hash)
}
