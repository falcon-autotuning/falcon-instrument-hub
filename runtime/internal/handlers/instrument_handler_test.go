package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstrumentHandler(t *testing.T) {
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create temporary directory for test files
	tempDir := t.TempDir()

	// Change to temp directory for script creation
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(oldDir)
	os.Chdir(tempDir)

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Create mock Python script for testing
	scriptsDir := filepath.Join(tempDir, "scripts")
	err = os.MkdirAll(scriptsDir, 0755)
	require.NoError(t, err)

	mockScript := `#!/usr/bin/env python3
import sys
import time
import signal

def signal_handler(sig, frame):
    print(f"Received signal {sig}, exiting gracefully")
    sys.exit(0)

signal.signal(signal.SIGTERM, signal_handler)
signal.signal(signal.SIGINT, signal_handler)

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: script.py <instrument_name> <nats_url>")
        sys.exit(1)
    
    instrument_name = sys.argv[1]
    nats_url = sys.argv[2]
    
    print(f"Mock instrument {instrument_name} started with NATS URL: {nats_url}")
    
    # Simulate instrument daemon running
    try:
        while True:
            time.sleep(0.1)
    except KeyboardInterrupt:
        print("Interrupted, exiting")
        sys.exit(0)
`

	scriptPath := filepath.Join(scriptsDir, "launch_instrument_daemon.py")
	err = os.WriteFile(scriptPath, []byte(mockScript), 0755)
	require.NoError(t, err)

	// Create handler
	handler := NewInstrumentHandler(logger, server.ClientURL())

	// Subscribe to instrument commands
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	t.Run("successful instrument setup", func(t *testing.T) {
		// Create setup request
		request := api.SetupInstrument{
			Name: "test-instrument",
		}
		requestData, err := json.Marshal(request)
		require.NoError(t, err)

		// Send setup request
		err = nc.Publish("SETUP_INSTRUMENT.external.test", requestData)
		require.NoError(t, err)

		// Wait for instrument to start
		time.Sleep(500 * time.Millisecond)

		// Verify instrument is running
		activeInstruments := handler.GetActiveInstruments()
		assert.Contains(t, activeInstruments, "test-instrument")
		assert.Len(t, activeInstruments, 1)

		// Verify the process is actually running
		handler.mutex.RLock()
		process, exists := handler.instruments["test-instrument"]
		handler.mutex.RUnlock()
		require.True(t, exists)
		assert.NotNil(t, process.Process)
		assert.NotNil(t, process.Cmd)
		assert.NotNil(t, process.Cancel)
	})

	t.Run("destroy existing instrument", func(t *testing.T) {
		// Ensure we have an instrument to destroy
		activeInstruments := handler.GetActiveInstruments()
		require.Contains(t, activeInstruments, "test-instrument")

		// Create destroy request
		request := api.DestroyInstrument{
			Name: "test-instrument",
		}
		requestData, err := json.Marshal(request)
		require.NoError(t, err)

		// Send destroy request
		err = nc.Publish("DESTROY_INSTRUMENT.external.test", requestData)
		require.NoError(t, err)

		// Wait for instrument to stop
		time.Sleep(500 * time.Millisecond)

		// Verify instrument is no longer running
		activeInstruments = handler.GetActiveInstruments()
		assert.NotContains(t, activeInstruments, "test-instrument")
		assert.Len(t, activeInstruments, 0)
	})

	t.Run("setup duplicate instrument", func(t *testing.T) {
		// First setup
		request := api.SetupInstrument{
			Name: "duplicate-test",
		}
		requestData, err := json.Marshal(request)
		require.NoError(t, err)

		err = nc.Publish("SETUP_INSTRUMENT.external.test", requestData)
		require.NoError(t, err)
		time.Sleep(300 * time.Millisecond)

		// Verify first instrument is running
		activeInstruments := handler.GetActiveInstruments()
		assert.Contains(t, activeInstruments, "duplicate-test")

		// Try to setup same instrument again
		err = nc.Publish("SETUP_INSTRUMENT.external.test", requestData)
		require.NoError(t, err)
		time.Sleep(300 * time.Millisecond)

		// Should still only have one instance
		activeInstruments = handler.GetActiveInstruments()
		count := 0
		for _, name := range activeInstruments {
			if name == "duplicate-test" {
				count++
			}
		}
		assert.Equal(
			t,
			1,
			count,
			"Should only have one instance of duplicate-test",
		)

		// Cleanup
		destroyRequest := api.DestroyInstrument{Name: "duplicate-test"}
		destroyData, _ := json.Marshal(destroyRequest)
		nc.Publish("DESTROY_INSTRUMENT.external.test", destroyData)
		time.Sleep(300 * time.Millisecond)
	})

	t.Run("destroy non-existent instrument", func(t *testing.T) {
		// Try to destroy an instrument that doesn't exist
		request := api.DestroyInstrument{
			Name: "non-existent",
		}
		requestData, err := json.Marshal(request)
		require.NoError(t, err)

		// This should not cause any errors, just log a message
		err = nc.Publish("DESTROY_INSTRUMENT.external.test", requestData)
		require.NoError(t, err)

		// Brief wait to process message
		time.Sleep(100 * time.Millisecond)
		// No assertions needed - just ensuring no panic occurs
	})

	t.Run("invalid JSON in setup request", func(t *testing.T) {
		// Send invalid JSON
		err = nc.Publish(
			"SETUP_INSTRUMENT.external.test",
			[]byte("invalid json"),
		)
		require.NoError(t, err)

		// Brief wait to process message
		time.Sleep(100 * time.Millisecond)
		// Should not crash - error should be logged
	})

	t.Run("invalid JSON in destroy request", func(t *testing.T) {
		// Send invalid JSON
		err = nc.Publish(
			"DESTROY_INSTRUMENT.external.test",
			[]byte("invalid json"),
		)
		require.NoError(t, err)

		// Brief wait to process message
		time.Sleep(100 * time.Millisecond)
		// Should not crash - error should be logged
	})

	t.Run("empty instrument name in setup", func(t *testing.T) {
		request := api.SetupInstrument{
			Name: "", // Empty name
		}
		requestData, err := json.Marshal(request)
		require.NoError(t, err)

		err = nc.Publish("SETUP_INSTRUMENT.external.test", requestData)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		// Should be handled gracefully with error log
	})

	t.Run("empty instrument name in destroy", func(t *testing.T) {
		request := api.DestroyInstrument{
			Name: "", // Empty name
		}
		requestData, err := json.Marshal(request)
		require.NoError(t, err)

		err = nc.Publish("DESTROY_INSTRUMENT.external.test", requestData)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		// Should be handled gracefully with error log
	})
}

