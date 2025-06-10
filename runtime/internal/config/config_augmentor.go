package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConnectionType represents the type of connection
type ConnectionType string

const (
	// Port class identifiers
	KnobClass  = "Knob"
	MeterClass = "Meter"
	PortClass  = "InstrumentPort"

	// Module path template
	FalconCoreModuleTemplate = "falcon_core.physics.device_structures.%s"
)

const (
	ScreeningGate ConnectionType = "ScreeningGate"
	BarrierGate   ConnectionType = "BarrierGate"
	ReservoirGate ConnectionType = "ReservoirGate"
	PlungerGate   ConnectionType = "PlungerGate"
	Ohmic         ConnectionType = "Ohmic"
)

// getModuleName returns the module name for a connection type
func (ct ConnectionType) getModuleName() string {
	switch ct {
	case ScreeningGate:
		return "screening_gate"
	case BarrierGate:
		return "barrier_gate"
	case ReservoirGate:
		return "reservoir_gate"
	case PlungerGate:
		return "plunger_gate"
	case Ohmic:
		return "ohmic"
	default:
		return ""
	}
}

// DeviceConnection represents a connection with its type and name
type DeviceConnection struct {
	Name           string         `json:"name"`
	ConnectionType ConnectionType `json:"connection_type"`
	ModuleName     string         `json:"module_name"`
}

// ToJSON returns the JSON representation for falcon_core
func (dc *DeviceConnection) toMap() map[string]string {
	return map[string]string{
		"__class__": string(dc.ConnectionType),
		"__module__": fmt.Sprintf(
			FalconCoreModuleTemplate,
			dc.ModuleName,
		),
		"name": dc.Name,
	}
}

// buildNameMapping creates a mapping from wire names to device connections
func BuildNameMapping(
	deviceConfig *DeviceConfig,
	wireMap *WireMap,
) (map[string]*DeviceConnection, error) {
	nameMapping := make(map[string]*DeviceConnection)

	// Handle nil inputs gracefully
	if deviceConfig == nil || wireMap == nil {
		return nameMapping, nil
	}

	// Build category mappings
	categories := map[ConnectionType]string{
		ScreeningGate: deviceConfig.ScreeningGates,
		BarrierGate:   deviceConfig.BarrierGates,
		ReservoirGate: deviceConfig.ReservoirGates,
		PlungerGate:   deviceConfig.PlungerGates,
		Ohmic:         deviceConfig.Ohmics,
	}

	// Process each category to build connection sets
	connectionCategories := make(map[string]ConnectionType)
	for connType, gateString := range categories {
		connections := parseConnections(gateString)
		for _, conn := range connections {
			connectionCategories[conn] = connType
		}
	}

	// Process wire map - only include values without "."
	for wireName, readableName := range *wireMap {
		if strings.Contains(readableName, ".") {
			continue // Skip names with dots
		}

		// Check if readable name exists in our device categories
		if connType, exists := connectionCategories[readableName]; exists {
			nameMapping[wireName] = &DeviceConnection{
				Name:           readableName,
				ConnectionType: connType,
				ModuleName:     connType.getModuleName(),
			}
		}
	}

	return nameMapping, nil
}

// parseConnections parses semicolon-delimited strings into a slice of
// connections
func parseConnections(connectionString string) []string {
	if connectionString == "" {
		return nil
	}

	connections := strings.Split(connectionString, ";")
	var result []string
	for _, conn := range connections {
		conn = strings.TrimSpace(conn)
		if conn != "" {
			result = append(result, conn)
		}
	}
	return result
}

// PortObject represents a generic port (knob or meter)
type PortObject struct {
	Class          string            `json:"__class__"`
	Module         string            `json:"__module__"`
	DefaultName    string            `json:"default_name"`
	PseudoName     map[string]string `json:"pseudo_name"`
	InstrumentType string            `json:"instrument_type"`
	Units          map[string]any    `json:"units"`
	Description    string            `json:"description"`
}

// IsKnob returns true if this port is a knob
func (p *PortObject) IsKnob() bool {
	return p.Class == KnobClass
}

// IsMeter returns true if this port is a meter
func (p *PortObject) IsMeter() bool {
	return p.Class == MeterClass
}

// IsPort return true if this port is a port
func (p *PortObject) IsPort() bool {
	return p.Class == KnobClass
}

// FromInterface unmarshals from interface{} (string or map) into PortObject
func (p *PortObject) FromInterface(portValue string) error {
	return json.Unmarshal([]byte(portValue), p)
}

// ToInterface converts PortObject back to interface{} (matching original
// format)
func (p *PortObject) ToInterface() (string, error) {
	portBytes, err := json.Marshal(p)
	return string(portBytes), err
}

// ProcessInstrumentPorts processes all ports for an instrument process and
// augments them
// with device connection information from the wire mapping.
//
// The function constructs lookup keys in the format "instrument_name.index" to
// find matching device connections in the nameMapping. If found, it replaces
// the port's pseudo_name with the human-readable device name. If not found, it
// uses the InstrumentType.
//
// Example: For instrument "dac1" with port index 0, it looks up "dac1.0" in
// nameMapping.
func ProcessInstrumentPorts(
	instrumentPorts map[string]map[string]string,
	nameMapping map[string]*DeviceConnection,
	instrumentName string,
) error {
	// Process each property type (knobs, meters, etc.)
	for propertyName, properties := range instrumentPorts {
		if err := processPortProperty(properties, nameMapping, instrumentName); err != nil {
			return fmt.Errorf(
				"failed to process %s properties: %w",
				propertyName,
				err,
			)
		}
	}

	return nil
}

// processPortProperty processes a single port property (like "knobs" or
// "meters")
func processPortProperty(
	properties map[string]string,
	nameMapping map[string]*DeviceConnection,
	instrumentName string,
) error {
	var errors []string

	for index, portValue := range properties {
		wiremapKey := fmt.Sprintf("%s.%s", instrumentName, index)
		deviceConn, exists := nameMapping[wiremapKey]
		if !exists {
			continue
		}
		updatePort, err := updatePortPsuedoName(
			portValue,
			deviceConn,
		)
		if err != nil {
			errors = append(errors, fmt.Sprintf("index %s: %v", index, err))
		}
		properties[index] = updatePort
	}

	if len(errors) > 0 {
		return fmt.Errorf(
			"failed to process ports: %s",
			strings.Join(errors, "; "),
		)
	}

	return nil
}

// updatePortPsuedoName processes a single port and upgrades it
func updatePortPsuedoName(
	portValue string,
	deviceConn *DeviceConnection,
) (string, error) {
	var portObj PortObject
	if err := portObj.FromInterface(portValue); err != nil {
		return "", fmt.Errorf("failed to unmarshal port: %w", err)
	}
	if portObj.IsMeter() && deviceConn.ConnectionType != Ohmic {
		return "", fmt.Errorf("found non-ohmic meter: %s", portObj.PseudoName)
	}
	portObj.PseudoName = deviceConn.toMap()
	updatedPort, err := portObj.ToInterface()
	if err != nil {
		return updatedPort, fmt.Errorf(
			"failed to convert port back to interface: %w",
			err,
		)
	}
	return updatedPort, nil
}
