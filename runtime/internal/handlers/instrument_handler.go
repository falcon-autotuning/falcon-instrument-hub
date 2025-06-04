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
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

// Handler Names
const (
	InstrumentHandlerName = "INSTRUMENT_HANDLER"
)

// File Paths
const (
	ScriptsDir                 = "scripts"
	LaunchInstrumentScriptName = "launch_instrument_daemon.py"
)

// Process Management
const (
	GracefulShutdownTimeout = 5 // seconds
)

//go:embed scripts/launch_instrument_daemon.py
var embeddedScript embed.FS

var (
	SetupInstrumentCommand   = api.GetCommandName(api.SetupInstrument{})
	DestroyInstrumentCommand = api.GetCommandName(api.DestroyInstrument{})
	SetupInstrumentSubject   = SetupInstrumentCommand + ".external.*"
	DestroyInstrumentSubject = DestroyInstrumentCommand + ".external.*"
)

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
func NewInstrumentHandler(
	logger *logging.Logger,
	natsURL string,
) *InstrumentHandler {
	return &InstrumentHandler{
		logger:      logger,
		natsURL:     natsURL,
		instruments: make(map[string]*InstrumentProcess),
	}
}

// Subscribe sets up NATS subscriptions for instrument commands
func (h *InstrumentHandler) Subscribe(nc *nats.Conn) error {
	h.logger.Info(
		InstrumentHandlerName,
		"Setting up instrument handler subscriptions",
	)

	// Ensure script is up to date
	if err := h.ensureScriptExists(); err != nil {
		return fmt.Errorf("failed to ensure script exists: %w", err)
	}

	// Subscribe to setup commands
	setupSub, err := nc.Subscribe(
		SetupInstrumentSubject,
		h.handleSetupInstrument,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to %s: %w",
			SetupInstrumentCommand,
			err,
		)
	}
	h.setupSub = setupSub

	// Subscribe to destroy commands
	destroySub, err := nc.Subscribe(
		DestroyInstrumentSubject,
		h.handleDestroyInstrument,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to %s: %w",
			DestroyInstrumentCommand,
			err,
		)
	}
	h.destroySub = destroySub

	h.logger.Info(
		InstrumentHandlerName,
		"Instrument handler subscriptions ready",
	)
	return nil
}

// Unsubscribe removes NATS subscriptions
func (h *InstrumentHandler) Unsubscribe() error {
	h.logger.Info(
		InstrumentHandlerName,
		"Unsubscribing from instrument handler",
	)

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
		h.logger.Info(
			InstrumentHandlerName,
			fmt.Sprintf("Stopping instrument %s during cleanup", name),
		)
		h.stopInstrument(process)
	}
	h.instruments = make(map[string]*InstrumentProcess)

	return nil
}

// ensureScriptExists extracts the embedded script if needed
func (h *InstrumentHandler) ensureScriptExists() error {
	scriptsDir := ScriptsDir
	scriptPath := filepath.Join(scriptsDir, LaunchInstrumentScriptName)

	// Create scripts directory if it doesn't exist
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return fmt.Errorf("failed to create scripts directory: %w", err)
	}

	// Extract embedded script
	scriptContent, err := embeddedScript.ReadFile(
		filepath.Join(ScriptsDir, LaunchInstrumentScriptName),
	)
	if err != nil {
		return fmt.Errorf("failed to read embedded script: %w", err)
	}

	// Write script to filesystem
	if err := os.WriteFile(scriptPath, scriptContent, 0755); err != nil {
		return fmt.Errorf("failed to write script file: %w", err)
	}

	h.logger.Info(
		InstrumentHandlerName,
		fmt.Sprintf("Script updated at %s", scriptPath),
	)
	return nil
}

