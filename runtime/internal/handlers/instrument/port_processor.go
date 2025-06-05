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
