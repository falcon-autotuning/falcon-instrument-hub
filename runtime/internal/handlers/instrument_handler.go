//go:generate ./copy_script.sh

package handlers

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

//go:embed scripts/launch_instrument_daemon.py
var embeddedScript embed.FS

// InstrumentProcess represents a running instrument daemon
type InstrumentProcess struct {
	Name    string
	Process *os.Process
	Cmd     *exec.Cmd
	Cancel  context.CancelFunc
}

// InstrumentHandler handles instrument setup and destruction
type InstrumentHandler struct {
	logger      *logging.Logger
	natsURL     string
	instruments map[string]*InstrumentProcess
	mutex       sync.RWMutex
	setupSub    *nats.Subscription
	destroySub  *nats.Subscription
}

// NewInstrumentHandler creates a new instrument handler
func NewInstrumentHandler(logger *logging.Logger, natsURL string) *InstrumentHandler {
	return &InstrumentHandler{
		logger:      logger,
		natsURL:     natsURL,
		instruments: make(map[string]*InstrumentProcess),
	}
}

// Subscribe sets up NATS subscriptions for instrument commands
func (h *InstrumentHandler) Subscribe(nc *nats.Conn) error {
	h.logger.Info("INSTRUMENT_HANDLER", "Setting up instrument handler subscriptions")

	// Ensure script is up to date
	if err := h.ensureScriptExists(); err != nil {
		return fmt.Errorf("failed to ensure script exists: %w", err)
	}

	// Subscribe to setup commands
	setupSub, err := nc.Subscribe("SETUP_INSTRUMENT.external.*", h.handleSetupInstrument)
	if err != nil {
		return fmt.Errorf("failed to subscribe to SETUP_INSTRUMENT: %w", err)
	}
	h.setupSub = setupSub

	// Subscribe to destroy commands
	destroySub, err := nc.Subscribe("DESTROY_INSTRUMENT.external.*", h.handleDestroyInstrument)
	if err != nil {
		return fmt.Errorf("failed to subscribe to DESTROY_INSTRUMENT: %w", err)
	}
	h.destroySub = destroySub

	h.logger.Info("INSTRUMENT_HANDLER", "Instrument handler subscriptions ready")
	return nil
}

// Unsubscribe removes NATS subscriptions
func (h *InstrumentHandler) Unsubscribe() error {
	h.logger.Info("INSTRUMENT_HANDLER", "Unsubscribing from instrument handler")

	if h.setupSub != nil {
		h.setupSub.Unsubscribe()
	}
	if h.destroySub != nil {
		h.destroySub.Unsubscribe()
	}

	// Clean up all running instruments
	h.mutex.Lock()
	defer h.mutex.Unlock()

	for name, process := range h.instruments {
		h.logger.Info("INSTRUMENT_HANDLER", fmt.Sprintf("Stopping instrument %s during cleanup", name))
		h.stopInstrument(process)
	}
	h.instruments = make(map[string]*InstrumentProcess)

	return nil
}

// ensureScriptExists extracts the embedded script if needed
func (h *InstrumentHandler) ensureScriptExists() error {
	scriptsDir := "scripts"
	scriptPath := filepath.Join(scriptsDir, "launch_instrument_daemon.py")

	// Create scripts directory if it doesn't exist
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return fmt.Errorf("failed to create scripts directory: %w", err)
	}

	// Extract embedded script
	scriptContent, err := embeddedScript.ReadFile("scripts/launch_instrument_daemon.py")
	if err != nil {
		return fmt.Errorf("failed to read embedded script: %w", err)
	}

	// Write script to filesystem
	if err := os.WriteFile(scriptPath, scriptContent, 0755); err != nil {
		return fmt.Errorf("failed to write script file: %w", err)
	}

	h.logger.Info("INSTRUMENT_HANDLER", fmt.Sprintf("Script updated at %s", scriptPath))
	return nil
}