// handleSetupInstrument processes SETUP_INSTRUMENT commands
func (h *InstrumentHandler) handleSetupInstrument(msg *nats.Msg) {
	h.logger.Info(
		InstrumentHandlerName,
		fmt.Sprintf(
			"Received %s on subject: %s",
			SetupInstrumentCommand,
			msg.Subject,
		),
	)

	var req api.SetupInstrument
	if err := h.unmarshalAndValidate(msg.Data, &req, SetupInstrumentCommand); err != nil {
		return
	}

	// Check if instrument is already running
	h.mutex.RLock()
	if _, exists := h.instruments[req.Name]; exists {
		h.mutex.RUnlock()
		h.logger.Error(
			InstrumentHandlerName,
			fmt.Sprintf("Instrument %s is already running", req.Name),
		)
		return
	}
	h.mutex.RUnlock()

	// Start the instrument
	if err := h.startInstrument(req.Name); err != nil {
		h.logger.Error(
			InstrumentHandlerName,
			fmt.Sprintf("Failed to start instrument %s: %v", req.Name, err),
		)
		return
	}

	h.logger.Info(
		InstrumentHandlerName,
		fmt.Sprintf("Successfully started instrument: %s", req.Name),
	)
}

// handleDestroyInstrument processes DESTROY_INSTRUMENT commands
func (h *InstrumentHandler) handleDestroyInstrument(msg *nats.Msg) {
	h.logger.Info(
		InstrumentHandlerName,
		fmt.Sprintf(
			"Received %s on subject: %s",
			DestroyInstrumentCommand,
			msg.Subject,
		),
	)

	var req api.DestroyInstrument
	if err := h.unmarshalAndValidate(msg.Data, &req, DestroyInstrumentCommand); err != nil {
		return
	}

	// Find and stop the instrument
	h.mutex.Lock()
	defer h.mutex.Unlock()

	process, exists := h.instruments[req.Name]
	if !exists {
		h.logger.Error(
			InstrumentHandlerName,
			fmt.Sprintf("Instrument %s not found", req.Name),
		)
		return
	}

	h.stopInstrument(process)
	delete(h.instruments, req.Name)

	h.logger.Info(
		InstrumentHandlerName,
		fmt.Sprintf("Successfully stopped instrument: %s", req.Name),
	)
}

// startInstrument launches a new instrument daemon
func (h *InstrumentHandler) startInstrument(name string) error {
	scriptPath := filepath.Join(ScriptsDir, LaunchInstrumentScriptName)

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

	h.logger.Info(
		InstrumentHandlerName,
		fmt.Sprintf("Started instrument %s with PID %d", name, cmd.Process.Pid),
	)
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
			h.logger.Error(
				InstrumentHandlerName,
				fmt.Sprintf(
					"Failed to send SIGTERM to %s: %v",
					process.Name,
					err,
				),
			)
		}

		// Wait a bit for graceful shutdown
		done := make(chan error, 1)
		go func() {
			_, err := process.Process.Wait()
			done <- err
		}()

		select {
		case <-done:
			h.logger.Info(
				InstrumentHandlerName,
				fmt.Sprintf("Instrument %s stopped gracefully", process.Name),
			)
		case <-time.After(time.Duration(GracefulShutdownTimeout) * time.Second):
			// Force kill if it doesn't stop gracefully
			h.logger.Error(
				InstrumentHandlerName,
				fmt.Sprintf("Force killing instrument %s", process.Name),
			)
			process.Process.Kill()
		}
	}
}

// unmarshalAndValidate handles the common unmarshaling and validation logic
func (h *InstrumentHandler) unmarshalAndValidate(
	data []byte,
	req any,
	commandName string,
) error {
	if err := json.Unmarshal(data, req); err != nil {
		h.logger.Error(
			InstrumentHandlerName,
			fmt.Sprintf("Failed to unmarshal %s: %v", commandName, err),
		)
		return err
	}

	// Use reflection to get the Name field
	v := reflect.ValueOf(req).Elem()
	nameField := v.FieldByName("Name")
	if !nameField.IsValid() || nameField.String() == "" {
		h.logger.Error(
			InstrumentHandlerName,
			fmt.Sprintf("%s missing instrument name", commandName),
		)
		return fmt.Errorf("missing instrument name")
	}

	return nil
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
