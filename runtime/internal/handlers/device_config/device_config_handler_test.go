package deviceconfighandlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runNATSServer(t *testing.T) *server.Server {
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // Use random port
		JetStream: true,
		StoreDir:  t.TempDir(),
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
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	testConfig := `{"ScreeningGates":"S1;S2","NumUniqueChannels":2}`

	tempDir := t.TempDir()

	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	handler := NewDeviceConfigHandler(testConfig, logger)

	require.NoError(t, handler.Subscribe(nc))
	defer handler.Unsubscribe()

	responseCh := make(chan *nats.Msg, 1)

	sub, err := nc.Subscribe(deviceConfigResponseSubject, func(msg *nats.Msg) {
		responseCh <- msg
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	require.NoError(t, nc.Flush())

	request := api.DeviceConfigRequest{
		Timestamp: time.Now().UnixMicro(),
	}

	requestData, err := json.Marshal(request)
	require.NoError(t, err)

	require.NoError(t, nc.Publish(deviceConfigRequestSubject, requestData))
	require.NoError(t, nc.Flush())

	select {
	case responseMsg := <-responseCh:
		var response api.DeviceConfigResponse
		require.NoError(t, json.Unmarshal(responseMsg.Data, &response))

		assert.Equal(t, testConfig, response.Response)
		assert.Greater(t, response.Timestamp, int64(0))

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for device config response")
	}
}
