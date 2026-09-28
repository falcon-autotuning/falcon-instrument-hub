package handlers

import (
	"encoding/json"
	"testing"
	"time"

	falconports "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
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

func TestNewPortRequestHandler(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	cp := &ports.ConnectedPorts{}

	handler := NewPortRequestHandler(logger, cp)

	require.NotNil(t, handler)
	assert.Equal(t, logger, handler.logger)
	assert.Equal(t, cp, handler.ports)
	assert.Nil(t, handler.nc)
	assert.Nil(t, handler.subscription)
}

func TestPortRequestHandler_SubscribeUnsubscribe(t *testing.T) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	handler := NewPortRequestHandler(logger, &ports.ConnectedPorts{})

	require.NoError(t, handler.Subscribe(nc))

	assert.NotNil(t, handler.subscription)
	assert.Equal(t, nc, handler.nc)

	require.NoError(t, handler.Unsubscribe())

	assert.Nil(t, handler.subscription)
}

func TestPortRequestHandler_UnsubscribeWithoutSubscription(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	handler := NewPortRequestHandler(logger, &ports.ConnectedPorts{})

	require.NoError(t, handler.Unsubscribe())
}

func TestPortRequestHandler_InvalidJSON(t *testing.T) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	handler := NewPortRequestHandler(logger, &ports.ConnectedPorts{})

	require.NoError(t, handler.Subscribe(nc))
	defer handler.Unsubscribe()

	require.NoError(t, nc.Publish(PortRequestSubject, []byte("not json")))
	require.NoError(t, nc.Flush())

	time.Sleep(100 * time.Millisecond)
}

func TestPortRequestHandler_EmptyPortsE2E(t *testing.T) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	tempDir := t.TempDir()

	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	handler := NewPortRequestHandler(
		logger,
		&ports.ConnectedPorts{},
	)

	require.NoError(t, handler.Subscribe(nc))
	defer handler.Unsubscribe()

	responseCh := make(chan api.PortPayload, 1)

	sub, err := nc.Subscribe(PortPayloadSubject, func(msg *nats.Msg) {
		var payload api.PortPayload
		if err := json.Unmarshal(msg.Data, &payload); err == nil {
			responseCh <- payload
		}
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()

	request := api.PortRequest{
		Timestamp: time.Now().UnixMicro(),
	}

	data, err := json.Marshal(request)
	require.NoError(t, err)

	require.NoError(t, nc.Publish(PortRequestSubject, data))
	require.NoError(t, nc.Flush())

	select {
	case response := <-responseCh:
		assert.Equal(t, request.Timestamp, response.Timestamp)

		assert.NotEmpty(t, response.Knobs)
		assert.NotEmpty(t, response.Meters)

	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestSerializePortsToCerealJSON_Empty(t *testing.T) {
	out, err := serializePortsToCerealJSON(nil)

	require.NoError(t, err)
	assert.NotEmpty(t, out)

	handle, err := falconports.FromJSON(out)
	require.NoError(t, err)
	defer handle.Close()
}

func TestSerializePortsToCerealJSON_InvalidUnit(t *testing.T) {
	_, err := serializePortsToCerealJSON([]ports.ConnectedPort{
		{
			PortName:    "Source1.voltage",
			Role:        "output",
			Unit:        "not-a-unit",
			Description: "bad unit",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create instrument port")
}

func TestSerializePortsToCerealJSON_HappyPath(t *testing.T) {
	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer conn.Close()

	out, err := serializePortsToCerealJSON([]ports.ConnectedPort{
		{
			PortName:       "Mock.Source1.analog.voltage",
			Role:           "output",
			Unit:           "V",
			Description:    "Voltage source",
			InstrumentType: "dc_voltage_source",
			Handle:         conn,
		},
	})

	require.NoError(t, err)

	portsHandle, err := falconports.FromJSON(out)
	require.NoError(t, err)
	defer portsHandle.Close()
}
