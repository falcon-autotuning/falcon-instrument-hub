package instrument

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/nats-io/nats.go"
)

// handleSetupInstrument processes SETUP_INSTRUMENT commands
func (h *Handler) handleSetupInstrument(msg *nats.Msg) {
	h.logger.Info(
		HandlerName,
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
			HandlerName,
			fmt.Sprintf("Instrument %s is already running", req.Name),
		)
		return
	}
	h.mutex.RUnlock()

	// Start the instrument
	if err := h.startInstrument(req.Name); err != nil {
		h.logger.Error(
			HandlerName,
			fmt.Sprintf("Failed to start instrument %s: %v", req.Name, err),
		)
		return
	}

	h.logger.Info(
		HandlerName,
		fmt.Sprintf("Successfully started instrument: %s", req.Name),
	)
}

// handleDestroyInstrument processes DESTROY_INSTRUMENT commands
func (h *Handler) handleDestroyInstrument(msg *nats.Msg) {
	h.logger.Info(
		HandlerName,
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
			HandlerName,
			fmt.Sprintf("Instrument %s not found", req.Name),
		)
		return
	}

	h.stopInstrument(process)
	delete(h.instruments, req.Name)

	h.logger.Info(
		HandlerName,
		fmt.Sprintf("Successfully stopped instrument: %s", req.Name),
	)
}

// handleConfirmInitialization processes CONFIRM_INITIALIZATION responses
func (h *Handler) handleConfirmInitialization(msg *nats.Msg) {
	h.logger.Info(
		HandlerName,
		fmt.Sprintf(
			"Received %s on subject: %s",
			ConfirmInitializationCommand,
			msg.Subject,
		),
	)

	var resp api.ConfirmInitialization
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		h.logger.Error(
			HandlerName,
			fmt.Sprintf(
				"Failed to unmarshal %s: %v",
				ConfirmInitializationCommand,
				err,
			),
		)
		return
	}

	// Extract instrument name from subject (CONFIRM_INITIALIZATION.<name>)
	parts := strings.Split(msg.Subject, ".")
	if len(parts) < 2 {
		h.logger.Error(
			HandlerName,
			fmt.Sprintf(
				"Invalid subject format for %s: %s",
				ConfirmInitializationCommand,
				msg.Subject,
			),
		)
		return
	}
	instrumentName := parts[len(parts)-1]

	// Update the instrument process with initialization data
	h.mutex.Lock()
	defer h.mutex.Unlock()

	process, exists := h.instruments[instrumentName]
	if !exists {
		h.logger.Error(
			HandlerName,
			fmt.Sprintf(
				"Received initialization for unknown instrument: %s",
				instrumentName,
			),
		)
		return
	}

	process.Ports = resp.Port
	process.Configuration = resp.Init
	process.Initialized = true

	h.logger.Info(
		HandlerName,
		fmt.Sprintf("Successfully initialized instrument: %s", instrumentName),
	)
}
