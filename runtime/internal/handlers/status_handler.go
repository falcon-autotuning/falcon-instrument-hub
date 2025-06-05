package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

// StatusHandler handles periodic status publishing
type StatusHandler struct {
	logger    *logging.Logger
	mu        sync.RWMutex
	nc        *nats.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	isRunning bool
}

// NewStatusHandler creates a new status handler
func NewStatusHandler(logger *logging.Logger) *StatusHandler {
	return &StatusHandler{
		logger: logger,
	}
}

// Start begins publishing status messages every 4 seconds
func (h *StatusHandler) Start(nc *nats.Conn) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.isRunning {
		return fmt.Errorf("status handler is already running")
	}

	h.nc = nc
	h.ctx, h.cancel = context.WithCancel(context.Background())
	h.isRunning = true

	// Start the status publishing goroutine
	go h.publishStatusLoop()

	h.logger.Info(
		"STATUS_HANDLER",
		"Started publishing status messages every 4 seconds",
	)
	log.Printf(
		"STATUS handler started - publishing to STATUS.instument-server every 4 seconds",
	)

	return nil
}

// Stop stops the status publishing
func (h *StatusHandler) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.isRunning {
		return nil
	}

	h.cancel()
	h.isRunning = false
	h.nc = nil

	h.logger.Info("STATUS_HANDLER", "Stopped publishing status messages")
	log.Printf("STATUS handler stopped")

	return nil
}

// publishStatusLoop runs the periodic status publishing
func (h *StatusHandler) publishStatusLoop() {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	// Publish initial status immediately
	h.publishStatus()

	for {
		select {
		case <-ticker.C:
			h.publishStatus()
		case <-h.ctx.Done():
			h.logger.Debug("STATUS_HANDLER", "Status publishing loop stopped")
			return
		}
	}
}

// publishStatus publishes a single status message
func (h *StatusHandler) publishStatus() {
	h.mu.RLock()
	nc := h.nc
	h.mu.RUnlock()

	if nc == nil {
		h.logger.Error(
			"STATUS_HANDLER",
			"NATS connection is nil, cannot publish status",
		)
		return
	}

	// Create status message
	status := api.Status{
		Status:    true, // Always true when we're running
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal to JSON
	statusData, err := json.Marshal(status)
	if err != nil {
		h.logger.Error(
			"STATUS_HANDLER",
			fmt.Sprintf("Failed to marshal status message: %v", err),
		)
		return
	}

	// Publish to STATUS.instrument-server
	if err := nc.Publish("STATUS.instrument-server", statusData); err != nil {
		h.logger.Error(
			"STATUS_HANDLER",
			fmt.Sprintf("Failed to publish status message: %v", err),
		)
		return
	}

	// Log to both our logger and stdout
	h.logger.Info("STATUS_HANDLER", "Published status: true")
	log.Printf(
		"STATUS: Published status=true to STATUS.instrument-server at %d",
		status.Timestamp,
	)

	// Also print to stdout as requested
	fmt.Printf(
		"STATUS: instrument-server is running (timestamp: %d)\n",
		status.Timestamp,
	)
}

// IsRunning returns whether the status handler is currently running
func (h *StatusHandler) IsRunning() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.isRunning
}

// GetContext returns the current context (for testing)
func (h *StatusHandler) GetContext() context.Context {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ctx
}
