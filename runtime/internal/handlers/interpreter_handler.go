//go:generate ./copy_script.sh

package handlers

import (
	"bytes"
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
	Process   *os.Process
	Cmd       *exec.Cmd
	Cancel    context.CancelFunc
	Stdout    *bytes.Buffer
	Stderr    *bytes.Buffer
	StartTime time.Time
}

// InterpreterHandler handles interpreter setup and destruction
type InterpreterHandler struct {
	logger            *logging.Logger
	Log               *LogWrapper
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
		Log:               NewLogWrapper(logger, InterpreterHandlerName),
		natsURL:           natsURL,
		pythonInterpreter: pythonInterpreter,
	}
}

// Start sets up and starts the interpreter
func (h *InterpreterHandler) Start() error {
	h.Log.Info(
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

	h.Log.Info(
		"Interpreter handler started successfully",
	)
	return nil
}

// Stop removes NATS subscriptions and stops the interpreter
func (h *InterpreterHandler) Stop() error {
	h.Log.Info(
		"Stopping interpreter handler",
	)

	// Clean up running interpreter
	if h.interpreter != nil {
		h.Log.Info(
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

	h.Log.Info(
		"Interpreter script updated at %s",
		scriptPath,
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

	h.Log.Debug(
		"Using script path: %s", scriptPath,
	)

	// Verify script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		h.Log.Error("Script file does not exist: %s", scriptPath)
		return fmt.Errorf("script file not found at %s", scriptPath)
	}
	// Check if python interpreter exists
	if _, err := os.Stat(h.pythonInterpreter); os.IsNotExist(err) {
		h.Log.Error(
			"Python interpreter does not exist: %s",
			h.pythonInterpreter,
		)
		return fmt.Errorf(
			"python interpreter does not exist: %s",
			h.pythonInterpreter,
		)
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

	// Set up pipes to capture output
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}

	// Copy current environment and add any needed variables
	cmd.Env = os.Environ()

	// Start the process
	if err := cmd.Start(); err != nil {
		cancel()
		h.Log.Error(
			"Failed to start interpreter daemon: %v", err,
		)
		return fmt.Errorf("failed to start interpreter daemon: %w", err)
	}
	// Store the process info
	h.interpreter = &InterpreterProcess{
		Process:   cmd.Process,
		Cmd:       cmd,
		Cancel:    cancel,
		StartTime: time.Now(),
	}

	h.Log.Info(
		"Started interpreter with PID %d", cmd.Process.Pid,
	)
	// Start a goroutine to monitor the process
	go h.monitorProcess(h.interpreter)
	return nil
}

// monitorProcess monitors a running instrument process and logs any issues
func (h *InterpreterHandler) monitorProcess(process *InterpreterProcess) {
	defer func() {
		if r := recover(); r != nil {
			h.Log.Error(
				"Process monitor panic \n panic %v",
				r,
			)
		}
	}()

	// Wait for the process to complete
	err := process.Cmd.Wait()

	// Get output
	stdout := ""
	stderr := ""
	if process.Stdout != nil {
		stdout = process.Stdout.String()
	}
	if process.Stderr != nil {
		stderr = process.Stderr.String()
	}

	duration := time.Since(process.StartTime)

	// Log the process completion
	if err != nil {
		h.Log.Error(
			"Interpreter daemon process exited with error \n pid %v \n error %v \n exit code %v \n runtime %v \n stdout %s \n stderr %s",
			process.Cmd.Process.Pid,
			err,
			process.Cmd.ProcessState.ExitCode(),
			duration.Seconds(),
			stdout,
			stderr,
		)
	} else {
		h.Log.Info(
			"Interpreter daemon process exited normally \n pid %v \n runtime %v \n stdout %s \n stderr %s",
			process.Cmd.Process.Pid,
			duration.Seconds(),
			stdout,
			stderr,
		)
	}
}

// stopInterpreter terminates the interpreter daemon
func (h *InterpreterHandler) stopInterpreter(process *InterpreterProcess) {
	if process.Cancel != nil {
		process.Cancel()
	}

	if process.Process != nil {
		// Send SIGTERM first
		if err := process.Process.Signal(syscall.SIGTERM); err != nil {
			h.Log.Error(
				"Failed to send SIGTERM to interpreter: %v", err,
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
			h.Log.Info(
				"Interpreter stopped gracefully",
			)
		case <-time.After(time.Duration(InterpreterGracefulShutdownTimeout) * time.Second):
			// Force kill if it doesn't stop gracefully
			h.Log.Error(
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

// LogWrapper provides convenient logging with automatic handler name and
// sprintf formatting
type LogWrapper struct {
	logger      *logging.Logger
	handlerName string
}

// NewLogWrapper creates a new log wrapper for the given handler
func NewLogWrapper(logger *logging.Logger, handlerName string) *LogWrapper {
	return &LogWrapper{
		logger:      logger,
		handlerName: handlerName,
	}
}

// Info logs an info message with sprintf formatting
func (l *LogWrapper) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Info(l.handlerName, msg)
}

// Warn logs a warning message with sprintf formatting
func (l *LogWrapper) Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Warn(l.handlerName, msg)
}

// Error logs an error message with sprintf formatting
func (l *LogWrapper) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Error(l.handlerName, msg)
}

// Debug logs a debug message with sprintf formatting
func (l *LogWrapper) Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Debug(l.handlerName, msg)
}
