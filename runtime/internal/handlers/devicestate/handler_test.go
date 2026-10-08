package devicestatehandler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/devicestate"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runNATSServer(t *testing.T) *server.Server {
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}

	s, err := server.NewServer(opts)
	require.NoError(t, err)

	go s.Start()

	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("NATS server not ready")
	}

	return s
}

func TestHandler_EndToEnd(t *testing.T) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	logger, err := logging.NewLogger(t.TempDir())
	require.NoError(t, err)
	defer logger.Close()

	devicestate.Close()

	require.NoError(t,
		devicestate.Startup(
			devicestate.DeviceVoltageStates{
				"P1": {
					Connection: devicestate.Connection{
						Name: "P1",
						Type: devicestate.PlungerGate,
					},
					Quantity: devicestate.Quantity{
						Value: 1.23,
						Unit:  devicestate.Volt,
					},
				},
			},
		),
	)
	defer devicestate.Close()

	handler := NewHandler(logger)

	require.NoError(t, handler.Subscribe(nc))
	defer handler.Unsubscribe()

	responseCh := make(chan *nats.Msg, 1)

	sub, err := nc.Subscribe(
		deviceStateResponseSubject,
		func(msg *nats.Msg) {
			responseCh <- msg
		},
	)
	require.NoError(t, err)
	defer sub.Unsubscribe()

	require.NoError(t, nc.Flush())

	req := api.DeviceStateRequest{
		Timestamp: time.Now().UnixMicro(),
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	require.NoError(
		t,
		nc.Publish(
			deviceStateRequestSubject,
			data,
		),
	)

	require.NoError(t, nc.Flush())

	select {
	case msg := <-responseCh:
		var response api.DeviceStateResponse

		require.NoError(
			t,
			json.Unmarshal(msg.Data, &response),
		)

		assert.NotEmpty(t, response.Response)
		assert.Greater(t, response.Timestamp, int64(0))

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestParseRequest(t *testing.T) {
	handler := &Handler{}

	req := api.DeviceStateRequest{
		Timestamp: 123,
	}

	raw, err := json.Marshal(req)
	require.NoError(t, err)

	var parsed api.DeviceStateRequest

	err = handler.parseRequest(raw, &parsed)

	require.NoError(t, err)
	assert.Equal(t, req.Timestamp, parsed.Timestamp)
}

func TestParseRequest_InvalidJSON(t *testing.T) {
	handler := &Handler{}

	var parsed api.DeviceStateRequest

	err := handler.parseRequest(
		[]byte("{invalid"),
		&parsed,
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"failed to decode device config request JSON",
	)
}

func TestSubscribe(t *testing.T) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	logger, err := logging.NewLogger(t.TempDir())
	require.NoError(t, err)
	defer logger.Close()

	handler := NewHandler(logger)

	require.NoError(
		t,
		handler.Subscribe(nc),
	)

	assert.NotNil(t, handler.GetSubscription())
}

func TestUnsubscribe(t *testing.T) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	logger, err := logging.NewLogger(t.TempDir())
	require.NoError(t, err)
	defer logger.Close()

	handler := NewHandler(logger)

	require.NoError(
		t,
		handler.Subscribe(nc),
	)

	require.NotNil(t, handler.GetSubscription())

	require.NoError(
		t,
		handler.Unsubscribe(),
	)

	assert.Nil(t, handler.GetSubscription())
}

func TestUnsubscribe_NoSubscription(t *testing.T) {
	handler := &Handler{}

	require.NoError(
		t,
		handler.Unsubscribe(),
	)
}

func TestGetSubscription(t *testing.T) {
	handler := &Handler{}

	assert.Nil(
		t,
		handler.GetSubscription(),
	)
}
