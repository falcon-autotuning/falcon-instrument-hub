package deviceconfighandlers

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	handlerName                 = "DEVICE_CONFIG_HANDLER"
	deviceConfigRequestSubject  = "INSTRUMENTHUB.DEVICE_CONFIG_REQUEST"
	deviceConfigResponseSubject = "FALCON.DEVICE_CONFIG_RESPONSE"
)

// DeviceConfigHandler handles INSTRUMENTHUB.DEVICE_CONFIG_REQUEST messages.
type Handler struct {
	configJSON   string
	logger       *logging.Logger
	nc           *nats.Conn
	subscription *nats.Subscription
}

// NewDeviceConfigHandler creates a new device config handler
func NewDeviceConfigHandler(
	configJSON string,
	logger *logging.Logger,
) *Handler {
	return &Handler{
		configJSON: configJSON,
		logger:     logger,
	}
}

// Subscribe subscribes to INSTRUMENTHUB.DEVICE_CONFIG_REQUEST
func (h *Handler) Subscribe(nc *nats.Conn) error {
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

// Unsubscribe removes the device configuration request subscription.
func (h *Handler) Unsubscribe() error {
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
func (h *Handler) handleDeviceConfigRequest(msg *nats.Msg) {
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
func (h *Handler) parseRequest(
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

// sendDeviceConfigResponse sends the device config in cereal JSON format so that
// C++ Config::from_json_string can parse it directly.
func (h *Handler) sendDeviceConfigResponse() error {
	// Create the response
	response := api.DeviceConfigResponse{
		Response:  h.configJSON,
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
func (h *Handler) GetSubscription() *nats.Subscription {
	return h.subscription
}
