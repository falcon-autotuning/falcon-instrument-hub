package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	handlerName                 = "DEVICE_CONFIG_HANDLER"
	deviceConfigRequestSubject  = "INSTRUMENTHUB.DEVICE_CONFIG_REQUEST"
	deviceConfigResponseSubject = "FALCON.DEVICE_CONFIG_RESPONSE"
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

// Subscribe subscribes to INSTRUMENTHUB.DEVICE_CONFIG_REQUEST
func (h *DeviceConfigHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc

	sub, err := nc.Subscribe(
		deviceConfigRequestSubject,
		h.handleDeviceConfigRequest,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to %s: %w",
			deviceConfigRequestSubject,
			err,
		)
	}
	h.subscription = sub

	h.logger.Info(
		handlerName,
		fmt.Sprintf("Subscribed to %s channels", deviceConfigRequestSubject),
	)
	log.Printf(
		"%s subscribed to %s channels",
		handlerName,
		deviceConfigRequestSubject,
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
				deviceConfigRequestSubject,
			),
		)
		h.subscription = nil
	}
	return nil
}

// handleDeviceConfigRequest processes incoming INSTRUMENTHUB.DEVICE_CONFIG_REQUEST messages
func (h *DeviceConfigHandler) handleDeviceConfigRequest(msg *nats.Msg) {
	rawData := msg.Data

	h.logger.Debug(
		handlerName,
		fmt.Sprintf("Received request on %s: %s", deviceConfigRequestSubject, string(rawData)),
	)

	var deviceConfigReq api.DeviceConfigRequest
	if err := h.parseRequest(rawData, &deviceConfigReq); err != nil {
		h.logger.Error(handlerName, err.Error())
		return
	}

	if err := h.sendDeviceConfigResponse(); err != nil {
		h.logger.Error(handlerName, err.Error())
	}
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

// sendDeviceConfigResponse serializes the in-memory DeviceConfig to JSON and
// sends it as the response payload.
func (h *DeviceConfigHandler) sendDeviceConfigResponse() error {
	deviceConfigBytes, err := json.Marshal(h.config.DeviceConfig)
	if err != nil {
		return fmt.Errorf("failed to serialize device config to JSON: %v", err)
	}

	// Create the response
	response := api.DeviceConfigResponse{
		Response:  string(deviceConfigBytes),
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal the response
	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal device config response: %v", err)
	}

	// Send response to FALCON.DEVICE_CONFIG_RESPONSE
	if err := h.nc.Publish(deviceConfigResponseSubject, responseData); err != nil {
		return fmt.Errorf(
			"failed to send response to %s: %v",
			deviceConfigResponseSubject,
			err,
		)
	}

	h.logger.Debug(
		handlerName,
		fmt.Sprintf("Sent device config response to %s", deviceConfigResponseSubject),
	)
	h.logger.Info(
		handlerName,
		"Successfully sent device config response",
	)
	return nil
}

// GetSubscription returns the current subscription (for testing)
func (h *DeviceConfigHandler) GetSubscription() *nats.Subscription {
	return h.subscription
}
