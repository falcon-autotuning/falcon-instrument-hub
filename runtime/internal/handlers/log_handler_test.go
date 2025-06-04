package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

func TestLogHandler_StructuredMessages(t *testing.T) {
	// Create temporary directory for logs
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	// Note: We'll close the logger manually at the end to control timing

	// Start embedded NATS server for testing
	opts := &server.Options{
		Port: -1, // Random available port
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create test NATS server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		t.Fatal("Test NATS server did not start in time")
	}
	defer ns.Shutdown()

	// Connect to the embedded server
	url := fmt.Sprintf("nats://%s", ns.Addr().String())
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("Failed to connect to test NATS server: %v", err)
	}
	defer nc.Close()

	// Flush connection to ensure it's ready
	if err := nc.Flush(); err != nil {
		t.Fatalf("Failed to flush NATS connection: %v", err)
	}
	// Create LOG handler
	logHandler := NewLogHandler(logger)

	// Subscribe to LOG.* channels
	if err := logHandler.Subscribe(nc); err != nil {
		t.Fatalf("Failed to subscribe to LOG channels: %v", err)
	}

	// Setup cleanup function that will be called at the end
	cleanup := func() {
		t.Log("Starting cleanup...")

		// 1. Unsubscribe first
		if err := logHandler.Unsubscribe(); err != nil {
			t.Logf("Error unsubscribing: %v", err)
		}

		// 2. Close NATS connection
		nc.Close()

		// 3. Shutdown NATS server with timeout
		done := make(chan struct{})
		go func() {
			ns.Shutdown()
			close(done)
		}()

		select {
		case <-done:
			t.Log("NATS server shutdown gracefully")
		case <-time.After(2 * time.Second):
			t.Log("NATS server shutdown timeout")
		}

		// 4. Close logger last
		if err := logger.Close(); err != nil {
			t.Logf("Error closing logger: %v", err)
		}

		t.Log("Cleanup complete")
	}

	// Wait for subscription to be active
	time.Sleep(100 * time.Millisecond)
	// Test structured messages with different scenarios
	testCases := []struct {
		name    string
		channel string
		log     api.Log
	}{
		{
			name:    "Full structured log with timestamp and hash",
			channel: "LOG.INFO",
			log: api.Log{
				Timestamp: int64(time.Now().Add(-1 * time.Hour).UnixMicro()),
				Hash:      123456,
				Message:   "This is a structured log message with hash",
			},
		},
		{
			name:    "Structured log with timestamp only",
			channel: "LOG.DEBUG",
			log: api.Log{
				Timestamp: int64(time.Now().Add(-30 * time.Minute).UnixMicro()),
				Message:   "Debug message with timestamp only",
			},
		},
		{
			name:    "Structured log with hash only",
			channel: "LOG.WARN",
			log: api.Log{
				Hash:    123,
				Message: "Warning message with hash only",
			},
		},
		{
			name:    "Minimal structured log",
			channel: "LOG.ERROR",
			log: api.Log{
				Message: "Error message with no timestamp or hash",
			},
		},
		{
			name:    "Custom channel with structured log",
			channel: "LOG.DEVICE.SENSOR1",
			log: api.Log{
				Timestamp: int64(time.Now().UnixMicro()),
				Hash:      123,
				Message:   "Sensor reading: temperature=25.5°C",
			},
		},
	}

	// Send structured test messages
	for i, tc := range testCases {
		t.Logf("Sending test case %d (%s) to channel %s: %s", i+1, tc.name, tc.channel, tc.log.Message)
		// Marshal to JSON
		jsonData, err := json.Marshal(tc.log)
		if err != nil {
			t.Fatalf("Failed to marshal log to JSON: %v", err)
		}
		t.Logf("JSON payload: %s", string(jsonData))

		// Publish to NATS
		if err := nc.Publish(tc.channel, jsonData); err != nil {
			t.Fatalf("Failed to publish to %s: %v", tc.channel, err)
		}

		// Small delay between messages to ensure ordering
		time.Sleep(10 * time.Millisecond)
	}

	// Flush to ensure all messages are sent
	if err := nc.Flush(); err != nil {
		t.Fatalf("Failed to flush messages: %v", err)
	}

	// Test plain text messages (backward compatibility)
	plainTextTests := []struct {
		channel string
		message string
	}{
		{"LOG.INFO", "Plain text info message"},
		{"LOG.ERROR", "Plain text error message"},
	}

	for i, test := range plainTextTests {
		t.Logf("Sending plain text %d to channel %s: %s", i+1, test.channel, test.message)
		if err := nc.Publish(test.channel, []byte(test.message)); err != nil {
			t.Errorf("Failed to publish plain text to %s: %v", test.channel, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Flush to ensure all messages are sent
	if err := nc.Flush(); err != nil {
		t.Fatalf("Failed to flush plain text messages: %v", err)
	}

	// Wait for messages to be processed with timeout
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	expectedMessageCount := len(testCases) + len(plainTextTests)
	t.Logf("Expecting %d messages total", expectedMessageCount)

	var logContent string
	for {
		select {
		case <-timeout:
			// Print debug info before failing
			if content, err := os.ReadFile(logger.GetLogPath()); err == nil {
				t.Logf("Final log content:\n%s", string(content))
			}
			cleanup() // Ensure cleanup happens even on timeout
			t.Fatalf(
				"Timeout waiting for messages to be processed. Expected %d messages",
				expectedMessageCount,
			)
		case <-ticker.C:
			// Check if log file exists and has content
			if _, err := os.Stat(logger.GetLogPath()); err == nil {
				content, err := os.ReadFile(logger.GetLogPath())
				if err == nil {
					logContent = string(content)
					// Count how many test messages we can find
					foundCount := 0
					t.Logf("Checking for messages in log content...")
					for i, tc := range testCases {
						if strings.Contains(logContent, tc.log.Message) {
							foundCount++
							t.Logf("✓ Found test case %d: %s", i+1, tc.log.Message)
						} else {
							t.Logf("✗ Missing test case %d: %s", i+1, tc.log.Message)
						}
					}
					for i, test := range plainTextTests {
						if strings.Contains(logContent, test.message) {
							foundCount++
							t.Logf("✓ Found plain text %d: %s", i+1, test.message)
						} else {
							t.Logf("✗ Missing plain text %d: %s", i+1, test.message)
						}
					}

					t.Logf("Found %d/%d messages", foundCount, expectedMessageCount)
					if foundCount >= expectedMessageCount {
						// All messages found, proceed to verification
						goto verification
					}
				}
			}
		}
	}

verification:
	// Call cleanup before verification to ensure all resources are properly closed
	cleanup()

	// Verify log file was created and contains messages
	logPath := logger.GetLogPath()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("Log file was not created: %s", logPath)
		return
	}

	// Read log file content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent = string(content)
	t.Logf("Log file created at: %s", logPath)
	t.Logf("Log content preview:\n%s", logContent)

	// Check that structured messages were logged correctly
	for _, tc := range testCases {
		if !strings.Contains(logContent, tc.log.Message) {
			t.Errorf("Log file does not contain message: %s", tc.log.Message)
		}
		if !strings.Contains(logContent, tc.channel) {
			t.Errorf("Log file does not contain channel: %s", tc.channel)
		}

		// Check hash if present
		if tc.log.Hash != 0 {
			expectedHash := fmt.Sprintf("[HASH:%s]", strconv.FormatInt(tc.log.Hash, 10))
			if !strings.Contains(logContent, expectedHash) {
				t.Errorf("Log file does not contain hash: %s", expectedHash)
			}
		}
	}

	// Check plain text messages
	for _, test := range plainTextTests {
		if !strings.Contains(logContent, test.message) {
			t.Errorf("Log file does not contain plain text message: %s", test.message)
		}
	}
}

func TestLogTimestampConversion(t *testing.T) {
	// test timestamp conversion
	now := time.Now()
	microseconds := int64(now.UnixMicro())

	log := api.Log{
		Timestamp: microseconds,
		Message:   "test message",
	}

	convertedtime := api.ToTime(log)

	// should be within a few milliseconds
	diff := convertedtime.Sub(now)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("timestamp conversion error: expected ~%v, got %v (diff: %v)",
			now, convertedtime, diff)
	}

	// test zero timestamp
	lognotimestamp := api.Log{
		Message: "test message without timestamp",
	}

	currenttime := api.ToTime(lognotimestamp)
	if time.Since(currenttime) > time.Second {
		t.Error("totime() should return current time when timestamp is zero")
	}
}
