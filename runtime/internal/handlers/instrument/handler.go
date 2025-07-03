package instrument

import (
	"fmt"
	"syscall"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

// NewHandler creates a new instrument handler
func NewHandler(
	logger *logging.Logger,
	natsURL string,
	nc *nats.Conn,
	cfg *config.Config,
	pythonInterpreter string,
) (*Handler, error) {
	Log := NewLogWrapper(logger, HandlerName)
	portProcessor, err := NewPortProcessor(logger, Log, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create port processor: %w", err)
	}
	h := &Handler{
		logger:            logger,
		Log:               Log,
		natsURL:           natsURL,
		nc:                nc,
		Instruments:       make(map[Name]*InstrumentProcess),
		subscriptions:     make([]*nats.Subscription, 0),
		portProcessor:     portProcessor,
		pythonInterpreter: pythonInterpreter,
		cleanupStop:       make(chan struct{}),
		destroyQueue:      make(chan Name, 100),
	}
	go h.destroyWorker()
	return h, nil
}

// destroyWorker processes instrument destruction requests asynchronously
func (h *Handler) destroyWorker() {
	h.Log.Debug("Destroy worker started")

	for name := range h.destroyQueue {
		h.Log.Info("Processing destruction request for instrument: %s", name)

		h.mutex.Lock()
		instrument, exists := h.Instruments[name]
		if !exists {
			h.mutex.Unlock()
			h.Log.Warn("Attempted to destroy non-existent instrument %s", name)
			continue
		}

		// Check if already completed
		if instrument.Completed {
			h.Log.Info("Instrument %s already completed, cleaning up", name)
			delete(h.Instruments, name)
			h.mutex.Unlock()
			continue
		}

		// Remove from map and unlock before stopping (which may take time)
		delete(h.Instruments, name)
		h.mutex.Unlock()

		// Stop the instrument outside the critical section
		h.Log.Info("Stopping instrument: %s", name)
		h.stopInstrument(instrument)
		h.Log.Info("Successfully destroyed instrument: %s", name)
	}

	h.Log.Debug("Destroy worker stopped")
}

// GetActiveInstruments returns a list of currently running instruments
func (h *Handler) GetActiveInstruments() []Name {
	h.Log.Debug("Fetching all the active instrument")
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	names := make([]Name, 0, len(h.Instruments))
	for name := range h.Instruments {
		names = append(names, name)
	}
	return names
}

// CleanupCompletedProcesses removes completed processes from the map
// This should be called periodically to prevent memory leaks
func (h *Handler) CleanupCompletedProcesses() {
	h.Log.Debug("Attempting to cleanup all the completed processes")
	h.mutex.Lock()

	names := make([]Name, 0)

	for name, process := range h.Instruments {
		if process.Completed {
			// Keep completed processes for a while for debugging
			if time.Since(process.CompletedAt) > 5*time.Minute {
				names = append(names, name)
				delete(h.Instruments, name)
			}
		}
	}
	h.mutex.Unlock()
	for _, name := range names {
		h.Log.Debug(
			"Cleaned up completed process %s",
			name,
		)
	}
}

// GetProcessStatus returns the current status of a process
func (h *Handler) GetProcessStatus(
	name Name,
) (status string, exists bool) {
	h.Log.Debug("Trying to get the status of instrument %s", name)
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	instrument, exists := h.Instruments[name]
	if !exists {
		return "not_found", false
	}

	if instrument.Completed {
		if instrument.ExitError != nil {
			return "completed_with_error", true
		}
		return "completed_successfully", true
	}

	// Check if process is still alive
	if instrument.Cmd.Process != nil {
		err := instrument.Cmd.Process.Signal(syscall.Signal(0))
		if err != nil {
			return "dead", true
		}
		return "running", true
	}

	return "unknown", true
}

// stopInstrument terminates an instrument daemon
func (h *Handler) stopInstrument(process *InstrumentProcess) {
	if process.Cancel != nil {
		process.Cancel()
	}

	if process.Cmd.Process != nil {
		// Send SIGTERM first
		if err := process.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
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
			_, err := process.Cmd.Process.Wait()
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
			process.Cmd.Process.Kill()
		}
	}
}

func (h *Handler) cleanupLoop() {
	ticker := time.NewTicker(2 * time.Minute) // Cleanup every 2 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.CleanupCompletedProcesses()

		case <-h.cleanupStop:
			h.Log.Debug("Cleanup loop stopping")
			return
		}
	}
}
