package testutil

import (
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/stretchr/testify/require"
)

func TestLogger(t *testing.T) *logging.Logger {
	t.Helper()

	logger, err := logging.NewLogger(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() {
		logger.Close()
	})

	return logger
}
