package instrument

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

// PortProcessor handles processing and augmentation of instrument ports
type PortProcessor struct {
	logger            *logging.Logger
	Log               *LogWrapper // Log wrapper for structured logging
	nameMapping       map[string]*config.DeviceConnection
	cachedPortConfigs map[string]any // Cache for port configurations
	portConfigsCached bool           // Flag to track if cache is valid
	cacheMutex        sync.RWMutex   // Mutex for cache access
}

// NewPortProcessor creates a new port processor
func NewPortProcessor(
	logger *logging.Logger,
	log *LogWrapper,
	cfg *config.Config,
) (*PortProcessor, error) {
	// Build name mapping once during initialization
	nameMapping, err := config.BuildNameMapping(cfg.DeviceConfig, cfg.WireMap)
	if err != nil {
		logger.Error(
			HandlerName,
			fmt.Sprintf("Failed to build name mapping: %v", err),
		)
		nameMapping = make(
			map[string]*config.DeviceConnection,
		) // Use empty mapping
	}

	return &PortProcessor{
		logger:            logger,
		Log:               log,
		nameMapping:       nameMapping,
		cachedPortConfigs: make(map[string]any),
		portConfigsCached: false,
	}, nil
}

// ProcessInstrumentPorts processes and augments ports for a specific instrument
// This should be called immediately after an instrument is loaded and
// initialized
func (pp *PortProcessor) ProcessInstrumentPorts(
	instrumentName string,
	instrumentPorts map[string]any,
) error {
	if instrumentPorts == nil {
		return fmt.Errorf("instrument %s has no ports", instrumentName)
	}

	// Augment the ports with device connection information using pre-built
	// mapping
	if err := config.ProcessInstrumentPorts(instrumentPorts, pp.nameMapping, instrumentName); err != nil {
		pp.Log.Error(
			"Failed to augment ports for instrument %s: %v",
			instrumentName,
			err,
		)
		return err
	}

	pp.Log.Debug(
		"Successfully processed ports for instrument %s",
		instrumentName,
	)

	return nil
}

// CollectPortProperties queries instrument ports and categorizes them into
// knobs and meters
func (pp *PortProcessor) CollectPortProperties(
	instruments map[string]*InstrumentProcess,
) (knobs, meters []string) {
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

		for _, innerMap := range instrument.Ports {
			// Type assert innerMap to map[int64]any or similar
			pp.Log.Debug(
				"currently checking a range of port value %v",
				innerMap,
			)
			if portMap, ok := innerMap.(map[string]any); ok {
				pp.Log.Debug(
					"indeed it is a map string any %v",
					portMap,
				)
				for _, value := range portMap {
					if valueStr, ok := value.(string); ok {
						pp.Log.Debug(
							"currently checking a possible port value %s",
							valueStr,
						)

						if strings.Contains(valueStr, KnobIdentifier) {
							knobs = append(knobs, valueStr)
						}
						if strings.Contains(valueStr, MeterIdentifier) {
							meters = append(meters, valueStr)
						}
					}
				}
			}
		}
	}

	pp.logger.Debug(
		HandlerName,
		fmt.Sprintf(
			"Collected %d knobs and %d meters",
			len(knobs),
			len(meters),
		),
	)

	return knobs, meters
}

// PortConfiguration represents the inverted mapping for a port
type PortConfiguration struct {
	Instrument string   `json:"instrument"`
	Properties []string `json:"properties"`
	Index      int64    `json:"index"`
}

// BuildConfigurations creates the configuration mapping by collecting and
// inverting port mappings
func (pp *PortProcessor) BuildConfigurations(
	instruments map[string]*InstrumentProcess,
) (map[string]map[string]any, error) {
	// Get cached port configurations (Step 1 + Step 2)
	var portConfigurations map[string]any
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
	instruments map[string]*InstrumentProcess,
) map[string]map[string]map[int64]any {
	instrumentPorts := make(map[string]map[string]map[int64]any)

	for instrumentName, instrumentProcess := range instruments {
		if !instrumentProcess.Initialized || instrumentProcess.Ports == nil {
			continue
		}

		instrumentPorts[instrumentName] = make(map[string]map[int64]any)

		// Copy the ports structure
		for propertyName, propertyData := range instrumentProcess.Ports {
			if portMap, ok := propertyData.(map[string]any); ok {
				instrumentPorts[instrumentName][propertyName] = make(
					map[int64]any,
				)
				for index, portValue := range portMap {
					countIndex, err := strconv.ParseInt(index, 10, 64)
					if err != nil {
						pp.Log.Error(
							"Could not convert index %s to int64: %v",
							index,
							err,
						)
					} else {
						pp.Log.Debug(
							"Storing port value for instrument %s, property %s, index %d: %v",
							instrumentName,
							propertyName,
							countIndex,
							portValue,
						)
						instrumentPorts[instrumentName][propertyName][countIndex] = portValue
					}
				}
			}
		}
	}

	return instrumentPorts
}

