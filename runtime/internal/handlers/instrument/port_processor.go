package instrument

import (
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
	nameMapping       map[string]*config.DeviceConnection
	cachedPortConfigs map[string]PortConfiguration // Cache for port configurations
	portConfigsCached bool                         // Flag to track if cache is valid
	cacheMutex        sync.RWMutex                 // Mutex for cache access
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
		cachedPortConfigs: make(map[string]PortConfiguration),
		portConfigsCached: false,
	}, nil
}

// ProcessInstrumentPorts processes and augments ports for a specific instrument
// This should be called immediately after an instrument is loaded and
// initialized
func (pp *PortProcessor) ProcessInstrumentPorts(
	instrumentName string,
	instrumentPorts map[string]map[string]string,
) error {
	if instrumentPorts == nil {
		return fmt.Errorf("instrument %s has no ports", instrumentName)
	}
	return config.ProcessInstrumentPorts(
		instrumentPorts,
		pp.nameMapping,
		instrumentName,
	)
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

		for _, portMap := range instrument.Ports {
			for _, value := range portMap {

				if strings.Contains(value, KnobIdentifier) {
					knobs = append(knobs, value)
				}
				if strings.Contains(value, MeterIdentifier) {
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
type PortConfiguration struct {
	Instrument string   `json:"instrument"`
	Properties []string `json:"properties"`
	Index      string   `json:"index"`
}

// BuildConfigurations creates the configuration mapping by collecting and
// inverting port mappings
func (pp *PortProcessor) BuildConfigurations(
	instruments map[string]*InstrumentProcess,
) (map[string]map[string]any, error) {
	// Get cached port configurations (Step 1 + Step 2)
	var portConfigurations map[string]PortConfiguration
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
) map[string]map[string]map[string]string {
	instrumentPorts := make(map[string]map[string]map[string]string)
	for instrumentName, instrumentProcess := range instruments {
		if !instrumentProcess.Initialized || instrumentProcess.Ports == nil {
			continue
		}
		instrumentPorts[instrumentName] = make(map[string]map[string]string)
		for propertyName, propertyContents := range instrumentProcess.Ports {
			instrumentPorts[instrumentName][propertyName] = make(
				map[string]string,
			)
			maps.Copy(
				instrumentPorts[instrumentName][propertyName],
				propertyContents,
			)
		}
	}
	return instrumentPorts
}

// InvertPortMappings inverts the mapping to index by port name, handling
// collisions
func (pp *PortProcessor) InvertPortMappings(
	instrumentPorts map[string]map[string]map[string]string,
) map[string]PortConfiguration {
	portConfigurations := make(map[string]PortConfiguration)

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

		for propertyName, indices := range properties {
			for index, port := range indices {
				if existingConfig, exists := portConfigurations[port]; exists {
					// Add this property to the existing configuration
					existingConfig.Properties = append(
						existingConfig.Properties,
						propertyName,
					)
					portConfigurations[port] = existingConfig
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
	portConfigurations map[string]PortConfiguration,
	instruments map[string]*InstrumentProcess,
) map[string]map[string]any {
	finalConfigurations := make(map[string]map[string]any)

	for portName, portConfig := range portConfigurations {
		// Get the instrument process to access its configuration
		if instrumentProcess, exists := instruments[portConfig.Instrument]; exists &&
			instrumentProcess.Configuration != nil {

			finalConfigurations[portName] = make(map[string]any)

			// For each property in this port configuration
			for _, propertyName := range portConfig.Properties {
				// Get the configuration value for this property at the
				// specific index
				if propertyConfig, exists := instrumentProcess.Configuration[propertyName]; exists {
					if configValue, exists := propertyConfig[portConfig.Index]; exists {
						finalConfigurations[portName][propertyName] = configValue
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
) (map[string]PortConfiguration, error) {
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
	var portConfigurations map[string]PortConfiguration
	if cached, exists := pp.getCachedPortConfigurations(); exists {
		portConfigurations = cached
	} else {
		// Build and cache if not available
		portConfigurations = pp.buildAndCachePortConfigurations(instruments)
	}

	if portConfig, exists := portConfigurations[portName]; exists {
		return &portConfig, nil
	}

	return nil, fmt.Errorf("port %s not found in configurations", portName)
}

// InvalidatePortConfigCache invalidates the cached port configurations
// This should be called when instruments are added, removed, or reconfigured
func (pp *PortProcessor) InvalidatePortConfigCache() {
	pp.cacheMutex.Lock()
	defer pp.cacheMutex.Unlock()

	pp.portConfigsCached = false
	pp.cachedPortConfigs = make(map[string]PortConfiguration)
}

// buildAndCachePortConfigurations builds port configurations and caches them
func (pp *PortProcessor) buildAndCachePortConfigurations(
	instruments map[string]*InstrumentProcess,
) map[string]PortConfiguration {
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
func (pp *PortProcessor) getCachedPortConfigurations() (map[string]PortConfiguration, bool) {
	pp.cacheMutex.RLock()
	defer pp.cacheMutex.RUnlock()

	if pp.portConfigsCached {
		// Return a copy to prevent external modification

		result := make(map[string]PortConfiguration)
		maps.Copy(result, pp.cachedPortConfigs)
		return result, true
	}

	return nil, false
}
