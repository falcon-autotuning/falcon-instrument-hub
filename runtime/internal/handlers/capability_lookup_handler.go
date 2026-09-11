package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
	"github.com/nats-io/nats.go"
)

const (
	CapabilityRequestType = "CAPABILITY_REQUEST"
	CapabilityPayloadType = "CAPABILITY_PAYLOAD"
	CapabilityHandlerName = "CAPABILITY_LOOKUP_HANDLER"
	CapabilityRequestSubj = "INSTRUMENTHUB.CAPABILITY_REQUEST"
	CapabilityPayloadSubj = "FALCON.CAPABILITY_PAYLOAD"
)

// CapabilityLookupHandler resolves a logical device capability to one hub-owned
// connected port.
type CapabilityLookupHandler struct {
	logger            *logging.Logger
	nc                *nats.Conn
	subscription      *nats.Subscription
	instrumentHandler *instrument.Handler
	config            *config.Config
}

func NewCapabilityLookupHandler(
	logger *logging.Logger,
	instrumentHandler *instrument.Handler,
	cfg *config.Config,
) *CapabilityLookupHandler {
	return &CapabilityLookupHandler{
		logger:            logger,
		instrumentHandler: instrumentHandler,
		config:            cfg,
	}
}

func (h *CapabilityLookupHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error

	h.subscription, err = nc.Subscribe(CapabilityRequestSubj, h.handleCapabilityRequest)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", CapabilityRequestSubj, err)
	}

	h.logger.Info(CapabilityHandlerName, "Subscribed to "+CapabilityRequestSubj)
	return nil
}

func (h *CapabilityLookupHandler) Unsubscribe() error {
	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
			return err
		}
		h.subscription = nil
	}

	h.logger.Info(CapabilityHandlerName, "Unsubscribed from "+CapabilityRequestSubj)
	return nil
}

func (h *CapabilityLookupHandler) handleCapabilityRequest(msg *nats.Msg) {
	h.logger.Debug(CapabilityHandlerName, fmt.Sprintf("Received %s : %s", CapabilityRequestType, string(msg.Data)))

	var request api.CapabilityRequest
	if err := json.Unmarshal(msg.Data, &request); err != nil {
		h.logger.Error(CapabilityHandlerName, fmt.Sprintf("Failed to unmarshal %s : %v", CapabilityRequestType, err))
		return
	}

	response := h.resolveCapability(request)
	responseSubject := CapabilityPayloadSubj
	if msg.Reply != "" {
		responseSubject = msg.Reply
	}
	if err := h.publishCapabilityPayload(responseSubject, response); err != nil {
		h.logger.Error(CapabilityHandlerName, fmt.Sprintf("Failed to publish %s : %v", CapabilityPayloadType, err))
		return
	}

	h.logger.Debug(CapabilityHandlerName, fmt.Sprintf("Sent %s", CapabilityPayloadType))
}

func (h *CapabilityLookupHandler) resolveCapability(request api.CapabilityRequest) api.CapabilityPayload {
	request.DeviceName = strings.TrimSpace(request.DeviceName)
	request.Capability = strings.TrimSpace(request.Capability)
	request.Role = strings.TrimSpace(request.Role)

	response := api.CapabilityPayload{
		Timestamp:  request.Timestamp,
		DeviceName: request.DeviceName,
		Capability: request.Capability,
		Role:       request.Role,
	}

	if request.DeviceName == "" {
		response.Error = "device_name is required"
		return response
	}
	if request.Capability == "" {
		response.Error = "capability is required"
		return response
	}
	if request.Role == "" {
		response.Error = "role is required"
		return response
	}

	cp, err := h.instrumentHandler.ResolveConnectedPort(request.DeviceName, request.Capability, request.Role)
	if err != nil {
		response.Error = err.Error()
		return response
	}

	var deviceCfg *config.DeviceConfig
	if h.config != nil {
		deviceCfg = h.config.DeviceConfig
	}
	portJSON, err := serializeConnectedPortToCerealJSON(cp, deviceCfg)
	if err != nil {
		response.Error = err.Error()
		return response
	}

	return capabilityPayloadFromConnectedPort(request.Timestamp, cp, portJSON)
}

func (h *CapabilityLookupHandler) publishCapabilityPayload(subject string, response api.CapabilityPayload) error {
	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", CapabilityPayloadType, err)
	}
	return h.nc.Publish(subject, responseData)
}

func capabilityPayloadFromConnectedPort(timestamp int64, cp ports.ConnectedPort, portJSON string) api.CapabilityPayload {
	return api.CapabilityPayload{
		Timestamp:      timestamp,
		Port:           portJSON,
		PortName:       string(cp.PortName),
		DeviceName:     cp.DeviceName,
		InstrumentName: cp.InstrumentName,
		ChannelName:    cp.ChannelName,
		ChannelIndex:   cp.ChannelIndex,
		Capability:     cp.IoTypeName,
		Role:           cp.Role,
		Unit:           cp.Unit,
	}
}