// InvertPortMappings inverts the mapping to index by port name, handling
// collisions
func (pp *PortProcessor) InvertPortMappings(
	instrumentPorts map[string]map[string]map[int64]any,
) map[string]any {
	portConfigurations := make(map[string]any)

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
		pp.Log.Debug(
			"The properties are %v",
			properties,
		)

		for propertyName, indices := range properties {
			pp.Log.Debug(
				"Processing property %s with %d indices",
				propertyName,
				len(indices),
			)

			for index, portValue := range indices {
				pp.logger.Debug(
					HandlerName,
					fmt.Sprintf(
						"Processing index %d with portValue: %v (type: %T)",
						index,
						portValue,
						portValue,
					),
				)
				if port, ok := portValue.(string); ok {
					// Check if this port already exists
					if existingValue, exists := portConfigurations[port]; exists {
						if existingConfig, ok := existingValue.(PortConfiguration); ok {
							// Add this property to the existing configuration
							existingConfig.Properties = append(
								existingConfig.Properties,
								propertyName,
							)
							portConfigurations[port] = existingConfig
						}
					} else {
						// First occurrence, create new config with property
						// array
						portConfigurations[port] = PortConfiguration{
							Instrument: instrumentName,
							Properties: []string{propertyName},
							Index:      index,
						}
					}
				}
			}
		}
	}
	pp.logger.Debug(
		HandlerName,
		fmt.Sprintf(
			"Port mapping inversion complete. Found %d port configurations",
			len(portConfigurations),
		),
	)

	return portConfigurations
}

// BuildFinalConfigurations builds final configuration mapping from port names
// to instrument configurations
func (pp *PortProcessor) BuildFinalConfigurations(
	portConfigurations map[string]any,
	instruments map[string]*InstrumentProcess,
) map[string]map[string]any {
	finalConfigurations := make(map[string]map[string]any)

	for portName, portConfigValue := range portConfigurations {
		if portConfig, ok := portConfigValue.(PortConfiguration); ok {
			// Get the instrument process to access its configuration
			if instrumentProcess, exists := instruments[portConfig.Instrument]; exists &&
				instrumentProcess.Configuration != nil {

				finalConfigurations[portName] = make(map[string]any)

				// For each property in this port configuration
				for _, propertyName := range portConfig.Properties {
					// Get the configuration value for this property at the
					// specific index
					if propertyConfig, exists := instrumentProcess.Configuration[propertyName]; exists {
						if propertyMap, ok := propertyConfig.(map[int64]any); ok {
							if configValue, exists := propertyMap[portConfig.Index]; exists {
								finalConfigurations[portName][propertyName] = configValue
							}
						}
					}
				}
			}
		}
	}

	return finalConfigurations
}

// BuildPortConfigurations builds the port configurations mapping (Step 1 + Step
// 2)
// Returns a mapping from port names to their configuration details
func (pp *PortProcessor) BuildPortConfigurations(
	instruments map[string]*InstrumentProcess,
) (map[string]any, error) {
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
	portName string,
	instruments map[string]*InstrumentProcess,
) (*PortConfiguration, error) {
	// Check cache first
	var portConfigurations map[string]any
	if cached, exists := pp.getCachedPortConfigurations(); exists {
		portConfigurations = cached
	} else {
		// Build and cache if not available
		portConfigurations = pp.buildAndCachePortConfigurations(instruments)
	}

	if portConfigValue, exists := portConfigurations[portName]; exists {
		if portConfig, ok := portConfigValue.(PortConfiguration); ok {
			return &portConfig, nil
		}
		return nil, fmt.Errorf(
			"port %s has invalid configuration type",
			portName,
		)
	}

	return nil, fmt.Errorf("port %s not found in configurations", portName)
}

// InvalidatePortConfigCache invalidates the cached port configurations
// This should be called when instruments are added, removed, or reconfigured
func (pp *PortProcessor) InvalidatePortConfigCache() {
	pp.cacheMutex.Lock()
	defer pp.cacheMutex.Unlock()

	pp.portConfigsCached = false
	pp.cachedPortConfigs = make(map[string]any)
}

// buildAndCachePortConfigurations builds port configurations and caches them
func (pp *PortProcessor) buildAndCachePortConfigurations(
	instruments map[string]*InstrumentProcess,
) map[string]any {
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
func (pp *PortProcessor) getCachedPortConfigurations() (map[string]any, bool) {
	pp.cacheMutex.RLock()
	defer pp.cacheMutex.RUnlock()

	if pp.portConfigsCached {
		// Return a copy to prevent external modification
		result := make(map[string]any)
		for k, v := range pp.cachedPortConfigs {
			result[k] = v
		}
		return result, true
	}

	return nil, false
}
