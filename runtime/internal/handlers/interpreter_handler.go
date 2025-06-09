//go:generate ./copy_script.sh

package handlers

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

// Handler Names
const (
	InterpreterHandlerName = "INTERPRETER_HANDLER"
)

// File Paths
const (
	ScriptsDir                  = "scripts"
	LaunchInterpreterScriptName = "launch_interpreter.py"
)

// Process Management
const (
	InterpreterGracefulShutdownTimeout = 5 // seconds
)

//go:embed scripts/launch_interpreter.py
var embeddedInterpreterScript embed.FS

// InterpreterProcess represents a running interpreter daemon
type InterpreterProcess struct {
	Process *os.Process
	Cmd     *exec.Cmd
	Cancel  context.CancelFunc
}

// InterpreterHandler handles interpreter setup and destruction
type InterpreterHandler struct {
	logger            *logging.Logger
	natsURL           string
	interpreter       *InterpreterProcess
	pythonInterpreter string
}

// NewInterpreterHandler creates a new interpreter handler
func NewInterpreterHandler(
	logger *logging.Logger,
	natsURL string,
	pythonInterpreter string,
) *InterpreterHandler {
	return &InterpreterHandler{
		logger:            logger,
		natsURL:           natsURL,
		pythonInterpreter: pythonInterpreter,
	}
}

// Start sets up and starts the interpreter
func (h *InterpreterHandler) Start() error {
	h.logger.Info(
		InterpreterHandlerName,
		"Starting interpreter handler",
	)

	// Ensure script is up to date
	if err := h.ensureScriptExists(); err != nil {
		return fmt.Errorf("failed to ensure interpreter script exists: %w", err)
	}

	// Start the interpreter
	if err := h.startInterpreter(); err != nil {
		return fmt.Errorf("failed to start interpreter: %w", err)
	}

	h.logger.Info(
		InterpreterHandlerName,
		"Interpreter handler started successfully",
	)
	return nil
}

// Stop removes NATS subscriptions and stops the interpreter
func (h *InterpreterHandler) Stop() error {
	h.logger.Info(
		InterpreterHandlerName,
		"Stopping interpreter handler",
	)

	// Clean up running interpreter
	if h.interpreter != nil {
		h.logger.Info(
			InterpreterHandlerName,
			"Stopping interpreter during cleanup",
		)
		h.stopInterpreter(h.interpreter)
		h.interpreter = nil
	}

	return nil
}

// ensureScriptExists extracts the embedded script if needed
func (h *InterpreterHandler) ensureScriptExists() error {
	scriptsDir := ScriptsDir
	scriptPath := filepath.Join(scriptsDir, LaunchInterpreterScriptName)

	// Create scripts directory if it doesn't exist
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return fmt.Errorf("failed to create scripts directory: %w", err)
	}

	// Extract embedded script
	scriptContent, err := embeddedInterpreterScript.ReadFile(
		filepath.Join(ScriptsDir, LaunchInterpreterScriptName),
	)
	if err != nil {
		return fmt.Errorf("failed to read embedded interpreter script: %w", err)
	}

	// Write script to filesystem
	if err := os.WriteFile(scriptPath, scriptContent, 0755); err != nil {
		return fmt.Errorf("failed to write interpreter script file: %w", err)
	}

	h.logger.Info(
		InterpreterHandlerName,
		fmt.Sprintf("Interpreter script updated at %s", scriptPath),
	)
	return nil
}

// startInterpreter launches the interpreter daemon
func (h *InterpreterHandler) startInterpreter() error {
	// Get absolute path to script (relative to where Go binary is running)
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	scriptPath := filepath.Join(
		currentDir,
		"scripts",
		LaunchInterpreterScriptName,
	)

	h.logger.Debug(
		InterpreterHandlerName,
		fmt.Sprintf("Using script path: %s", scriptPath),
	)

	// Verify script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("script file not found at %s", scriptPath)
	}

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Create command
	cmd := exec.CommandContext(
		ctx,
		h.pythonInterpreter,
		scriptPath,
		h.natsURL,
	)

	// Set up process group for clean shutdown
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Start the process
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start interpreter daemon: %w", err)
	}

	// Store the process info
	h.interpreter = &InterpreterProcess{
		Process: cmd.Process,
		Cmd:     cmd,
		Cancel:  cancel,
	}

	h.logger.Info(
		InterpreterHandlerName,
		fmt.Sprintf("Started interpreter with PID %d", cmd.Process.Pid),
	)
	return nil
}

// stopInterpreter terminates the interpreter daemon
func (h *InterpreterHandler) stopInterpreter(process *InterpreterProcess) {
	if process.Cancel != nil {
		process.Cancel()
	}

	if process.Process != nil {
		// Send SIGTERM first
		if err := process.Process.Signal(syscall.SIGTERM); err != nil {
			h.logger.Error(
				InterpreterHandlerName,
				fmt.Sprintf("Failed to send SIGTERM to interpreter: %v", err),
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
				InterpreterHandlerName,
				"Interpreter stopped gracefully",
			)
		case <-time.After(time.Duration(InterpreterGracefulShutdownTimeout) * time.Second):
			// Force kill if it doesn't stop gracefully
			h.logger.Error(
				InterpreterHandlerName,
				"Force killing interpreter",
			)
			process.Process.Kill()
		}
	}
}

// IsRunning checks if the interpreter is currently running
func (h *InterpreterHandler) IsRunning() bool {
	return h.interpreter != nil && h.interpreter.Process != nil
}
