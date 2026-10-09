package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	falconports "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/interpreter"
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
	logger       *logging.Logger
	nc           *nats.Conn
	subscription *nats.Subscription
	ports        *config.ConnectedPorts
}

// NewPortRequestHandler creates a new handler
func NewPortRequestHandler(
	logger *logging.Logger,
	ports *config.ConnectedPorts,
) *PortRequestHandler {
	return &PortRequestHandler{
		logger: logger,
		ports:  ports,
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
	encodedKnobs, err := serializePortsToCerealJSON(h.ports.Knobs)
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to serialize knobs: %v", err),
		)
		return
	}

	encodedMeters, err := serializePortsToCerealJSON(h.ports.Meters)
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to serialize meters: %v", err),
		)
		return
	}

	encodedSettings, err := serializePortsToCerealJSON(h.ports.Settings)
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to serialize settings: %v", err),
		)
		return
	}

	// Create response
	response := api.PortPayload{
		Knobs:     encodedKnobs,
		Meters:    encodedMeters,
		Settings:  encodedSettings,
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

func serializePortsToCerealJSON(connectedPorts []config.ConnectedPort) (string, error) {
	if len(connectedPorts) == 0 {
		portsHandle, err := falconports.NewEmpty()
		if err != nil {
			return "", fmt.Errorf("failed to create empty ports handle: %w", err)
		}

		jsonStr, err := portsHandle.ToJSON()
		if err != nil {
			return "", fmt.Errorf("failed to serialize empty ports: %w", err)
		}

		return jsonStr, nil
	}

	portHandles := make([]*instrumentport.Handle, 0, len(connectedPorts))
	for _, cp := range connectedPorts {
		conn := cp.Handle
		unit, err := interpreter.SymbolUnitFromString(cp.Unit)
		if err != nil {
			return "", fmt.Errorf("failed to create unit %q for port %s: %w", cp.Unit, cp.PortName, err)
		}
		realconn, err := conn.NewFalconConnection()
		if err != nil {
			return "", err
		}

		var h *instrumentport.Handle
		if cp.IsKnob() {
			h, err = instrumentport.NewKnob(string(cp.PortName), cp.InstrumentName, realconn, cp.InstrumentType, unit, cp.Description)
		} else {
			h, err = instrumentport.NewMeter(string(cp.PortName), cp.InstrumentName, realconn, cp.InstrumentType, unit, cp.Description)
		}
		if err != nil {
			return "", fmt.Errorf("failed to create instrument port for %s: %w", cp.PortName, err)
		}
		portHandles = append(portHandles, h)
	}

	portsHandle, err := falconports.New(portHandles)
	if err != nil {
		return "", fmt.Errorf("failed to create ports handle: %w", err)
	}

	jsonStr, err := portsHandle.ToJSON()
	if err != nil {
		return "", fmt.Errorf("failed to serialize ports: %w", err)
	}

	return jsonStr, nil
}
