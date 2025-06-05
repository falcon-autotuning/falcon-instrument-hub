package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	PortRequestHandlerName = "PORT_REQUEST_HANDLER"
	PortRequestSubject     = "PORT_REQUEST.external"
	PortPayloadSubject     = "PORT_PAYLOAD.external"
	KnobIdentifier         = "Knob"
	MeterIdentifier        = "Meter"
)

// PortRequestHandler handles PORT_REQUEST commands
type PortRequestHandler struct {
	logger            *logging.Logger
	nc                *nats.Conn
	subscription      *nats.Subscription
	instrumentHandler *instrument.Handler
	config            *config.Config
	nameMapping       map[string]*config.DeviceConnection
}

// NewPortRequestHandler creates a new handler
func NewPortRequestHandler(
	logger *logging.Logger,
	instrumentHandler *instrument.Handler,
	cfg *config.Config,
) *PortRequestHandler {
	// Build name mapping once during initialization
	nameMapping, err := config.BuildNameMapping(cfg.DeviceConfig, cfg.WireMap)
	if err != nil {
		logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to build name mapping: %v", err),
		)
		nameMapping = make(
			map[string]*config.DeviceConnection,
		) // Use empty mapping
	}
	return &PortRequestHandler{
		logger:            logger,
		instrumentHandler: instrumentHandler,
		config:            cfg,
		nameMapping:       nameMapping,
	}
}

// Subscribe starts listening for PORT_REQUEST commands
func (h *PortRequestHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error
	h.subscription, err = nc.Subscribe(
		PortRequestSubject+".>",
		h.handleMessage,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+PortRequestSubject+": %w",
			err,
		)
	}

	h.logger.Info(
		PortRequestHandlerName,
		"Subscribed to "+PortRequestSubject+".>",
	)
	return nil
}

// Unsubscribe stops listening for commands
func (h *PortRequestHandler) Unsubscribe() error {
	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
			h.logger.Error(
				PortRequestHandlerName,
				fmt.Sprintf("Failed to unsubscribe: %v", err),
			)
			return err
		}
		h.subscription = nil
	}

	h.logger.Info(
		PortRequestHandlerName,
		"Unsubscribed from "+PortRequestSubject,
	)
	return nil
}

// handleMessage processes incoming PORT_REQUEST commands
func (h *PortRequestHandler) handleMessage(msg *nats.Msg) {
	h.logger.Debug(
		PortRequestHandlerName,
		fmt.Sprintf("Received command: %s", string(msg.Data)),
	)

	// Extract the name from the subject (PORT_REQUEST.external.<name>)
	subjectParts := strings.Split(msg.Subject, ".")
	if len(subjectParts) < 3 {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Invalid subject format: %s", msg.Subject),
		)
		return
	}
	name := subjectParts[2]

	// Parse the incoming message
	var portRequest api.PortRequest
	if err := json.Unmarshal(msg.Data, &portRequest); err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to unmarshal PORT_REQUEST: %v", err),
		)
		return
	}

	// Get all active instruments and their port properties
	knobs, meters := h.instrumentHandler.CollectPortProperties()

	// Marshal knobs and meters arrays to JSON strings
	knobsJSON, err := json.Marshal(knobs)
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to marshal knobs array: %v", err),
		)
		return
	}

	metersJSON, err := json.Marshal(meters)
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to marshal meters array: %v", err),
		)
		return
	}

	// Create the response
	portPayload := api.PortPayload{
		Knobs:     string(knobsJSON),
		Meters:    string(metersJSON),
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal the response
	payloadData, err := json.Marshal(portPayload)
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to marshal PORT_PAYLOAD: %v", err),
		)
		return
	}

	// Send response on PORT_PAYLOAD.external.<name>
	responseSubject := fmt.Sprintf("%s.%s", PortPayloadSubject, name)
	if err := h.nc.Publish(responseSubject, payloadData); err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf(
				"Failed to publish response to %s: %v",
				responseSubject,
				err,
			),
		)
		return
	}

	h.logger.Info(
		PortRequestHandlerName,
		fmt.Sprintf(
			"Sent PORT_PAYLOAD response to %s with %d knobs and %d meters",
			responseSubject,
			len(knobs),
			len(meters),
		),
	)
}

// ProcessInstrumentPorts processes and augments ports for a specific instrument
// This should be called immediately after an instrument is loaded
func (h *PortRequestHandler) ProcessInstrumentPorts(
	instrumentName string,
) error {
	instrument, exists := h.instrumentHandler.Instruments[instrumentName]
	if !exists || instrument.Ports == nil {
		return fmt.Errorf(
			"instrument %s not found or has no ports",
			instrumentName,
		)
	}

	// Augment the ports with device connection information using pre-built
	// mapping
	if err := config.ProcessInstrumentPorts(instrument.Ports, h.nameMapping, instrumentName); err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf(
				"Failed to augment ports for instrument %s: %v",
				instrumentName,
				err,
			),
		)
		return err
	}

	h.logger.Debug(
		PortRequestHandlerName,
		fmt.Sprintf(
			"Successfully processed ports for instrument %s",
			instrumentName,
		),
	)

	return nil
}

// collectPortProperties queries all instruments for their Port properties
// and categorizes them into knobs and meters
func (h *PortRequestHandler) collectPortProperties() (knobs, meters []string) {
	// Get all active instruments
	activeInstruments := h.instrumentHandler.GetActiveInstruments()

	h.logger.Debug(
		PortRequestHandlerName,
		fmt.Sprintf("Collecting port properties from %d active instruments: %v",
			len(activeInstruments), activeInstruments),
	)

	// Collect ports from all active instruments (processing should already be
	// done)
	for _, instrumentName := range activeInstruments {
		// Get the instrument's ports directly from the handler
		if instrument, exists := h.instrumentHandler.Instruments[instrumentName]; exists &&
			instrument.Ports != nil {
			for _, innerMap := range instrument.Ports {
				// Type assert innerMap to map[int64]any or similar
				if portMap, ok := innerMap.(map[int64]any); ok {
					for _, value := range portMap {
						if valueStr, ok := value.(string); ok {
							if strings.Contains(valueStr, KnobIdentifier) {
								knobs = append(knobs, valueStr)
							}
							if strings.Contains(valueStr, MeterIdentifier) {
								meters = append(meters, valueStr)
							}
						}
					}
				}
			}
		}
	}

	h.logger.Debug(
		PortRequestHandlerName,
		fmt.Sprintf(
			"Collected %d knobs and %d meters",
			len(knobs),
			len(meters),
		),
	)

	return knobs, meters
}
