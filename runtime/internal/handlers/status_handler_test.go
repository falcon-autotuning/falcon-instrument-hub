package handlers

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

func TestStatusHandler_PeriodicPublishing(t *testing.T) {
	// Create temporary directory for logs
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Start embedded NATS server
	opts := &server.Options{
		Port: -1,
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

	url := fmt.Sprintf("nats://%s", ns.Addr().String())
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("Failed to connect to test NATS server: %v", err)
	}
	defer nc.Close()

	// Track status messages
	statusMessages := make(chan *nats.Msg, 10)
	statusSub, err := nc.Subscribe(
		"STATUS.instrument-server",
		func(msg *nats.Msg) {
			t.Logf("Received status message: %s", string(msg.Data))
			statusMessages <- msg
		},
	)
	if err != nil {
		t.Fatalf("Failed to subscribe to STATUS.instrument-server: %v", err)
	}
	defer statusSub.Unsubscribe()

	// Create and start StatusHandler
	statusHandler := NewStatusHandler(logger)
	if err := statusHandler.Start(nc); err != nil {
		t.Fatalf("Failed to start StatusHandler: %v", err)
	}
	defer statusHandler.Stop()

	// Verify handler is running
	if !statusHandler.IsRunning() {
		t.Error("Expected status handler to be running after Start()")
	}

	// Wait for initial message and at least one periodic message
	// Should get immediate message + one after 4 seconds
	receivedMessages := []*nats.Msg{}
	timeout := time.After(6 * time.Second)

	for len(receivedMessages) < 2 {
		select {
		case msg := <-statusMessages:
			receivedMessages = append(receivedMessages, msg)
		case <-timeout:
			t.Fatalf(
				"Timeout waiting for status messages. Got %d messages",
				len(receivedMessages),
			)
		}
	}

	// Verify we got at least 2 messages
	if len(receivedMessages) < 2 {
		t.Fatalf(
			"Expected at least 2 status messages, got %d",
			len(receivedMessages),
		)
	}

	// Verify message content
	for i, msg := range receivedMessages {
		var status api.Status
		if err := json.Unmarshal(msg.Data, &status); err != nil {
			t.Fatalf("Message %d: Failed to unmarshal status: %v", i+1, err)
		}

		// Verify status is always true
		if !status.Status {
			t.Errorf("Message %d: Expected status to be true, got false", i+1)
		}

		// Verify timestamp is recent (within last 10 seconds)
		now := time.Now().UnixMicro()
		if status.Timestamp > now ||
			status.Timestamp < now-10000000 { // 10 seconds
			t.Errorf(
				"Message %d: Timestamp %d seems incorrect (now: %d)",
				i+1,
				status.Timestamp,
				now,
			)
		}

		t.Logf(
			"Message %d: Status=%t, Timestamp=%d",
			i+1,
			status.Status,
			status.Timestamp,
		)
	}

	// Verify timing between messages (should be ~4 seconds apart)
	if len(receivedMessages) >= 2 {
		var firstStatus, secondStatus api.Status
		json.Unmarshal(receivedMessages[0].Data, &firstStatus)
		json.Unmarshal(receivedMessages[1].Data, &secondStatus)

		timeDiff := secondStatus.Timestamp - firstStatus.Timestamp
		expectedDiff := int64(
			4 * time.Second / time.Microsecond,
		) // 4 seconds in microseconds

		// Allow some tolerance (±500ms)
		tolerance := int64(500 * time.Millisecond / time.Microsecond)
		if timeDiff < expectedDiff-tolerance ||
			timeDiff > expectedDiff+tolerance {
			t.Errorf(
				"Expected ~4 seconds between messages, got %d microseconds (%.2f seconds)",
				timeDiff,
				float64(timeDiff)/1000000.0,
			)
		} else {
			t.Logf("✓ Messages are ~4 seconds apart: %.2f seconds", float64(timeDiff)/1000000.0)
		}
	}

	t.Log("✓ Periodic publishing test passed")
}

func TestStatusHandler_StartStop(t *testing.T) {
	// Create temporary directory for logs
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Start embedded NATS server
	opts := &server.Options{
		Port: -1,
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

	url := fmt.Sprintf("nats://%s", ns.Addr().String())
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("Failed to connect to test NATS server: %v", err)
	}
	defer nc.Close()

	// Create StatusHandler
	statusHandler := NewStatusHandler(logger)

	// Verify initial state
	if statusHandler.IsRunning() {
		t.Error("Expected status handler to not be running initially")
	}

	// Test start
	if err := statusHandler.Start(nc); err != nil {
		t.Fatalf("Failed to start status handler: %v", err)
	}

	// Verify running state
	if !statusHandler.IsRunning() {
		t.Error("Expected status handler to be running after Start()")
	}

	// Test double start (should error)
	if err := statusHandler.Start(nc); err == nil {
		t.Error("Expected error when starting already running handler")
	}

	// Test stop
	if err := statusHandler.Stop(); err != nil {
		t.Fatalf("Failed to stop status handler: %v", err)
	}

	// Verify stopped state
	if statusHandler.IsRunning() {
		t.Error("Expected status handler to not be running after Stop()")
	}

	// Test double stop (should not error)
	if err := statusHandler.Stop(); err != nil {
		t.Errorf("Double stop should not error: %v", err)
	}

	t.Log("✓ Start/Stop lifecycle test passed")
}

