package instrument

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	if _, exists := h.Instruments[req.Name]; exists {
		h.mutex.RUnlock()
		h.Log.Error(
			"Instrument %s is already running",
			req.Name,
		)
		return
	}
	h.mutex.RUnlock()

	// Start the instrument
	if err := h.startInstrument(req.Name); err != nil {
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

	// Find and stop the instrument
	h.mutex.Lock()
	process, exists := h.Instruments[req.Name]
	if !exists {
		h.mutex.Unlock()
		h.Log.Warn(
			"Attempted to destroy non-existent instrument %s",
			req.Name,
		)
		return
	}
	h.mutex.Unlock()

	// Check if process already completed
	if process.Completed {
		h.Log.Info(
			"Instrument already completed at %v, cleaning up %s",
			process.CompletedAt,
			req.Name,
		)

		// Remove from map since it's already dead
		h.mutex.Lock()
		delete(h.Instruments, req.Name)
		h.mutex.Unlock()

		return
	}

	h.stopInstrument(process)
}

// handleConfirmInitialization processes CONFIRM_INITIALIZATION responses
func (h *Handler) handleConfirmInitialization(msg *nats.Msg) {
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
	instrumentName := parts[len(parts)-1]

	// Update the instrument process with initialization data
	h.mutex.Lock()
	defer h.mutex.Unlock()

	process, exists := h.Instruments[instrumentName]
	if !exists {
		h.Log.Error(
			"Received initialization for unknown instrument: %s",
			instrumentName,
		)
		return
	}

	// Unmarshal the JSON strings into proper data structures
	var ports map[string]any
	if err := json.Unmarshal([]byte(resp.Port), &ports); err != nil {
		h.Log.Error(
			"Failed to unmarshal ports JSON: %v",
			err,
		)
		return
	}

	var configuration map[string]any
	if err := json.Unmarshal([]byte(resp.Init), &configuration); err != nil {
		h.Log.Error(
			"Failed to unmarshal configuration JSON: %v",
			err,
		)
		return
	}

	process.Ports = ports
	process.Configuration = configuration
	process.Initialized = true

	h.Log.Info(
		"Successfully initialized instrument: %s",
		instrumentName,
	)
	// Process the instrument ports to make them human-readable
	// This should be done after the instrument is initialized and ports are set
	if h.portProcessor != nil {
		if err := h.portProcessor.ProcessInstrumentPorts(instrumentName, process.Ports); err != nil {
			h.Log.Error(
				"Failed to process ports for instrument %s: %v",
				instrumentName,
				err,
			)
		} else {
			h.Log.Debug(
				"Successfully processed ports for instrument %s",
				instrumentName,
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

	// Find the instrument and index by searching through all instrument ports
	var targetInstrument string
	var targetIndex int64
	found := false
	h.mutex.RLock()

	for instrumentName, process := range h.Instruments {
		if !process.Initialized || process.Ports == nil {
			continue
		}

		// Check if this instrument has the requested property
		if propertyData, exists := process.Ports[req.Property]; exists {
			h.Log.Info(
				"Received %s end it exists %v",
				propertyData,
				exists,
			)
			// Try map[int64]any first (direct assignment)
			if propertyMap, ok := propertyData.(map[int64]any); ok {
				h.Log.Info(
					"Found property map with int64 keys: %v",
					propertyMap,
				)
				// Search through the index map to find the matching port name
				for index, portValue := range propertyMap {
					if portName, ok := portValue.(string); ok &&
						portName == req.Name {
						targetInstrument = instrumentName
						targetIndex = index
						found = true
						break
					}
				}
			} else if propertyMapStr, ok := propertyData.(map[string]any); ok {
				// Handle case where JSON unmarshaling converts int64 keys to
				// strings
				h.Log.Info(
					"Found property map with string keys: %v",
					propertyMapStr,
				)
				for indexStr, portValue := range propertyMapStr {
					if portName, ok := portValue.(string); ok && portName == req.Name {
						// Convert string key back to int64
						if index, err := strconv.ParseInt(indexStr, 10, 64); err == nil {
							targetInstrument = instrumentName
							targetIndex = index
							found = true
							break
						}
					}
				}
			}
			if found {
				break
			}
		}
	}
	h.mutex.RUnlock()

	if !found {
		h.Log.Error(
			"Could not find instrument with property %s and name %s",
			req.Property,
			req.Name,
		)
		return
	}

	// Create and send the SET command to the target instrument
	setCommand := api.Set{
		Property: req.Property,
		Index:    targetIndex,
		Value:    req.Value,
	}

	setData, err := json.Marshal(setCommand)
	if err != nil {
		h.Log.Error(
			"Failed to marshal %s command: %v", SetCommand, err,
		)
		return
	}

	// Publish the SET command to the target instrument
	setSubject := fmt.Sprintf("%s.%s", SetCommand, targetInstrument)

	if err := h.nc.Publish(setSubject, setData); err != nil {
		h.logger.Error(
			HandlerName,
			fmt.Sprintf(
				"Failed to publish %s command to %s: %v",
				SetCommand,
				setSubject,
				err,
			),
		)
		return
	}

	h.logger.Info(
		HandlerName,
		fmt.Sprintf(
			"Successfully sent %s command to %s: property=%s, index=%d, value=%v",
			SetCommand,
			setSubject,
			req.Property,
			targetIndex,
			req.Value,
		),
	)
}
