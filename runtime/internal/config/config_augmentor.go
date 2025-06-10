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
func (dc *DeviceConnection) ToJSON() map[string]string {
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
	Class          string `json:"__class__"`
	Module         string `json:"__module__"`
	DefaultName    string `json:"default_name,omitempty"`
	PseudoName     string `json:"pseudo_name"`
	InstrumentType string `json:"instrument_type"`
	Units          string `json:"units,omitempty"`
	Description    string `json:"description,omitempty"`

	// Additional fields added during augmentation
	DeviceConnection map[string]string `json:"device_connection,omitempty"`
	ConnectionName   string            `json:"connection_name,omitempty"`
	ConnectionType   string            `json:"connection_type,omitempty"`
}

// IsKnob returns true if this port is a knob
func (p *PortObject) IsKnob() bool {
	return p.Class == KnobClass
}

// IsMeter returns true if this port is a meter
func (p *PortObject) IsMeter() bool {
	return p.Class == MeterClass
}

// FromInterface unmarshals from interface{} (string or map) into PortObject
func (p *PortObject) FromInterface(portValue interface{}) error {
	if portStr, ok := portValue.(string); ok {
		return json.Unmarshal([]byte(portStr), p)
	} else if portMap, ok := portValue.(map[string]interface{}); ok {
		portBytes, err := json.Marshal(portMap)
		if err != nil {
			return err
		}
		return json.Unmarshal(portBytes, p)
	}
	return fmt.Errorf("unsupported port value type")
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
		err := processIndividualPort(
			portValue,
			index,
			nameMapping,
			wiremapKey,
			properties,
		)
		if err != nil {
			errors = append(errors, fmt.Sprintf("index %s: %v", index, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf(
			"failed to process ports: %s",
			strings.Join(errors, "; "),
		)
	}

	return nil
}

// processIndividualPort processes a single port at a specific index
func processIndividualPort(
	portValue string,
	index string,
	nameMapping map[string]*DeviceConnection,
	wiremapKey string,
	properties map[string]string,
) error {
	var portObj PortObject
	if err := portObj.FromInterface(portValue); err != nil {
		return fmt.Errorf("failed to unmarshal port: %w", err)
	}
	updatePortWithDeviceInfo(&portObj, wiremapKey, nameMapping)
	updatedPort, err := portObj.ToInterface()

	properties[index] = updatedPort
	if err != nil {
		return fmt.Errorf("failed to convert port back to interface: %w", err)
	}
	return nil
}

// updatePortWithDeviceInfo updates the port object with device connection info
// or fallback
func updatePortWithDeviceInfo(
	portObj *PortObject,
	lookupKey string,
	nameMapping map[string]*DeviceConnection,
) {
	if deviceConn, exists := nameMapping[lookupKey]; exists {
		// Check if this should be a meter (only Ohmics can be meters)
		if portObj.IsMeter() && deviceConn.ConnectionType != Ohmic {
			return // Skip non-ohmic meters
		}

		// Replace pseudo_name with the human-readable name
		portObj.PseudoName = deviceConn.Name

		// Add device connection information
		portObj.DeviceConnection = deviceConn.ToJSON()
		portObj.ConnectionName = deviceConn.Name
		portObj.ConnectionType = string(deviceConn.ConnectionType)
	} else {
		// No matching name found, use InstrumentType
		portObj.PseudoName = portObj.InstrumentType
	}
}