func TestStatusHandler_StopBehavior(t *testing.T) {
	// Create temporary directory for logs
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Start embedded NATS server
	opts := &server.Options{
		Port: -1,
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

	url := fmt.Sprintf("nats://%s", ns.Addr().String())
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("Failed to connect to test NATS server: %v", err)
	}
	defer nc.Close()

	// Track status messages
	var messageCount int32
	statusSub, err := nc.Subscribe(
		"STATUS.instrument-server",
		func(msg *nats.Msg) {
			atomic.AddInt32(&messageCount, 1)
			current := atomic.LoadInt32(&messageCount)
			t.Logf("Received status message #%d", current)
		},
	)
	if err != nil {
		t.Fatalf("Failed to subscribe to STATUS.instrument-server: %v", err)
	}
	defer statusSub.Unsubscribe()

	// Create and start StatusHandler
	statusHandler := NewStatusHandler(logger)
	if err := statusHandler.Start(nc); err != nil {
		t.Fatalf("Failed to start StatusHandler: %v", err)
	}

	// Wait for initial message
	time.Sleep(500 * time.Millisecond)
	initialCount := atomic.LoadInt32(&messageCount)

	// Stop the handler
	if err := statusHandler.Stop(); err != nil {
		t.Fatalf("Failed to stop StatusHandler: %v", err)
	}

	// Wait longer than the publish interval to ensure no more messages
	time.Sleep(5 * time.Second)
	finalCount := atomic.LoadInt32(&messageCount)

	// Verify no new messages after stop
	if finalCount > initialCount {
		t.Errorf(
			"Expected no new messages after stop, but got %d new messages",
			finalCount-initialCount,
		)
	}

	// Verify context is cancelled
	ctx := statusHandler.GetContext()
	if ctx != nil {
		select {
		case <-ctx.Done():
			t.Log("✓ Context was properly cancelled")
		default:
			t.Error("Expected context to be cancelled after Stop()")
		}
	}

	t.Log("✓ Stop behavior test passed")
}

func TestStatusHandler_MessageFormat(t *testing.T) {
	// Create temporary directory for logs
	tempDir := t.TempDir()

	// Create logger
	logger, err := logging.NewLogger(tempDir)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Start embedded NATS server
	opts := &server.Options{
		Port: -1,
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

	url := fmt.Sprintf("nats://%s", ns.Addr().String())
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("Failed to connect to test NATS server: %v", err)
	}
	defer nc.Close()

	// Track first status message
	statusReceived := make(chan *nats.Msg, 1)
	statusSub, err := nc.Subscribe(
		"STATUS.instrument-server",
		func(msg *nats.Msg) {
			select {
			case statusReceived <- msg:
			default:
				// Ignore additional messages
			}
		},
	)
	if err != nil {
		t.Fatalf("Failed to subscribe to STATUS.instrument-server: %v", err)
	}
	defer statusSub.Unsubscribe()

	// Create and start StatusHandler
	statusHandler := NewStatusHandler(logger)
	if err := statusHandler.Start(nc); err != nil {
		t.Fatalf("Failed to start StatusHandler: %v", err)
	}
	defer statusHandler.Stop()

	// Wait for first message
	select {
	case msg := <-statusReceived:
		// Verify message is published to correct subject
		if msg.Subject != "STATUS.instrument-server" {
			t.Errorf(
				"Expected subject 'STATUS.instrument-server', got '%s'",
				msg.Subject,
			)
		}

		// Verify JSON structure
		var status api.Status
		if err := json.Unmarshal(msg.Data, &status); err != nil {
			t.Fatalf("Failed to unmarshal status JSON: %v", err)
		}

		// Verify required fields exist and have correct types
		if status.Status != true {
			t.Errorf("Expected status to be true, got %t", status.Status)
		}

		if status.Timestamp <= 0 {
			t.Errorf("Expected positive timestamp, got %d", status.Timestamp)
		}

		// Verify JSON contains expected fields
		var rawJson map[string]any
		if err := json.Unmarshal(msg.Data, &rawJson); err != nil {
			t.Fatalf("Failed to unmarshal raw JSON: %v", err)
		}

		expectedFields := []string{"status", "timestamp"}
		for _, field := range expectedFields {
			if _, exists := rawJson[field]; !exists {
				t.Errorf("Missing required field '%s' in JSON", field)
			}
		}

		// Verify no extra fields
		if len(rawJson) != len(expectedFields) {
			t.Errorf("Expected exactly %d fields in JSON, got %d: %v",
				len(expectedFields), len(rawJson), rawJson)
		}

		t.Logf("✓ Status message format correct: %s", string(msg.Data))

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for status message")
	}

	t.Log("✓ Message format test passed")
}
