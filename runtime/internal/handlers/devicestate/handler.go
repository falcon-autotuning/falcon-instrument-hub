package devicestatehandler

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/devicestate"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	handlerName                = "DEVICE_STATE_HANDLER"
	deviceStateRequestSubject  = "INSTRUMENTHUB.DEVICE_STATE_REQUEST"
	deviceStateResponseSubject = "FALCON.DEVICE_STATE_RESPONSE"
)

// Handler handles INSTRUMENTHUB.DEVICE_STATE_REQUEST messages.
type Handler struct {
	logger       *logging.Logger
	nc           *nats.Conn
	subscription *nats.Subscription
}

// NewHandler creates a new device state handler
func NewHandler(
	logger *logging.Logger,
) *Handler {
	return &Handler{
		logger: logger,
	}
}

// Subscribe subscribes to INSTRUMENTHUB.DEVICE_CONFIG_REQUEST
func (h *Handler) Subscribe(nc *nats.Conn) error {
	h.nc = nc

	sub, err := nc.Subscribe(
		deviceStateRequestSubject,
		h.handleDeviceStateRequest,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to %s: %w",
			deviceStateRequestSubject,
			err,
		)
	}
	h.subscription = sub

	h.logger.Info(
		handlerName,
		fmt.Sprintf("Subscribed to %s channels", deviceStateRequestSubject),
	)
	log.Printf(
		"%s subscribed to %s channels",
		handlerName,
		deviceStateRequestSubject,
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
				deviceStateRequestSubject,
			),
		)
		h.subscription = nil
	}
	return nil
}

// handleDeviceStateRequest processes incoming INSTRUMENTHUB.DEVICE_STATE_REQUEST messages
func (h *Handler) handleDeviceStateRequest(msg *nats.Msg) {
	rawData := msg.Data

	h.logger.Debug(
		handlerName,
		fmt.Sprintf("Received request on %s: %s", deviceStateRequestSubject, string(rawData)),
	)

	var deviceStateReq api.DeviceStateRequest
	if err := h.parseRequest(rawData, &deviceStateReq); err != nil {
		h.logger.Error(handlerName, err.Error())
		return
	}

	if err := h.sendDeviceStateResponse(); err != nil {
		h.logger.Error(handlerName, err.Error())
	}
}

// parseRequest unmarshals the request data
func (h *Handler) parseRequest(
	rawData []byte,
	deviceStateReq *api.DeviceStateRequest,
) error {
	if err := json.Unmarshal(rawData, deviceStateReq); err != nil {
		return fmt.Errorf(
			"failed to decode device config request JSON: %v",
			err,
		)
	}
	return nil
}

// sendDeviceStateResponse sends the device config in cereal JSON format so that
// C++ Config::from_json_string can parse it directly.
func (h *Handler) sendDeviceStateResponse() error {
	states, err := devicestate.Manager().Snapshot().NewFalconDeviceVoltageStates()
	if err != nil {
		return err
	}
	jsonStates, err := states.ToJSON()
	if err != nil {
		return err
	}
	// Create the response
	response := api.DeviceStateResponse{
		Response:  jsonStates,
		Timestamp: time.Now().UnixMicro(),
	}

	// Marshal the response
	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal device state response: %v", err)
	}

	if err := h.nc.Publish(deviceStateResponseSubject, responseData); err != nil {
		return fmt.Errorf(
			"failed to send response to %s: %v",
			deviceStateResponseSubject,
			err,
		)
	}

	h.logger.Debug(
		handlerName,
		fmt.Sprintf("Sent device state response to %s", deviceStateResponseSubject),
	)
	h.logger.Info(
		handlerName,
		"Successfully sent device state response",
	)
	return nil
}

// GetSubscription returns the current subscription (for testing)
func (h *Handler) GetSubscription() *nats.Subscription {
	return h.subscription
}
