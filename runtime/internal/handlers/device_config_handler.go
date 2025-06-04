package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

// DeviceConfigHandler handles DEVICE_CONFIG_REQUEST.external.<name> messages
type DeviceConfigHandler struct {
	config       *config.Config
	logger       *logging.Logger
	nc           *nats.Conn
	subscription *nats.Subscription
}

// NewDeviceConfigHandler creates a new device config handler
func NewDeviceConfigHandler(cfg *config.Config, logger *logging.Logger) *DeviceConfigHandler {
	return &DeviceConfigHandler{
		config: cfg,
		logger: logger,
	}
}

// Subscribe subscribes to DEVICE_CONFIG_REQUEST.external.* channels
func (h *DeviceConfigHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc

	// Subscribe to DEVICE_CONFIG_REQUEST.external.*
	sub, err := nc.Subscribe("DEVICE_CONFIG_REQUEST.external.*", h.handleDeviceConfigRequest)
	if err != nil {
		return fmt.Errorf("failed to subscribe to DEVICE_CONFIG_REQUEST.external.*: %w", err)
	}
	h.subscription = sub

	h.logger.Info("DEVICE_CONFIG_HANDLER", "Subscribed to DEVICE_CONFIG_REQUEST.external.* channels")
	log.Printf("DEVICE_CONFIG handler subscribed to DEVICE_CONFIG_REQUEST.external.* channels")

	return nil
}

// Unsubscribe unsubscribes from DEVICE_CONFIG_REQUEST.external.* channels
func (h *DeviceConfigHandler) Unsubscribe() error {
	if h.subscription != nil {
		err := h.subscription.Unsubscribe()
		if err != nil {
			h.logger.Error("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Failed to unsubscribe: %v", err))
			return err
		}
		h.logger.Info("DEVICE_CONFIG_HANDLER", "Unsubscribed from DEVICE_CONFIG_REQUEST.external.* channels")
		h.subscription = nil
	}
	return nil
}

// handleDeviceConfigRequest processes incoming DEVICE_CONFIG_REQUEST messages
func (h *DeviceConfigHandler) handleDeviceConfigRequest(msg *nats.Msg) {
	channel := msg.Subject
	rawData := msg.Data

	h.logger.Debug("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Received request on %s: %s", channel, string(rawData)))

	// Parse channel: DEVICE_CONFIG_REQUEST.external.<name>
	parts := strings.Split(channel, ".")
	if len(parts) != 3 {
		h.logger.Error("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Invalid channel format %s, expected DEVICE_CONFIG_REQUEST.external.<name>", channel))
		h.sendErrorResponse(msg, "Invalid channel format")
		return
	}

	if parts[0] != "DEVICE_CONFIG_REQUEST" || parts[1] != "external" {
		h.logger.Error("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Invalid channel format %s, expected DEVICE_CONFIG_REQUEST.external.<name>", channel))
		h.sendErrorResponse(msg, "Invalid channel format")
		return
	}

	name := parts[2]

	// Parse the device config request
	var deviceConfigReq api.DeviceConfigRequest
	if err := json.Unmarshal(rawData, &deviceConfigReq); err != nil {
		h.logger.Error("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Failed to decode device config request JSON: %v", err))
		h.sendErrorResponse(msg, fmt.Sprintf("Failed to unmarshal request: %v", err))
		return
	}

	// Marshal the device config to JSON
	deviceConfigJSON, err := json.Marshal(h.config.DeviceConfig)
	if err != nil {
		h.logger.Error("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Failed to marshal device config: %v", err))
		h.sendErrorResponse(msg, "Failed to marshal device config")
		return
	}

	// Create the response
	response := api.DeviceConfigResponse{
		Response:  string(deviceConfigJSON),
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal the response
	responseData, err := json.Marshal(response)
	if err != nil {
		h.logger.Error("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Failed to marshal device config response: %v", err))
		h.sendErrorResponse(msg, "Failed to marshal response")
		return
	}

	// Send response back to DEVICE_CONFIG_RESPONSE.external.<name>
	responseChannel := fmt.Sprintf("DEVICE_CONFIG_RESPONSE.external.%s", name)
	if err := h.nc.Publish(responseChannel, responseData); err != nil {
		h.logger.Error("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Failed to send response to %s: %v", responseChannel, err))
	} else {
		h.logger.Debug("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Sent device config response to %s", responseChannel))
		h.logger.Info("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Successfully sent device config response to %s", name))
	}
}

// sendErrorResponse sends an error response
func (h *DeviceConfigHandler) sendErrorResponse(msg *nats.Msg, errorMsg string) {
	if msg.Reply != "" {
		response := fmt.Sprintf("ERROR: %s", errorMsg)
		if err := msg.Respond([]byte(response)); err != nil {
			h.logger.Error("DEVICE_CONFIG_HANDLER", fmt.Sprintf("Failed to send error response: %v", err))
		}
	}
}

// GetSubscription returns the current subscription (for testing)
func (h *DeviceConfigHandler) GetSubscription() *nats.Subscription {
	return h.subscription
}
