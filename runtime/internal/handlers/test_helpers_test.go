package handlers

import (
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// setupTestNATSServer starts an embedded NATS server and returns a connected
// client. Both the server and the connection are cleaned up via t.Cleanup.
func setupTestNATSServer(t *testing.T) *nats.Conn {
	t.Helper()
	srv := runNATSServer(t)
	t.Cleanup(func() { srv.Shutdown() })

	nc, err := nats.Connect(srv.ClientURL())
	require.NoError(t, err)
	t.Cleanup(func() { nc.Close() })
	return nc
}
