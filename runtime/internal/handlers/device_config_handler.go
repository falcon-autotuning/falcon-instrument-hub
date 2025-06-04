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

const (
	handlerName                 = "DEVICE_CONFIG_HANDLER"
	deviceConfigRequestSegment  = "DEVICE_CONFIG_REQUEST"
	deviceConfigResponseSegment = "DEVICE_CONFIG_RESPONSE"
	externalSegment             = "external"
	deviceConfigRequestPrefix   = deviceConfigRequestSegment + "." + externalSegment
	deviceConfigRequestPattern  = deviceConfigRequestPrefix + ".*"
	deviceConfigResponsePrefix  = deviceConfigResponseSegment + "." + externalSegment
)

// DeviceConfigHandler handles DEVICE_CONFIG_REQUEST.external.<name> messages
type DeviceConfigHandler struct {
	config       *config.Config
	logger       *logging.Logger
	nc           *nats.Conn
	subscription *nats.Subscription
}

// NewDeviceConfigHandler creates a new device config handler
func NewDeviceConfigHandler(
	cfg *config.Config,
	logger *logging.Logger,
) *DeviceConfigHandler {
	return &DeviceConfigHandler{
		config: cfg,
		logger: logger,
	}
}

// Subscribe subscribes to DEVICE_CONFIG_REQUEST.external.* channels
func (h *DeviceConfigHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc

	// Subscribe to DEVICE_CONFIG_REQUEST.external.*
	sub, err := nc.Subscribe(
		deviceConfigRequestPattern,
		h.handleDeviceConfigRequest,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to %s: %w",
			deviceConfigRequestPattern,
			err,
		)
	}
	h.subscription = sub

	h.logger.Info(
		handlerName,
		fmt.Sprintf("Subscribed to %s channels", deviceConfigRequestPattern),
	)
	log.Printf(
		"%s subscribed to %s channels",
		handlerName,
		deviceConfigRequestPattern,
	)

	return nil
}

// Unsubscribe unsubscribes from DEVICE_CONFIG_REQUEST.external.* channels
func (h *DeviceConfigHandler) Unsubscribe() error {
	if h.subscription != nil {
		err := h.subscription.Unsubscribe()
		if err != nil {
			h.logger.Error(
				handlerName,
				fmt.Sprintf("Failed to unsubscribe: %v", err),
			)
			return err
		}
		h.logger.Info(
			handlerName,
			fmt.Sprintf(
				"Unsubscribed from %s channels",
				deviceConfigRequestPattern,
			),
		)
		h.subscription = nil
	}
	return nil
}

// handleDeviceConfigRequest processes incoming DEVICE_CONFIG_REQUEST messages
func (h *DeviceConfigHandler) handleDeviceConfigRequest(msg *nats.Msg) {
	channel := msg.Subject
	rawData := msg.Data

	h.logger.Debug(
		handlerName,
		fmt.Sprintf("Received request on %s: %s", channel, string(rawData)),
	)

	name, err := h.parseChannelName(channel)
	if err != nil {
		h.logger.Error(handlerName, err.Error())
		h.sendErrorResponse(msg, "Invalid channel format")
		return
	}

	var deviceConfigReq api.DeviceConfigRequest
	if err := h.parseRequest(rawData, &deviceConfigReq); err != nil {
		h.logger.Error(handlerName, err.Error())
		h.sendErrorResponse(
			msg,
			fmt.Sprintf("Failed to unmarshal request: %v", err),
		)
		return
	}

	if err := h.sendDeviceConfigResponse(name); err != nil {
		h.logger.Error(handlerName, err.Error())
		h.sendErrorResponse(msg, "Failed to send device config")
	}
}

// parseChannelName extracts and validates the name from the channel
func (h *DeviceConfigHandler) parseChannelName(channel string) (string, error) {
	parts := strings.Split(channel, ".")
	if len(parts) != 3 || parts[0] != deviceConfigRequestSegment ||
		parts[1] != externalSegment {
		return "", fmt.Errorf(
			"invalid channel format %s, expected %s.<name>",
			channel,
			deviceConfigRequestPrefix,
		)
	}
	return parts[2], nil
}

// parseRequest unmarshals the request data
func (h *DeviceConfigHandler) parseRequest(
	rawData []byte,
	deviceConfigReq *api.DeviceConfigRequest,
) error {
	if err := json.Unmarshal(rawData, deviceConfigReq); err != nil {
		return fmt.Errorf(
			"failed to decode device config request JSON: %v",
			err,
		)
	}
	return nil
}

// sendDeviceConfigResponse creates and sends the device config response
func (h *DeviceConfigHandler) sendDeviceConfigResponse(name string) error {
	// Marshal the device config to JSON
	deviceConfigJSON, err := json.Marshal(h.config.DeviceConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal device config: %v", err)
	}

	// Create the response
	response := api.DeviceConfigResponse{
		Response:  string(deviceConfigJSON),
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal the response
	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal device config response: %v", err)
	}

	// Send response back to DEVICE_CONFIG_RESPONSE.external.<name>
	responseChannel := fmt.Sprintf("%s.%s", deviceConfigResponsePrefix, name)
	if err := h.nc.Publish(responseChannel, responseData); err != nil {
		return fmt.Errorf(
			"failed to send response to %s: %v",
			responseChannel,
			err,
		)
	}

	h.logger.Debug(
		handlerName,
		fmt.Sprintf("Sent device config response to %s", responseChannel),
	)
	h.logger.Info(
		handlerName,
		fmt.Sprintf("Successfully sent device config response to %s", name),
	)
	return nil
}

// sendErrorResponse sends an error response
func (h *DeviceConfigHandler) sendErrorResponse(
	msg *nats.Msg,
	errorMsg string,
) {
	if msg.Reply != "" {
		response := fmt.Sprintf("ERROR: %s", errorMsg)
		if err := msg.Respond([]byte(response)); err != nil {
			h.logger.Error(
				handlerName,
				fmt.Sprintf("Failed to send error response: %v", err),
			)
		}
	}
}

// GetSubscription returns the current subscription (for testing)
func (h *DeviceConfigHandler) GetSubscription() *nats.Subscription {
	return h.subscription
}