func TestInstrumentHandlerScriptEnsure(t *testing.T) {
	tempDir := t.TempDir()

	// Change to temp directory
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(oldDir)
	os.Chdir(tempDir)

	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	handler := NewInstrumentHandler(logger, "nats://localhost:4222")

	t.Run("script creation", func(t *testing.T) {
		// This should create the scripts directory and extract the embedded script
		err := handler.ensureScriptExists()
		require.NoError(t, err)

		// Verify script exists
		scriptPath := filepath.Join("scripts", "launch_instrument_daemon.py")
		_, err = os.Stat(scriptPath)
		assert.NoError(t, err, "Script file should exist")

		// Verify script is executable
		info, err := os.Stat(scriptPath)
		require.NoError(t, err)
		assert.NotEqual(t, 0, info.Mode()&0111, "Script should be executable")
	})
}

func TestInstrumentHandlerCleanup(t *testing.T) {
	tempDir := t.TempDir()

	// Change to temp directory
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(oldDir)
	os.Chdir(tempDir)

	// Create mock script
	scriptsDir := "scripts"
	err = os.MkdirAll(scriptsDir, 0755)
	require.NoError(t, err)

	mockScript := `#!/usr/bin/env python3
import time
import signal
import sys

def signal_handler(sig, frame):
    sys.exit(0)

signal.signal(signal.SIGTERM, signal_handler)

while True:
    time.sleep(0.1)
`

	scriptPath := filepath.Join(scriptsDir, "launch_instrument_daemon.py")
	err = os.WriteFile(scriptPath, []byte(mockScript), 0755)
	require.NoError(t, err)

	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	handler := NewInstrumentHandler(logger, "nats://localhost:4222")

	// Start a test instrument manually
	err = handler.startInstrument("cleanup-test")
	require.NoError(t, err)

	// Verify it's running
	activeInstruments := handler.GetActiveInstruments()
	assert.Contains(t, activeInstruments, "cleanup-test")

	// Test cleanup via Unsubscribe
	err = handler.Unsubscribe()
	require.NoError(t, err)

	// Brief wait for cleanup to complete
	time.Sleep(200 * time.Millisecond)

	// Verify all instruments are stopped
	activeInstruments = handler.GetActiveInstruments()
	assert.Len(t, activeInstruments, 0)
}

func TestGetActiveInstruments(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	handler := NewInstrumentHandler(logger, "nats://localhost:4222")

	// Initially should be empty
	activeInstruments := handler.GetActiveInstruments()
	assert.Len(t, activeInstruments, 0)

	// Add some mock instruments directly to test the getter
	handler.mutex.Lock()
	handler.instruments["test1"] = &InstrumentProcess{Name: "test1"}
	handler.instruments["test2"] = &InstrumentProcess{Name: "test2"}
	handler.mutex.Unlock()

	activeInstruments = handler.GetActiveInstruments()
	assert.Len(t, activeInstruments, 2)
	assert.Contains(t, activeInstruments, "test1")
	assert.Contains(t, activeInstruments, "test2")
}