// handleSetupInstrument processes SETUP_INSTRUMENT commands
func (h *InstrumentHandler) handleSetupInstrument(msg *nats.Msg) {
	h.logger.Info("INSTRUMENT_HANDLER", fmt.Sprintf("Received SETUP_INSTRUMENT on subject: %s", msg.Subject))

	var req api.SetupInstrument
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		h.logger.Error("INSTRUMENT_HANDLER", fmt.Sprintf("Failed to unmarshal SETUP_INSTRUMENT: %v", err))
		return
	}

	if req.Name == "" {
		h.logger.Error("INSTRUMENT_HANDLER", "SETUP_INSTRUMENT missing instrument name")
		return
	}

	// Check if instrument is already running
	h.mutex.RLock()
	if _, exists := h.instruments[req.Name]; exists {
		h.mutex.RUnlock()
		h.logger.Error("INSTRUMENT_HANDLER", fmt.Sprintf("Instrument %s is already running", req.Name))
		return
	}
	h.mutex.RUnlock()

	// Start the instrument
	if err := h.startInstrument(req.Name); err != nil {
		h.logger.Error("INSTRUMENT_HANDLER", fmt.Sprintf("Failed to start instrument %s: %v", req.Name, err))
		return
	}

	h.logger.Info("INSTRUMENT_HANDLER", fmt.Sprintf("Successfully started instrument: %s", req.Name))
}

// handleDestroyInstrument processes DESTROY_INSTRUMENT commands
func (h *InstrumentHandler) handleDestroyInstrument(msg *nats.Msg) {
	h.logger.Info("INSTRUMENT_HANDLER", fmt.Sprintf("Received DESTROY_INSTRUMENT on subject: %s", msg.Subject))

	var req api.DestroyInstrument
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		h.logger.Error("INSTRUMENT_HANDLER", fmt.Sprintf("Failed to unmarshal DESTROY_INSTRUMENT: %v", err))
		return
	}

	if req.Name == "" {
		h.logger.Error("INSTRUMENT_HANDLER", "DESTROY_INSTRUMENT missing instrument name")
		return
	}

	// Find and stop the instrument
	h.mutex.Lock()
	defer h.mutex.Unlock()

	process, exists := h.instruments[req.Name]
	if !exists {
		h.logger.Error("INSTRUMENT_HANDLER", fmt.Sprintf("Instrument %s not found", req.Name))
		return
	}

	h.stopInstrument(process)
	delete(h.instruments, req.Name)

	h.logger.Info("INSTRUMENT_HANDLER", fmt.Sprintf("Successfully stopped instrument: %s", req.Name))
}

// startInstrument launches a new instrument daemon
func (h *InstrumentHandler) startInstrument(name string) error {
	scriptPath := filepath.Join("scripts", "launch_instrument_daemon.py")

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Create command
	cmd := exec.CommandContext(ctx, "python3", scriptPath, name, h.natsURL)

	// Set up process group for clean shutdown
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Start the process
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start instrument daemon: %w", err)
	}

	// Store the process info
	h.mutex.Lock()
	h.instruments[name] = &InstrumentProcess{
		Name:    name,
		Process: cmd.Process,
		Cmd:     cmd,
		Cancel:  cancel,
	}
	h.mutex.Unlock()

	h.logger.Info("INSTRUMENT_HANDLER", fmt.Sprintf("Started instrument %s with PID %d", name, cmd.Process.Pid))
	return nil
}

// stopInstrument terminates an instrument daemon
func (h *InstrumentHandler) stopInstrument(process *InstrumentProcess) {
	if process.Cancel != nil {
		process.Cancel()
	}

	if process.Process != nil {
		// Send SIGTERM first
		if err := process.Process.Signal(syscall.SIGTERM); err != nil {
			h.logger.Error("INSTRUMENT_HANDLER", fmt.Sprintf("Failed to send SIGTERM to %s: %v", process.Name, err))
		}

		// Wait a bit for graceful shutdown
		done := make(chan error, 1)
		go func() {
			_, err := process.Process.Wait()
			done <- err
		}()

		select {
		case <-done:
			h.logger.Info("INSTRUMENT_HANDLER", fmt.Sprintf("Instrument %s stopped gracefully", process.Name))
		case <-time.After(5 * time.Second):
			// Force kill if it doesn't stop gracefully
			h.logger.Error("INSTRUMENT_HANDLER", fmt.Sprintf("Force killing instrument %s", process.Name))
			process.Process.Kill()
		}
	}
}

// GetActiveInstruments returns a list of currently running instruments
func (h *InstrumentHandler) GetActiveInstruments() []string {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	names := make([]string, 0, len(h.instruments))
	for name := range h.instruments {
		names = append(names, name)
	}
	return names
}
