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
	h.Log.Info(
		"Received %s on subject: %s",
		SetupInstrumentCommand,
		msg.Subject,
	)

	var req api.SetupInstrument
	if err := h.unmarshalAndValidate(msg.Data, &req, SetupInstrumentCommand); err != nil {
		return
	}

	// Check if instrument is already running
	h.mutex.RLock()
	if _, exists := h.Instruments[Name(req.Name)]; exists {
		h.mutex.RUnlock()
		h.Log.Error(
			"Instrument %s is already running",
			req.Name,
		)
		return
	}
	h.mutex.RUnlock()

	// Start the instrument
	if err := h.startInstrument(Name(req.Name)); err != nil {
		h.Log.Error(
			"Failed to start instrument %s: %v",
			req.Name,
			err,
		)
		return
	}

	h.Log.Info(
		"Successfully started instrument: %s",
		req.Name,
	)
}

// handleDestroyInstrument processes DESTROY_INSTRUMENT commands
func (h *Handler) handleDestroyInstrument(msg *nats.Msg) {
	h.Log.Info(
		"Received %s on subject: %s",
		DestroyInstrumentCommand,
		msg.Subject,
	)

	var req api.DestroyInstrument
	if err := h.unmarshalAndValidate(msg.Data, &req, DestroyInstrumentCommand); err != nil {
		return
	}

	// Queue for async destruction - no mutex needed, no blocking
	select {
	case h.destroyQueue <- Name(req.Name):
		h.Log.Info("Queued instrument %s for destruction", req.Name)
	default:
		h.Log.Error(
			"Destruction queue full - cannot destroy instrument %s",
			req.Name,
		)
	}
}

// handleConfirmInitialization processes CONFIRM_INITIALIZATION responses
func (h *Handler) handleConfirmInitialization(msg *nats.Msg) {
	var ports propertyIndexedPorts
	var configuration map[PropertyName]map[Index]PortConfiguration
	h.Log.Info(
		"Received %s on subject: %s",
		ConfirmInitializationCommand,
		msg.Subject,
	)

	var resp api.ConfirmInitialization
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		h.Log.Error(
			"Failed to unmarshal %s: %v",
			ConfirmInitializationCommand,
			err,
		)
		return
	}

	// Extract instrument name from subject (CONFIRM_INITIALIZATION.<name>)
	parts := strings.Split(msg.Subject, ".")
	if len(parts) < 2 {
		h.Log.Error(
			"Invalid subject format for %s: %s",
			ConfirmInitializationCommand,
			msg.Subject,
		)
		return
	}
	name := parts[len(parts)-1]

	// Update the instrument process with initialization data
	h.mutex.Lock()
	instrument, exists := h.Instruments[Name(name)]
	if !exists {
		h.mutex.Unlock()
		h.Log.Error(
			"Received initialization for unknown instrument: %s",
			name,
		)
		return
	}

	// Unmarshal the JSON strings into proper data structures
	if err := json.Unmarshal([]byte(resp.Port), &ports); err != nil {
		h.mutex.Unlock()
		h.Log.Error(
			"Failed to unmarshal ports JSON: %v",
			err,
		)
		return
	}

	if err := json.Unmarshal([]byte(resp.Init), &configuration); err != nil {
		h.mutex.Unlock()
		h.Log.Error(
			"Failed to unmarshal configuration JSON: %v",
			err,
		)
		return
	}

	instrument.Ports = ports
	instrument.Configuration = configuration
	instrument.Initialized = true
	h.mutex.Unlock()

	h.Log.Info(
		"Successfully initialized instrument: %s",
		name,
	)
	// Process the instrument ports to make them human-readable
	if h.portProcessor != nil {
		if err := h.portProcessor.ProcessInstrumentPorts(instrument.Ports, Name(name)); err != nil {
			h.Log.Error(
				"Failed to process ports for instrument %s: %v",
				name,
				err,
			)
		} else {
			h.Log.Debug(
				"Successfully processed ports for instrument %s",
				name,
			)
		}
	}
}

// handleUpdateDaemonProperty processes UPDATE_DAEMON_PROPERTY commands
func (h *Handler) handleUpdateDaemonProperty(msg *nats.Msg) {
	h.Log.Info(
		"Received %s on subject: %s",
		UpdateDaemonPropertyCommand,
		msg.Subject,
	)

	var req api.UpdateDaemonProperty
	if err := h.unmarshalAndValidate(msg.Data, &req, UpdateDaemonPropertyCommand); err != nil {
		return
	}

	if req.Property == "" {
		h.Log.Error(
			"%s missing property field",
			UpdateDaemonPropertyCommand,
		)
		return
	}
	set := SetInstruction{
		Property: PropertyName(req.Property),
		Name:     JsonPort(req.Name),
		Value:    fmt.Sprintf("%v", req.Value),
	}
	h.SetPropertyWithDefaults(set)
}
