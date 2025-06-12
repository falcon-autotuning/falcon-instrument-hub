package instrument

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

// PortProcessor handles processing and augmentation of instrument ports
type PortProcessor struct {
	logger            *logging.Logger
	Log               *LogWrapper // Log wrapper for structured logging
	nameMapping       map[config.InstrumentConnection]*PsuedoName
	cachedPortConfigs map[JsonPort]PortOptions // Cache for port configurations
	portConfigsCached bool                     // Flag to track if cache is valid
	cacheMutex        sync.RWMutex             // Mutex for cache access
}

// NewPortProcessor creates a new port processor
func NewPortProcessor(
	logger *logging.Logger,
	log *LogWrapper,
	cfg *config.Config,
) (*PortProcessor, error) {
	// Build name mapping once during initialization
	nameMapping, err := buildNameMapping(cfg.DeviceConfig, cfg.WireMap)
	if err != nil {
		logger.Error(
			HandlerName,
			fmt.Sprintf("Failed to build name mapping: %v", err),
		)
		nameMapping = make(
			map[config.InstrumentConnection]*PsuedoName,
		)
	}

	return &PortProcessor{
		logger:            logger,
		Log:               log,
		nameMapping:       nameMapping,
		cachedPortConfigs: make(map[JsonPort]PortOptions),
		portConfigsCached: false,
	}, nil
}

