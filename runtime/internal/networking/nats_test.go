package networking

import (
	"net"
	"strconv"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNATSManager_Defaults(
	t *testing.T,
) {
	conf := &config.NatsConfig{}

	manager, err := NewNATSManager(conf)

	require.NoError(t, err)
	require.NotNil(t, manager)

	t.Cleanup(manager.Close)

	assert.NotEmpty(t, manager.GetNATSURL())
	assert.NotNil(t, manager.GetConnection())
}

func TestNewNATSManager_CustomHostPort(
	t *testing.T,
) {
	port := findAvailablePort()

	conf := &config.NatsConfig{
		Host: "127.0.0.1",
		Port: port,
	}

	manager, err := NewNATSManager(conf)

	require.NoError(t, err)
	require.NotNil(t, manager)

	t.Cleanup(manager.Close)

	assert.Equal(
		t,
		"nats://127.0.0.1:"+strconv.Itoa(port),
		manager.GetNATSURL(),
	)
}

func TestGetConnection(
	t *testing.T,
) {
	manager, err := NewNATSManager(
		&config.NatsConfig{},
	)

	require.NoError(t, err)

	t.Cleanup(manager.Close)

	assert.NotNil(
		t,
		manager.GetConnection(),
	)
}

func TestClose(
	t *testing.T,
) {
	manager, err := NewNATSManager(
		&config.NatsConfig{},
	)

	require.NoError(t, err)

	manager.Close()

	assert.Nil(
		t,
		manager.conn,
	)

	assert.Nil(
		t,
		manager.server,
	)
}

func TestClose_NilFields(
	t *testing.T,
) {
	manager := &NATSManager{}

	assert.NotPanics(
		t,
		func() {
			manager.Close()
		},
	)
}

func TestStartEmbeddedNATS(
	t *testing.T,
) {
	port := findAvailablePort()

	server,
		conn,
		url,
		err := startEmbeddedNATS(
		"127.0.0.1",
		port,
	)

	require.NoError(t, err)

	require.NotNil(t, server)
	require.NotNil(t, conn)

	assert.NotEmpty(t, url)

	conn.Close()
	server.Shutdown()
}

func TestFindAvailablePort(
	t *testing.T,
) {
	port := findAvailablePort()

	assert.Positive(
		t,
		port,
	)

	assert.True(
		t,
		isPortAvailable(port),
	)
}

func TestIsPortAvailable_Available(
	t *testing.T,
) {
	port := findAvailablePort()

	assert.True(
		t,
		isPortAvailable(port),
	)
}

func TestIsPortAvailable_Unavailable(
	t *testing.T,
) {
	ln, err := net.Listen(
		"tcp",
		"127.0.0.1:0",
	)

	require.NoError(t, err)
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	assert.False(
		t,
		isPortAvailable(port),
	)
}
