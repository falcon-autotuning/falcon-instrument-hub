package instrument

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// startInstrument launches a new instrument daemon
func (h *Handler) startInstrument(name string) error {
	scriptPath := filepath.Join(ScriptsDir, LaunchInstrumentScriptName)
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

	// Set up process group for clean shutdown
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Start the process
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start instrument daemon: %w", err)
	}

	// Store the process info
	h.mutex.Lock()
	h.Instruments[name] = &InstrumentProcess{
		Name:          name,
		Process:       cmd.Process,
		Cmd:           cmd,
		Cancel:        cancel,
		Ports:         nil,
		Configuration: nil,
		Initialized:   false,
	}
	h.mutex.Unlock()

	h.logger.Info(
		HandlerName,
		fmt.Sprintf("Started instrument %s with PID %d", name, cmd.Process.Pid),
	)
	return nil
}

// stopInstrument terminates an instrument daemon
func (h *Handler) stopInstrument(process *InstrumentProcess) {
	if process.Cancel != nil {
		process.Cancel()
	}

	if process.Process != nil {
		// Send SIGTERM first
		if err := process.Process.Signal(syscall.SIGTERM); err != nil {
			h.logger.Error(
				HandlerName,
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
				HandlerName,
				fmt.Sprintf("Instrument %s stopped gracefully", process.Name),
			)
		case <-time.After(time.Duration(GracefulShutdownTimeout) * time.Second):
			// Force kill if it doesn't stop gracefully
			h.logger.Error(
				HandlerName,
				fmt.Sprintf("Force killing instrument %s", process.Name),
			)
			process.Process.Kill()
		}
	}
}