// buildNameMapping creates a mapping from wire names to device connections
func buildNameMapping(
	deviceConfig *config.DeviceConfig,
	wireMap *config.WireMap,
) (map[config.InstrumentConnection]*PsuedoName, error) {
	nameMapping := make(map[config.InstrumentConnection]*PsuedoName)

	// Handle nil inputs gracefully
	if deviceConfig == nil || wireMap == nil {
		return nameMapping, nil
	}

	// Build category mappings
	categories := map[connectionType]string{
		ScreeningGate: deviceConfig.ScreeningGates,
		BarrierGate:   deviceConfig.BarrierGates,
		ReservoirGate: deviceConfig.ReservoirGates,
		PlungerGate:   deviceConfig.PlungerGates,
		Ohmic:         deviceConfig.Ohmics,
	}

	// Process each category to build connection sets
	connectionCategories := make(map[config.InstrumentConnection]connectionType)
	for connType, gateString := range categories {
		connections := config.ParseConnections(gateString)
		for _, conn := range connections {
			connectionCategories[conn] = connType
		}
	}

	// Process wire map - only include values without "."
	for wireName, wireConnection := range *wireMap {
		if wireConnection.Contains(".") {
			continue // skip names with dots
		}

		// Check if readable name exists in our device categories
		if connType, exists := connectionCategories[wireConnection]; exists {
			nameMapping[wireName] = &PsuedoName{
				Name:   wireConnection,
				Class:  connType,
				Module: connectionToModule[connType],
			}
		}
	}

	return nameMapping, nil
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
func (pp *PortProcessor) ProcessInstrumentPorts(
	ports propertyIndexedPorts,
	name Name,
) error {
	// Process each property type (knobs, meters, etc.)
	for propertyName, properties := range ports {
		if err := processPortProperty(properties, pp.nameMapping, name); err != nil {
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
	properties map[Index]JsonPort,
	nameMapping map[config.InstrumentConnection]*PsuedoName,
	instrumentName Name,
) error {
	var errors []string

	for index, portValue := range properties {
		wiremapKey := config.InstrumentConnection(
			fmt.Sprintf("%s.%s", instrumentName, index),
		)
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
	port JsonPort,
	deviceConn *PsuedoName,
) (JsonPort, error) {
	var portObj PortObject
	if err := portObj.FromInterface(port); err != nil {
		return "", fmt.Errorf("failed to unmarshal port: %w", err)
	}
	if portObj.IsMeter() && deviceConn.Class != Ohmic {
		return "", fmt.Errorf("found non-ohmic meter: %s", portObj.PseudoName)
	}
	portObj.PseudoName = *deviceConn
	updatedPort, err := portObj.ToInterface()
	if err != nil {
		return updatedPort, fmt.Errorf(
			"failed to convert port back to interface: %w",
			err,
		)
	}
	return updatedPort, nil
}

// CollectPortProperties queries instrument ports and categorizes them into
// knobs and meters
func (pp *PortProcessor) CollectPortProperties(
	instruments map[Name]*InstrumentProcess,
) (knobs, meters []JsonPort) {
	const (
		KnobIdentifier  = "Knob"
		MeterIdentifier = "Meter"
	)

	pp.Log.Debug(
		"Collecting port properties from %d instruments",
		len(instruments),
	)

	// Collect ports from all active instruments (processing should already be
	// done)
	for _, instrument := range instruments {
		if !instrument.Initialized || instrument.Ports == nil {
			continue
		}

		for _, portMap := range instrument.Ports {
			for _, value := range portMap {

				if value.Contains(KnobIdentifier) {
					knobs = append(knobs, value)
				}
				if value.Contains(MeterIdentifier) {
					meters = append(meters, value)
				}
			}
		}
	}

	pp.Log.Debug(
		"Collected %d knobs and %d meters",
		len(knobs),
		len(meters),
	)
	// pp.Log.Debug(
	// 	"The collections are knobs: %v and meters: %v",
	// 	knobs,
	// 	meters,
	// )
	//
	return knobs, meters
}

// PortConfiguration represents the inverted mapping for a port
type PortOptions struct {
	Instrument Name           `json:"instrument"`
	Properties []PropertyName `json:"properties"`
	Index      Index          `json:"index"`
}

// BuildConfigurations creates the configuration mapping by collecting and
// inverting port mappings
func (pp *PortProcessor) BuildConfigurations(
	instruments map[Name]*InstrumentProcess,
) (map[JsonPort]map[PropertyName]PortConfiguration, error) {
	// Get cached port configurations (Step 1 + Step 2)
	var portConfigurations map[JsonPort]PortOptions
	if cached, exists := pp.getCachedPortConfigurations(); exists {
		portConfigurations = cached
	} else {
		// Build and cache Steps 1 and 2 if not available
		portConfigurations = pp.buildAndCachePortConfigurations(instruments)
	}

	// Step 3: Build final configuration mapping from port names to instrument
	// configurations
	finalConfigurations := pp.BuildFinalConfigurations(
		portConfigurations,
		instruments,
	)

	return finalConfigurations, nil
}

// CollectInstrumentPorts collects all ports organized by instrument →
// property → index → port data
func (pp *PortProcessor) CollectInstrumentPorts(
	instruments map[Name]*InstrumentProcess,
) instrumentIndexedPorts {
	outs := make(instrumentIndexedPorts)
	for instrumentName, instrumentProcess := range instruments {
		if !instrumentProcess.Initialized || instrumentProcess.Ports == nil {
			continue
		}
		outs[instrumentName] = make(propertyIndexedPorts)
		for name, propertyContents := range instrumentProcess.Ports {
			outs[instrumentName][name] = make(
				map[Index]JsonPort,
			)
			maps.Copy(
				outs[instrumentName][name],
				propertyContents,
			)
		}
	}
	return outs
}

// InvertPortMappings inverts the mapping to index by port name, handling
// collisions
func (pp *PortProcessor) InvertPortMappings(
	instrumentPorts instrumentIndexedPorts,
) map[JsonPort]PortOptions {
	outs := make(map[JsonPort]PortOptions)

	pp.Log.Debug(
		"Starting port mapping inversion with %d instruments",
		len(instrumentPorts),
	)

	for instrumentName, properties := range instrumentPorts {
		pp.Log.Debug(
			"Processing instrument %s with %d properties",
			instrumentName,
			len(properties),
		)

		for property, indices := range properties {
			for index, port := range indices {
				if existingConfig, exists := outs[port]; !exists {
					outs[port] = PortOptions{
						Instrument: instrumentName,
						Properties: []PropertyName{property},
						Index:      index,
					}
				} else {
					existingConfig.Properties = append(
						existingConfig.Properties,
						property,
					)
					outs[port] = existingConfig
				}
			}
		}
	}
	pp.logger.Debug(
		HandlerName,
		fmt.Sprintf(
			"Port mapping inversion complete. Found %d port configurations",
			len(outs),
		),
	)

	return outs
}

// BuildFinalConfigurations builds final configuration mapping from port names
// to instrument configurations
func (pp *PortProcessor) BuildFinalConfigurations(
	portConfigurations map[JsonPort]PortOptions,
	instruments map[Name]*InstrumentProcess,
) map[JsonPort]map[PropertyName]PortConfiguration {
	outs := make(map[JsonPort]map[PropertyName]PortConfiguration)

	for portName, portConfig := range portConfigurations {
		// Get the instrument process to access its configuration
		if instrumentProcess, exists := instruments[portConfig.Instrument]; exists &&
			instrumentProcess.Configuration != nil {

			outs[portName] = make(map[PropertyName]PortConfiguration)

			// For each property in this port configuration
			for _, property := range portConfig.Properties {
				// Get the configuration value for this property at the
				// specific index
				if propertyConfig, exists := instrumentProcess.Configuration[property]; exists {
					if configValue, exists := propertyConfig[portConfig.Index]; exists {
						outs[portName][property] = configValue
					}
				}
			}
		}
	}

	return outs
}

// BuildPortConfigurations builds the port configurations mapping (Step 1 + Step
// 2)
// Returns a mapping from port names to their configuration details
func (pp *PortProcessor) BuildPortConfigurations(
	instruments map[Name]*InstrumentProcess,
) (map[JsonPort]PortOptions, error) {
	// Check cache first
	if cached, exists := pp.getCachedPortConfigurations(); exists {
		return cached, nil
	}

	// Build and cache if not available
	portConfigurations := pp.buildAndCachePortConfigurations(instruments)
	return portConfigurations, nil
}

// GetPortConfiguration finds the configuration for a specific port
func (pp *PortProcessor) GetPortConfiguration(
	name JsonPort,
	instruments map[Name]*InstrumentProcess,
) (*PortOptions, error) {
	// Check cache first
	var portConfigurations map[JsonPort]PortOptions
	if cached, exists := pp.getCachedPortConfigurations(); exists {
		portConfigurations = cached
	} else {
		// Build and cache if not available
		pp.Log.Debug(
			"Building the cached port configurations",
		)
		portConfigurations = pp.buildAndCachePortConfigurations(instruments)
	}

	// need to get compact json form:
	var data PortObject
	if err := json.Unmarshal([]byte(name), &data); err != nil {
		pp.Log.Error("Failed to unmarshal JsonPort: %v", err)
	}
	compactJSON, err := json.Marshal(data)
	if err != nil {
		pp.Log.Error("Failed to marshal JsonPort: %v", err)
	}
	compactPortName := JsonPort(compactJSON)
	pp.Log.Debug(
		"New compact port name: %s",
		compactPortName,
	)

	// now check if the port exists
	if portConfig, exists := portConfigurations[compactPortName]; exists {
		return &portConfig, nil
	}
	ports := make([]JsonPort, 0, len(portConfigurations))
	for port := range portConfigurations {
		ports = append(ports, port)
	}
	pp.Log.Error(
		"The current ports in the configuration are: %v",
		ports,
	)

	return nil, fmt.Errorf("port %s not found in configurations", name)
}

// InvalidatePortConfigCache invalidates the cached port configurations
// This should be called when instruments are added, removed, or reconfigured
func (pp *PortProcessor) InvalidatePortConfigCache() {
	pp.cacheMutex.Lock()
	defer pp.cacheMutex.Unlock()

	pp.portConfigsCached = false
	pp.cachedPortConfigs = make(map[JsonPort]PortOptions)
}

// buildAndCachePortConfigurations builds port configurations and caches them
func (pp *PortProcessor) buildAndCachePortConfigurations(
	instruments map[Name]*InstrumentProcess,
) map[JsonPort]PortOptions {
	// Step 1: Collect all ports organized by instrument → property → index
	// → port data
	instrumentPorts := pp.CollectInstrumentPorts(instruments)

	// Step 2: Invert the mapping - index by port name, handling collisions
	portConfigurations := pp.InvertPortMappings(instrumentPorts)

	// Cache the results
	pp.cacheMutex.Lock()
	pp.cachedPortConfigs = portConfigurations
	pp.portConfigsCached = true
	pp.cacheMutex.Unlock()

	return portConfigurations
}

// getCachedPortConfigurations returns cached port configurations if available
func (pp *PortProcessor) getCachedPortConfigurations() (map[JsonPort]PortOptions, bool) {
	pp.cacheMutex.RLock()
	defer pp.cacheMutex.RUnlock()

	if pp.portConfigsCached {
		// Return a copy to prevent external modification

		result := make(map[JsonPort]PortOptions)
		maps.Copy(result, pp.cachedPortConfigs)
		return result, true
	}

	return nil, false
}
