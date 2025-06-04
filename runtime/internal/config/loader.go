package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads both device config and wiremap files
func LoadConfig(deviceConfigPath, wiremapPath string) (*Config, error) {
	cfg := &Config{
		DeviceConfigPath: deviceConfigPath,
		WiremapPath:      wiremapPath,
	}

	// Load device config
	deviceConfig, err := loadDeviceConfig(deviceConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load device config: %w", err)
	}
	cfg.DeviceConfig = deviceConfig

	// Load wiremap
	wireMap, err := loadWireMap(wiremapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load wiremap: %w", err)
	}
	cfg.WireMap = wireMap

	return cfg, nil
}

func loadDeviceConfig(path string) (*DeviceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config DeviceConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Validate that all device connections have wiring specifications
	if err := validateWiringDC(&config); err != nil {
		return nil, fmt.Errorf("wiring validation failed: %w", err)
	}

	return &config, nil
}
func validateWiringDC(config *DeviceConfig) error {
	// Collect all device connections that should have wiring specifications
	deviceConnections := make(map[string]bool)

	// Add all gate types from Region 1
	addConnections(deviceConnections, config.ScreeningGates)
	addConnections(deviceConnections, config.PlungerGates)
	addConnections(deviceConnections, config.Ohmics)
	addConnections(deviceConnections, config.BarrierGates)
	addConnections(deviceConnections, config.ReservoirGates)

	// Check that all device connections have wiring specifications
	for connection := range deviceConnections {
		if _, exists := config.WiringDC[connection]; !exists {
			return fmt.Errorf("device connection '%s' missing wiring specification in wiringDC section", connection)
		}
	}

	return nil
}

// addConnections parses semicolon-delimited strings and adds each connection to the map
func addConnections(connections map[string]bool, gateString string) {
	if gateString == "" {
		return
	}

	gates := strings.Split(gateString, ";")
	for _, gate := range gates {
		gate = strings.TrimSpace(gate)
		if gate != "" {
			connections[gate] = true
		}
	}
}

func loadWireMap(path string) (*WireMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var wireMap WireMap
	if err := yaml.Unmarshal(data, &wireMap); err != nil {
		return nil, err
	}

	return &wireMap, nil
}
