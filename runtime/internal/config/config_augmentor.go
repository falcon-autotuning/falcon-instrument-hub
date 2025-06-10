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
	println("Built name mapping:", nameMapping)

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
	Class          string      `json:"__class__"`
	Module         string      `json:"__module__"`
	DefaultName    interface{} `json:"default_name,omitempty"`
	PseudoName     string      `json:"pseudo_name"`
	InstrumentType string      `json:"instrument_type"`
	Units          interface{} `json:"units,omitempty"`
	Description    interface{} `json:"description,omitempty"`

	// Additional fields added during augmentation
	DeviceConnection map[string]interface{} `json:"device_connection,omitempty"`
	ConnectionName   string                 `json:"connection_name,omitempty"`
	ConnectionType   string                 `json:"connection_type,omitempty"`
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
func (p *PortObject) ToInterface(originalWasString bool) (any, error) {
	portBytes, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	if originalWasString {
		return string(portBytes), nil
	} else {
		var portMap map[string]any
		if err := json.Unmarshal(portBytes, &portMap); err != nil {
			return nil, err
		}
		return portMap, nil
	}
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
	instrumentPorts map[string]any,
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
	properties any,
	nameMapping map[string]*DeviceConnection,
	instrumentName string,
) error {
	propertiesMap, ok := properties.(map[int64]any)
	if !ok {
		return nil // Skip non-map properties
	}

	for index, portValue := range propertiesMap {
		if err := processIndividualPort(portValue, index, nameMapping, instrumentName, propertiesMap); err != nil {
			// Log error but continue processing other ports
			continue
		}
	}
	return nil
}

// processIndividualPort processes a single port at a specific index
func processIndividualPort(
	portValue any,
	index int64,
	nameMapping map[string]*DeviceConnection,
	instrumentName string,
	propertiesMap map[int64]any,
) error {
	// Create and unmarshal port object
	var portObj PortObject
	if err := portObj.FromInterface(portValue); err != nil {
		return fmt.Errorf("failed to unmarshal port: %w", err)
	}

	// Construct the lookup key: instrument_name.index
	lookupKey := fmt.Sprintf("%s.%d", instrumentName, index)

	// Update port with device connection or fallback name
	updatePortWithDeviceInfo(&portObj, lookupKey, nameMapping)

	// Convert back to original format and store
	return updatePortInMap(&portObj, portValue, index, propertiesMap)
}

// updatePortInMap converts the updated port object back to its original format
// and stores it
func updatePortInMap(
	portObj *PortObject,
	originalPortValue any,
	index int64,
	propertiesMap map[int64]any,
) error {
	// Determine if original was string format
	originalWasString := false
	if _, ok := originalPortValue.(string); ok {
		originalWasString = true
	}

	// Convert back to original format and store
	updatedPort, err := portObj.ToInterface(originalWasString)
	if err != nil {
		return fmt.Errorf("failed to convert port back to interface: %w", err)
	}

	propertiesMap[index] = updatedPort
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
