package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	// Base message types
	PortRequestType = "PORT_REQUEST"
	PortPayloadType = "PORT_PAYLOAD"

	// Handler and subject constants
	PortRequestHandlerName = "PORT_REQUEST_HANDLER"
	PortRequestSubject     = "INSTRUMENTHUB.PORT_REQUEST"
	PortPayloadSubject     = "FALCON.PORT_PAYLOAD"
)

// PortRequestHandler handles PORT_REQUEST messages
type PortRequestHandler struct {
	logger            *logging.Logger
	nc                *nats.Conn
	subscription      *nats.Subscription
	instrumentHandler *instrument.Handler
	config            *config.Config
}

// NewPortRequestHandler creates a new handler
func NewPortRequestHandler(
	logger *logging.Logger,
	instrumentHandler *instrument.Handler,
	cfg *config.Config,
) *PortRequestHandler {
	return &PortRequestHandler{
		logger:            logger,
		instrumentHandler: instrumentHandler,
		config:            cfg,
	}
}

// Subscribe starts listening for PORT_REQUEST messages
func (h *PortRequestHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error

	h.subscription, err = nc.Subscribe(PortRequestSubject, h.handlePortRequest)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to %s: %w",
			PortRequestSubject,
			err,
		)
	}

	h.logger.Info(PortRequestHandlerName, "Subscribed to "+PortRequestSubject)
	return nil
}

// Unsubscribe stops listening for messages
func (h *PortRequestHandler) Unsubscribe() error {
	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
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

// handlePortRequest processes incoming PORT_REQUEST messages
func (h *PortRequestHandler) handlePortRequest(msg *nats.Msg) {
	h.logger.Debug(
		PortRequestHandlerName,
		fmt.Sprintf("Received %s : %s", PortRequestType, string(msg.Data)),
	)

	// Parse the request
	var request api.PortRequest
	if err := json.Unmarshal(msg.Data, &request); err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to unmarshal %s : %v", PortRequestType, err),
		)
		return
	}

	// Collect port properties using the instrument handler's existing
	// functionality
	knobs, meters := h.instrumentHandler.CollectPortProperties()
	encodedKnobs, err := serializePortsToCerealJSON(knobs)
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to serialize knobs: %v", err),
		)
		return
	}

	encodedMeters, err := serializePortsToCerealJSON(meters)
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to serialize meters: %v", err),
		)
		return
	}

	// Create response
	response := api.PortPayload{
		Knobs:     encodedKnobs,
		Meters:    encodedMeters,
		Timestamp: request.Timestamp,
	}

	// Marshal response
	h.logger.Debug(PortRequestHandlerName, "Marshalling response")
	responseData, err := json.Marshal(response)
	h.logger.Debug(PortRequestHandlerName, "Finished marshalling response")
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to marshal %s : %v", PortPayloadType, err),
		)
		return
	}

	// Send response
	if err := h.nc.Publish(PortPayloadSubject, responseData); err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to publish %s : %v", PortPayloadType, err),
		)
		return
	}

	h.logger.Debug(
		PortRequestHandlerName,
		fmt.Sprintf("Sent  %s ", PortPayloadType),
	)
}

// serializePortsToCerealJSON converts a list of serialized InstrumentPort
// objects into a serialized Ports object using falcon-core C API wrappers.
func serializePortsToCerealJSON(portPayloads []instrument.JsonPort) (string, error) {
	portHandles := make([]*instrumentport.Handle, 0, len(portPayloads))
	for _, p := range portPayloads {
		h, err := instrumentport.FromJSON(p.String())
		if err != nil {
			return "", fmt.Errorf("failed to parse instrument port JSON: %w", err)
		}
		defer h.Close()
		portHandles = append(portHandles, h)
	}

	portsHandle, err := ports.New(portHandles)
	if err != nil {
		return "", fmt.Errorf("failed to create ports handle: %w", err)
	}
	defer portsHandle.Close()

	jsonStr, err := portsHandle.ToJSON()
	if err != nil {
		return "", fmt.Errorf("failed to serialize ports: %w", err)
	}

	return jsonStr, nil
}

// isOhmicConnection checks if a port JSON represents an Ohmic connection
func (h *PortRequestHandler) isOhmicConnection(portJSON string) bool {
	var portData map[string]any
	if err := json.Unmarshal([]byte(portJSON), &portData); err != nil {
		return false
	}

	// Check if connection_type is "Ohmic"
	if connectionType, exists := portData["connection_type"]; exists {
		return connectionType == "Ohmic"
	}

	return false
}
