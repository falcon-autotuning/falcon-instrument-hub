package logging

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type diagnosticBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *diagnosticBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func TestLoggerDiagnosticsAreOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			output := &diagnosticBuffer{}
			logger, err := NewLoggerWithOptions(t.TempDir(), LoggerOptions{
				Diagnostics: enabled, DiagnosticWriter: output,
			})
			require.NoError(t, err)
			t.Cleanup(func() { logger.Close() })
			logger.Info("TEST", "written to file regardless of diagnostics")
			logger.WriteRaw("raw test entry")
			if enabled {
				require.Eventually(t, func() bool {
					return strings.Contains(output.String(), "[LOGGER_DEBUG] Tick #")
				}, 2*time.Second, 10*time.Millisecond)
			}
			require.NoError(t, logger.Close())
			data, err := os.ReadFile(logger.GetLogPath())
			require.NoError(t, err)
			assert.Contains(t, string(data), "written to file regardless of diagnostics")
			assert.Contains(t, string(data), "raw test entry\n")
			assert.NotContains(t, string(data), "[LOGGER_DEBUG]")
			if enabled {
				assert.Contains(t, output.String(), "AsyncWriter started")
				assert.Contains(t, output.String(), "AsyncWriter exiting normally")
			} else {
				assert.Empty(t, output.String())
			}
		})
	}
}

func TestNewLoggerDefaultsToNoDiagnostics(t *testing.T) {
	logger, err := NewLogger(t.TempDir())
	require.NoError(t, err)
	assert.Nil(t, logger.diagnosticWriter)
	require.NoError(t, logger.Close())
}

func TestEnabledDiagnosticsDefaultToStderr(t *testing.T) {
	logger, err := NewLoggerWithOptions(t.TempDir(), LoggerOptions{Diagnostics: true})
	require.NoError(t, err)
	assert.Equal(t, os.Stderr, logger.diagnosticWriter)
	require.NoError(t, logger.Close())
}
