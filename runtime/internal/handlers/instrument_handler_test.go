package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestConfig creates a basic config for testing
func setupTestConfig() *config.Config {
	return &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}
}

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
	handler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		setupTestConfig(),
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

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

		// Verify the process is actually running
		activeInstruments := handler.GetActiveInstruments()
		require.Contains(t, activeInstruments, "test-instrument")
		assert.Len(t, activeInstruments, 1)
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
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	tempDir := t.TempDir()

	// Change to temp directory
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(oldDir)
	os.Chdir(tempDir)

	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	handler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		setupTestConfig(),
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	t.Run("script creation", func(t *testing.T) {
		// This should create the scripts directory and extract the embedded
		// script - we'll test this indirectly by checking if the script file
		// exists
		scriptPath := filepath.Join("./", "launch_instrument_daemon.py")

		// First ensure the handler can start (which calls ensureScriptExists
		// internally)
		err := handler.Subscribe(nc)
		require.NoError(t, err)
		defer handler.Unsubscribe()

		// Verify script exists
		_, err = os.Stat(scriptPath)
		assert.NoError(t, err, "Script file should exist")

		// Verify script is executable
		info, err := os.Stat(scriptPath)
		require.NoError(t, err)
		assert.NotEqual(t, 0, info.Mode()&0111, "Script should be executable")
	})
}

