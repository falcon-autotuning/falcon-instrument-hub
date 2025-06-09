package instrument

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// startInstrument launches a new instrument daemon
func (h *Handler) startInstrument(name string) error {
	// Get absolute path to script (relative to where Go binary is running)
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	scriptPath := filepath.Join(
		currentDir,
		"scripts",
		LaunchInstrumentScriptName,
	)

	h.Log.Debug("Using script path: %s", scriptPath)

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
		name,
		h.natsURL,
	)

	// Add detailed logging
	h.Log.Info(
		"Starting instrument daemon \n instrument %s \n python interpreter %s \n script path %s \n nats URL %s",
		name,
		h.pythonInterpreter,
		scriptPath,
		h.natsURL,
	)
	// Check if script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		h.Log.Error("Script file does not exist: %s", scriptPath)
		return fmt.Errorf("script file does not exist: %s", scriptPath)
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

	// Set up pipes to capture output
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}

	// Copy current environment and add any needed variables
	cmd.Env = os.Environ()

	// Start the process
	err = cmd.Start()
	if err != nil {
		cancel()
		h.Log.Error(
			"Failed to start instrument daemon \n instrument %s \n error %v",
			name,
			err,
		)
		return fmt.Errorf("failed to start instrument %s: %w", name, err)
	}

	h.Log.Info(
		"Instrument daemon started successfully \n instrument %s \n pid %v",
		name,
		cmd.Process.Pid,
	)

	// Store the process info
	h.mutex.Lock()
	h.Instruments[name] = &InstrumentProcess{
		Name:      name,
		Cmd:       cmd,
		Cancel:    cancel,
		StartTime: time.Now(),
	}
	h.mutex.Unlock()

	// Start a goroutine to monitor the process
	go h.monitorProcess(h.Instruments[name])

	return nil
}

// monitorProcess monitors a running instrument process and logs any issues
func (h *Handler) monitorProcess(process *InstrumentProcess) {
	defer func() {
		if r := recover(); r != nil {
			h.Log.Error(
				"Process monitor panic \n instrument %s \n panic %v",
				process.Name,
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
			"Instrument daemon process exited with error \n instrument %s \n pid %v \n error %v \n exit code %v \n runtime %v \n stdout %s \n stderr %s",
			process.Name,
			process.Cmd.Process.Pid,
			err,
			process.Cmd.ProcessState.ExitCode(),
			duration.Seconds(),
			stdout,
			stderr,
		)
	} else {
		h.Log.Info(
			"Instrument daemon process exited normally \n instrument %s \n pid %v \n runtime %v \n stdout %s \n stderr %s",
			process.Name,
			process.Cmd.Process.Pid,
			duration.Seconds(),
			stdout,
			stderr,
		)
	}

	// Remove from active processes
	h.mutex.Lock()
	if proc, exists := h.Instruments[process.Name]; exists && proc == process {
		// Mark as completed so other operations know it's dead
		process.Completed = true
		process.CompletedAt = time.Now()
		process.ExitError = err
	}
	h.mutex.Unlock()
}
