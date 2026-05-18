package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	falconports "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
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
	encodedKnobs, err := serializePortsToCerealJSON(knobs, h.config.DeviceConfig)
	if err != nil {
		h.logger.Error(
			PortRequestHandlerName,
			fmt.Sprintf("Failed to serialize knobs: %v", err),
		)
		return
	}

	encodedMeters, err := serializePortsToCerealJSON(meters, h.config.DeviceConfig)
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

// serializePortsToCerealJSON converts a slice of ConnectedPort values into a
// serialized Ports object using the falcon-core C API.
//
// Each ConnectedPort is converted to an InstrumentPort using the typed
// constructors (NewKnob / NewMeter) rather than FromJSON, because
// InstrumentPort_from_json_string expects the C++ cereal wire format —
// not the Python-style __class__/__module__ shape that was previously
// assembled here by hand.
//
// The connection type for pseudo_name is resolved from DeviceConfig gate
// lists (ScreeningGates, PlungerGates, BarrierGates, ReservoirGates,
// Ohmics).  Any device name not found in those lists falls back to a
// generic PlungerGate connection.
func serializePortsToCerealJSON(connectedPorts []ports.ConnectedPort, deviceCfg *config.DeviceConfig) (string, error) {
	portHandles := make([]*instrumentport.Handle, 0, len(connectedPorts))
	for _, cp := range connectedPorts {
		conn, err := connectionFromDeviceName(cp.DeviceName, deviceCfg)
		if err != nil {
			return "", fmt.Errorf("failed to create connection for %s: %w", cp.DeviceName, err)
		}

		unit, err := symbolUnitFromString(cp.Unit)
		if err != nil {
			return "", fmt.Errorf("failed to create unit %q for port %s: %w", cp.Unit, cp.PortName, err)
		}

		var h *instrumentport.Handle
		if cp.IsKnob() {
			h, err = instrumentport.NewKnob(string(cp.PortName), conn, cp.InstrumentName, unit, cp.Description)
		} else {
			h, err = instrumentport.NewMeter(string(cp.PortName), conn, cp.InstrumentName, unit, cp.Description)
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

// connectionFromDeviceName creates a connection.Handle for the given device
// gate name by looking it up in the DeviceConfig gate lists.  The lookup
// order is: ScreeningGates → PlungerGates → BarrierGates → ReservoirGates →
// Ohmics.  If the name is not found, a PlungerGate is used as a default.
func connectionFromDeviceName(name string, cfg *config.DeviceConfig) (*connection.Handle, error) {
	if cfg != nil {
		if containsGateName(cfg.ScreeningGates, name) {
			return connection.NewScreeningGate(name)
		}
		if containsGateName(cfg.PlungerGates, name) {
			return connection.NewPlungerGate(name)
		}
		if containsGateName(cfg.BarrierGates, name) {
			return connection.NewBarrierGate(name)
		}
		if containsGateName(cfg.ReservoirGates, name) {
			return connection.NewReservoirGate(name)
		}
		if containsGateName(cfg.Ohmics, name) {
			return connection.NewOhmic(name)
		}
	}
	// Default: use PlungerGate for any device not explicitly categorised.
	return connection.NewPlungerGate(name)
}

// containsGateName reports whether semicolon-separated list contains name.
func containsGateName(list, name string) bool {
	for _, g := range strings.Split(list, ";") {
		if strings.TrimSpace(g) == name {
			return true
		}
	}
	return false
}

// symbolUnitFromString maps a unit symbol string (as written in instrument API
// YAML files) to the corresponding symbolunit.Handle.  The symbols follow the
// standard SI convention used by falcon-core's Constants.cpp.
func symbolUnitFromString(unit string) (*symbolunit.Handle, error) {
	switch unit {
	case "m":
		return symbolunit.NewMeter()
	case "kg":
		return symbolunit.NewKilogram()
	case "s":
		return symbolunit.NewSecond()
	case "A":
		return symbolunit.NewAmpere()
	case "K":
		return symbolunit.NewKelvin()
	case "mol":
		return symbolunit.NewMole()
	case "cd":
		return symbolunit.NewCandela()
	case "Hz":
		return symbolunit.NewHertz()
	case "N":
		return symbolunit.NewNewton()
	case "Pa":
		return symbolunit.NewPascal()
	case "J":
		return symbolunit.NewJoule()
	case "W":
		return symbolunit.NewWatt()
	case "C":
		return symbolunit.NewCoulomb()
	case "V":
		return symbolunit.NewVolt()
	case "F":
		return symbolunit.NewFarad()
	case "Ω", "ohm":
		return symbolunit.NewOhm()
	case "S":
		return symbolunit.NewSiemens()
	case "Wb":
		return symbolunit.NewWeber()
	case "T":
		return symbolunit.NewTesla()
	case "H":
		return symbolunit.NewHenry()
	case "min":
		return symbolunit.NewMinute()
	case "mV":
		return symbolunit.NewMillivolt()
	case "mA":
		return symbolunit.NewMilliampere()
	case "μA", "uA":
		return symbolunit.NewMicroampere()
	case "nA":
		return symbolunit.NewNanoampere()
	case "pA":
		return symbolunit.NewPicoampere()
	case "ns":
		return symbolunit.NewNanosecond()
	case "":
		return symbolunit.NewDimensionless()
	default:
		return symbolunit.NewDimensionless()
	}
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
