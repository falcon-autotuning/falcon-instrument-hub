package handlers

import (
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerOperationsExcludeRetiredHandlers(t *testing.T) {
	manager := &Manager{}
	for _, includeStatus := range []bool{false, true} {
		var names []string
		for _, op := range manager.getHandlerOperations(includeStatus) {
			names = append(names, op.name)
		}
		expected := []string{"log handler", "device config handler", "measure command handler", "port request handler"}
		if includeStatus {
			expected = append(expected, "status handler")
		}
		assert.Equal(t, expected, names)
	}
}

func TestInvalidInstrumentAPIPreventsHandlerStartup(t *testing.T) {
	logger, err := logging.NewLogger(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { logger.Close() })
	cfg := &config.Config{InstrumentAPIPaths: []string{filepath.Join(t.TempDir(), "missing-api.yml")}}
	manager := NewManager(cfg, logger, nil, nil)
	require.Error(t, manager.instrumentError)
	assert.NoError(t, manager.metadataError, "metadata loading must not overwrite the instrument failure")
	for _, start := range []func() error{manager.Start, manager.StartCoreHandlers} {
		err := start()
		require.Error(t, err)
		assert.ErrorContains(t, err, "invalid instrument configuration")
		assert.ErrorIs(t, err, manager.instrumentError)
	}
}