func TestInstrumentHandlerCleanup(t *testing.T) {
	server := runNATSServer(t)
	defer server.Shutdown()
	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	tempDir := t.TempDir()

	// Change to temp directory
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(oldDir)
	os.Chdir(tempDir)

	// Create mock script
	scriptsDir := "./"
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

	handler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		setupTestConfig(),
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	// Subscribe to enable the handler
	err = handler.Subscribe(nc)
	require.NoError(t, err)

	// Start a test instrument using the public API
	request := api.SetupInstrument{Name: "cleanup-test"}
	requestData, err := json.Marshal(request)
	require.NoError(t, err)

	err = nc.Publish("SETUP_INSTRUMENT.external.test", requestData)
	require.NoError(t, err)

	// Wait for instrument to start
	time.Sleep(300 * time.Millisecond)

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

func TestInstrumentHandlerInitialization(t *testing.T) {
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
import json
import asyncio
import signal
from nats.aio.client import Client as NATS

def signal_handler(sig, frame):
    print(f"Received signal {sig}, exiting gracefully")
    sys.exit(0)

signal.signal(signal.SIGTERM, signal_handler)
signal.signal(signal.SIGINT, signal_handler)

async def main():
    if len(sys.argv) < 3:
        print("Usage: script.py <instrument_name> <nats_url>")
        sys.exit(1)
    
    instrument_name = sys.argv[1]
    nats_url = sys.argv[2]
    
    print(f"Mock instrument {instrument_name} started with NATS URL: {nats_url}")
    
    try:
        nc = NATS()
        await nc.connect(nats_url)
        
        # Send initialization confirmation after a brief delay
        await asyncio.sleep(0.1)
        
        # Mock initialization data matching the Python daemon structure
        init_data = {
            "init": {
                "property1": {"0": {"bounds": [0, 100], "unit": "V"}},
                "property2": {"1": {"bounds": [-10, 10], "unit": "A"}}
            },
            "port": {
                "property1": {"0": f"{instrument_name}_property1_0"},
                "property2": {"1": f"{instrument_name}_property2_1"}
            },
            "timestamp": int(time.time() * 1000)
        }
        
        subject = f"CONFIRM_INITIALIZATION.{instrument_name}"
        await nc.publish(subject, json.dumps(init_data).encode())
        print(f"Sent initialization data for {instrument_name}")
        
        # Keep running briefly then exit
        await asyncio.sleep(0.2)
        
    except Exception as e:
        print(f"Error: {e}")
    finally:
        if 'nc' in locals():
            await nc.close()

if __name__ == "__main__":
    asyncio.run(main())
`

	scriptPath := filepath.Join(scriptsDir, "launch_instrument_daemon.py")
	err = os.WriteFile(scriptPath, []byte(mockScript), 0755)
	require.NoError(t, err)

	// Create handler
	handler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		setupTestConfig(),
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	// Subscribe to instrument commands
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	t.Run("initialization_confirmation_handling", func(t *testing.T) {
		// Start an instrument
		request := api.SetupInstrument{
			Name: "test-init-instrument",
		}
		requestData, err := json.Marshal(request)
		require.NoError(t, err)

		err = nc.Publish("SETUP_INSTRUMENT.external.test", requestData)
		require.NoError(t, err)

		// Wait for instrument to start and send initialization
		time.Sleep(800 * time.Millisecond)

		// Verify the instrument is listed as active
		activeInstruments := handler.GetActiveInstruments()
		require.Contains(t, activeInstruments, "test-init-instrument")

		// The mock script should have sent initialization data
		// We can't directly access the internal state, but we can verify
		// the handler received and processed the initialization by checking
		// that the instrument remains stable (no errors in logs)

		// Wait a bit more to ensure the process completes
		time.Sleep(200 * time.Millisecond)

		// Verify instrument is still active (initialization was successful)
		activeInstruments = handler.GetActiveInstruments()
		assert.Contains(t, activeInstruments, "test-init-instrument")
	})

	t.Run("invalid_initialization_subject", func(t *testing.T) {
		// Send initialization with invalid subject format
		initDataMap := map[string]interface{}{
			"test": "data",
		}
		portDataMap := map[string]interface{}{
			"test": "port",
		}

		initJSON, _ := json.Marshal(initDataMap)
		portJSON, _ := json.Marshal(portDataMap)

		invalidInitData := api.ConfirmInitialization{
			Init:      string(initJSON),
			Port:      string(portJSON),
			Timestamp: time.Now().UnixMicro(),
		}

		data, err := json.Marshal(invalidInitData)
		require.NoError(t, err)

		// Send with invalid subject (missing instrument name)
		err = nc.Publish("CONFIRM_INITIALIZATION", data)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Should be handled gracefully (no crash)
	})

	t.Run("initialization_for_unknown_instrument", func(t *testing.T) {
		// Send initialization for an instrument that wasn't started by the
		// handler
		unknownInitDataMap := map[string]interface{}{
			"unknown": "data",
		}
		unknownPortDataMap := map[string]interface{}{
			"unknown": "port",
		}

		unknownInitJSON, _ := json.Marshal(unknownInitDataMap)
		unknownPortJSON, _ := json.Marshal(unknownPortDataMap)

		unknownInitData := api.ConfirmInitialization{
			Init:      string(unknownInitJSON),
			Port:      string(unknownPortJSON),
			Timestamp: time.Now().UnixMicro(),
		}

		data, err := json.Marshal(unknownInitData)
		require.NoError(t, err)

		// Send for non-existent instrument
		err = nc.Publish("CONFIRM_INITIALIZATION.unknown-instrument", data)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Should be handled gracefully (no crash)
	})

	t.Run("malformed_initialization_data", func(t *testing.T) {
		// Send malformed JSON
		err = nc.Publish(
			"CONFIRM_INITIALIZATION.test-instrument",
			[]byte("invalid json"),
		)
		require.NoError(t, err)

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Should be handled gracefully (no crash)
	})
}

func TestInstrumentHandlerUpdateDaemonProperty(t *testing.T) {
	// Start NATS server for testing
	server := runNATSServer(t)
	defer server.Shutdown()

	// Connect to NATS
	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	// Create temporary directory for test files
	tempDir := t.TempDir()

	// Change to temp directory
	oldDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(oldDir)
	os.Chdir(tempDir)

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Create handler
	handler, err := instrument.NewHandler(
		logger,
		server.ClientURL(),
		nc,
		setupTestConfig(),
		"python3", // Use system python for tests
	)
	require.NoError(t, err)

	// Subscribe to instrument commands
	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create a subscription to capture SET commands that would be sent to
	// instruments
	var receivedSetCommand api.Set
	var setCommandReceived bool
	var mu sync.Mutex
	setSubject := "SET.*"

	setSub, err := nc.Subscribe(setSubject, func(msg *nats.Msg) {
		var setCmd api.Set
		if err := json.Unmarshal(msg.Data, &setCmd); err == nil {
			mu.Lock()
			receivedSetCommand = setCmd
			setCommandReceived = true
			mu.Unlock()
		}
	})
	require.NoError(t, err)
	defer setSub.Unsubscribe()

	t.Run("successful_update_daemon_property", func(t *testing.T) {
		// First, we need to simulate an instrument that has been initialized
		// with port configuration
		instrumentName := "test-multimeter"

		// Create a mock script that will send initialization data
		scriptsDir := "scripts"
		err = os.MkdirAll(scriptsDir, 0755)
		require.NoError(t, err)

		mockScript := `#!/usr/bin/env python3
import sys
import time
import json
import signal

def signal_handler(sig, frame):
    sys.exit(0)

signal.signal(signal.SIGTERM, signal_handler)
signal.signal(signal.SIGINT, signal_handler)

if __name__ == "__main__":
    if len(sys.argv) < 3:
        sys.exit(1)
    
    instrument_name = sys.argv[1]
    print(f"Mock instrument {instrument_name} started")
    
    # Simulate instrument daemon running
    try:
        while True:
            time.sleep(0.1)
    except KeyboardInterrupt:
        sys.exit(0)
`

		scriptPath := filepath.Join(scriptsDir, "launch_instrument_daemon.py")
		err = os.WriteFile(scriptPath, []byte(mockScript), 0755)
		require.NoError(t, err)

		// Start the instrument using the public API (this creates the
		// InstrumentProcess)
		setupRequest := api.SetupInstrument{Name: instrumentName}
		setupData, err := json.Marshal(setupRequest)
		require.NoError(t, err)

		err = nc.Publish("SETUP_INSTRUMENT.external.test-multimeter", setupData)
		require.NoError(t, err)

		// Wait for instrument to start
		time.Sleep(300 * time.Millisecond)

		// Verify the instrument is running
		activeInstruments := handler.GetActiveInstruments()
		require.Contains(t, activeInstruments, instrumentName)

		// Now simulate initialization data for the running instrument
		initDataMap := map[string]any{
			"voltage": map[int64]any{
				0: map[string]any{
					"bounds": []float64{0, 100},
					"unit":   "V",
				},
			},
		}

		portDataMap := map[string]any{
			"voltage": map[int64]any{
				0: "multimeter_voltage_ch1",
			},
		}

		initJSON, _ := json.Marshal(initDataMap)
		portJSON, _ := json.Marshal(portDataMap)

		initResponse := api.ConfirmInitialization{
			Init:      string(initJSON),
			Port:      string(portJSON),
			Timestamp: time.Now().UnixMicro(),
		}

		initData, err := json.Marshal(initResponse)
		require.NoError(t, err)

		// Send initialization for the running instrument
		err = nc.Publish("CONFIRM_INITIALIZATION."+instrumentName, initData)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond) // Wait for processing

		// Reset flags
		mu.Lock()
		setCommandReceived = false
		mu.Unlock()

		// Create UPDATE_DAEMON_PROPERTY request
		updateRequest := api.UpdateDaemonProperty{
			Name:      "multimeter_voltage_ch1", // This should match the port name
			Property:  "voltage",                // This should match the property key
			Value:     5.5,                      // New voltage value
			Timestamp: time.Now().Unix(),
		}

		updateData, err := json.Marshal(updateRequest)
		require.NoError(t, err)

		// Now send the UPDATE_DAEMON_PROPERTY command
		err = nc.Publish("UPDATE_DAEMON_PROPERTY.instrument-server", updateData)
		require.NoError(t, err)

		// Wait for the command to be processed and SET command to be sent
		time.Sleep(200 * time.Millisecond)

		// Verify SET command was sent correctly
		mu.Lock()
		received := setCommandReceived
		cmd := receivedSetCommand
		mu.Unlock()

		assert.True(t, received, "SET command should have been sent")
		if received {
			assert.Equal(t, "voltage", cmd.Property)
			assert.Equal(t, int64(0), cmd.Index)
			assert.Equal(t, 5.5, cmd.Value)
		}
	})

	t.Run("missing_property_field", func(t *testing.T) {
		// Test with missing property field
		updateRequest := api.UpdateDaemonProperty{
			Name:      "test_device",
			Property:  "", // Missing property
			Value:     123,
			Timestamp: time.Now().Unix(),
		}

		updateData, err := json.Marshal(updateRequest)
		require.NoError(t, err)

		err = nc.Publish("UPDATE_DAEMON_PROPERTY.instrument-server", updateData)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		// Should be handled gracefully with error log
	})

	t.Run("missing_name_field", func(t *testing.T) {
		// Test with missing name field
		updateRequest := api.UpdateDaemonProperty{
			Name:      "", // Missing name
			Property:  "voltage",
			Value:     123,
			Timestamp: time.Now().Unix(),
		}

		updateData, err := json.Marshal(updateRequest)
		require.NoError(t, err)

		err = nc.Publish("UPDATE_DAEMON_PROPERTY.instrument-server", updateData)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		// Should be handled gracefully with error log
	})

	t.Run("instrument_not_found", func(t *testing.T) {
		// Test with a name that doesn't match any instrument port
		updateRequest := api.UpdateDaemonProperty{
			Name:      "nonexistent_device",
			Property:  "voltage",
			Value:     123,
			Timestamp: time.Now().Unix(),
		}

		updateData, err := json.Marshal(updateRequest)
		require.NoError(t, err)

		err = nc.Publish("UPDATE_DAEMON_PROPERTY.instrument-server", updateData)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		// Should be handled gracefully with error log
	})

	t.Run("malformed_json", func(t *testing.T) {
		// Test with invalid JSON
		err = nc.Publish(
			"UPDATE_DAEMON_PROPERTY.instrument-server",
			[]byte("invalid json"),
		)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		// Should be handled gracefully with error log
	})
}
