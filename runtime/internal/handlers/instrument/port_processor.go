package instrument

import (
	"fmt"
	"strings"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

// PortProcessor handles processing and augmentation of instrument ports
type PortProcessor struct {
	logger      *logging.Logger
	nameMapping map[string]*config.DeviceConnection
}

// NewPortProcessor creates a new port processor
func NewPortProcessor(
	logger *logging.Logger,
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
		logger:      logger,
		nameMapping: nameMapping,
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
		pp.logger.Error(
			HandlerName,
			fmt.Sprintf(
				"Failed to augment ports for instrument %s: %v",
				instrumentName,
				err,
			),
		)
		return err
	}

	pp.logger.Debug(
		HandlerName,
		fmt.Sprintf(
			"Successfully processed ports for instrument %s",
			instrumentName,
		),
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

	pp.logger.Debug(
		HandlerName,
		fmt.Sprintf(
			"Collecting port properties from %d instruments",
			len(instruments),
		),
	)

	// Collect ports from all active instruments (processing should already be
	// done)
	for _, instrument := range instruments {
		if !instrument.Initialized || instrument.Ports == nil {
			continue
		}

		for _, innerMap := range instrument.Ports {
			// Type assert innerMap to map[int64]any or similar
			if portMap, ok := innerMap.(map[int64]any); ok {
				for _, value := range portMap {
					if valueStr, ok := value.(string); ok {
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
) (map[string]map[string]interface{}, error) {
	// Step 1: Collect all ports organized by instrument → property → index
	// → port data
	instrumentPorts := make(map[string]map[string]map[int64]interface{})

	for instrumentName, instrumentProcess := range instruments {
		if !instrumentProcess.Initialized || instrumentProcess.Ports == nil {
			continue
		}

		instrumentPorts[instrumentName] = make(map[string]map[int64]interface{})

		// Copy the ports structure
		for propertyName, propertyData := range instrumentProcess.Ports {
			if portMap, ok := propertyData.(map[int64]interface{}); ok {
				instrumentPorts[instrumentName][propertyName] = make(
					map[int64]interface{},
				)
				for index, portValue := range portMap {
					instrumentPorts[instrumentName][propertyName][index] = portValue
				}
			}
		}
	}

	// Step 2: Invert the mapping - index by port name, handling collisions
	portConfigurations := make(map[string]interface{})

	for instrumentName, properties := range instrumentPorts {
		for propertyName, indices := range properties {
			for index, portValue := range indices {
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

	// Step 3: Build final configuration mapping from port names to instrument
	// configurations
	finalConfigurations := make(map[string]map[string]interface{})

	for portName, portConfigValue := range portConfigurations {
		if portConfig, ok := portConfigValue.(PortConfiguration); ok {
			// Get the instrument process to access its configuration
			if instrumentProcess, exists := instruments[portConfig.Instrument]; exists &&
				instrumentProcess.Configuration != nil {

				finalConfigurations[portName] = make(map[string]interface{})

				// For each property in this port configuration
				for _, propertyName := range portConfig.Properties {
					// Get the configuration value for this property at the
					// specific index
					if propertyConfig, exists := instrumentProcess.Configuration[propertyName]; exists {
						if propertyMap, ok := propertyConfig.(map[int64]interface{}); ok {
							if configValue, exists := propertyMap[portConfig.Index]; exists {
								finalConfigurations[portName][propertyName] = configValue
							}
						}
					}
				}
			}
		}
	}

	return finalConfigurations, nil
}
