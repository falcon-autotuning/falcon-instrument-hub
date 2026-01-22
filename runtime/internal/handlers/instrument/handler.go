package instrument

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"

	"github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/client"
	"github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/config"
)

// Handler manages instruments through the instrument-script-server
type Handler struct {
	config        *config.Config
	client        *client.InstrumentServerClient
	daemonCmd     *exec.Cmd
	daemonStarted bool
	mu            sync.RWMutex
}

// NewHandler creates a new instrument handler
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		config:        cfg,
		client:        client.NewInstrumentServerClient(cfg.GetRPCBaseURL()),
		daemonStarted: false,
	}
}

// StartDaemon starts the instrument-script-server daemon
// This should be called at the startup of the instrument-hub
func (h *Handler) StartDaemon(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.daemonStarted {
		return fmt.Errorf("daemon already started")
	}

	// Start the instrument-script-server daemon
	cmd := exec.CommandContext(ctx, h.config.InstrumentServerBinary, "daemon", "start")

	// Set up output capture for debugging
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start instrument-script-server daemon: %w", err)
	}

	h.daemonCmd = cmd
	h.daemonStarted = true

	log.Printf("Instrument-script-server daemon started with PID %d", cmd.Process.Pid)

	return nil
}

// StopDaemon stops the instrument-script-server daemon
func (h *Handler) StopDaemon(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.daemonStarted {
		return fmt.Errorf("daemon not started")
	}

	// Stop the daemon using the CLI command
	cmd := exec.CommandContext(ctx, h.config.InstrumentServerBinary, "daemon", "stop")
	if err := cmd.Run(); err != nil {
		log.Printf("Warning: failed to stop daemon gracefully: %v", err)
	}

	// Wait for the process to finish
	if h.daemonCmd != nil {
		if err := h.daemonCmd.Wait(); err != nil {
			log.Printf("Daemon process exited with error: %v", err)
		}
	}

	h.daemonStarted = false
	h.daemonCmd = nil

	log.Println("Instrument-script-server daemon stopped")

	return nil
}

// StartInstrument starts an instrument with the given configuration file
//
// NOTE: This API endpoint will be refactored in future versions to accept
// instrument configuration directly rather than a file path.
func (h *Handler) StartInstrument(ctx context.Context, configFile string) error {
	h.mu.RLock()
	if !h.daemonStarted {
		h.mu.RUnlock()
		return fmt.Errorf("daemon not started, call StartDaemon first")
	}
	h.mu.RUnlock()

	if err := h.client.StartInstrument(ctx, configFile); err != nil {
		return fmt.Errorf("failed to start instrument: %w", err)
	}

	log.Printf("Instrument started from config: %s", configFile)
	return nil
}

// StopInstrument stops an instrument by name
//
// NOTE: This API endpoint will be refactored in future versions
func (h *Handler) StopInstrument(ctx context.Context, instrumentName string) error {
	if err := h.client.StopInstrument(ctx, instrumentName); err != nil {
		return fmt.Errorf("failed to stop instrument %s: %w", instrumentName, err)
	}

	log.Printf("Instrument stopped: %s", instrumentName)
	return nil
}

// ListInstruments returns a list of all instruments and their status
//
// NOTE: This API endpoint will be refactored in future versions
func (h *Handler) ListInstruments(ctx context.Context) ([]client.InstrumentStatus, error) {
	instruments, err := h.client.ListInstruments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list instruments: %w", err)
	}

	return instruments, nil
}

// SendCommand sends a command to a specific instrument
//
// NOTE: This API endpoint will be refactored in future versions to align
// with the new instrument-script-server command structure
func (h *Handler) SendCommand(ctx context.Context, instrumentName, command string, params map[string]interface{}) (interface{}, error) {
	result, err := h.client.SendCommand(ctx, instrumentName, command, params)
	if err != nil {
		return nil, fmt.Errorf("failed to send command to instrument %s: %w", instrumentName, err)
	}

	return result, nil
}
