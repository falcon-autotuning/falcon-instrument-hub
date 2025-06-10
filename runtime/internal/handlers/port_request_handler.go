package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

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

	// Handler and subject constants derived from base types
	PortRequestHandlerName = "PORT_REQUEST_HANDLER"
	PortRequestSubject     = PortRequestType + ".external.>"
	PortPayloadSubject     = PortPayloadType + ".external"
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

	// Create response
	response := api.PortPayload{
		Knobs:     fmt.Sprintf("[%s]", strings.Join(knobs, ",")),
		Meters:    fmt.Sprintf("[%s]", strings.Join(meters, ",")),
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

	parts := strings.Split(msg.Subject, ".")
	externalServerName := parts[len(parts)-1] // Get last part

	// Send response
	if err := h.nc.Publish(PortPayloadType+".external."+externalServerName, responseData); err != nil {
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
