// Package ports provides types and functions for building a static port
// library from instrument API YAML files and connecting ports to physical
// device gates via a wiremap.
package ports

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// InstrumentAPI represents the top-level structure of an instrument API YAML file.
type InstrumentAPI struct {
	APIVersion    string         `yaml:"api_version"`
	Instrument    APIInstrument  `yaml:"instrument"`
	Protocol      APIProtocol    `yaml:"protocol"`
	ChannelGroups []ChannelGroup `yaml:"channel_groups"`
}

// APIInstrument describes the instrument identity within an API file.
type APIInstrument struct {
	Vendor      string `yaml:"vendor"`
	Model       int    `yaml:"model"`
	Identifier  string `yaml:"identifier"`
	Description string `yaml:"description"`
}

// APIProtocol describes the communication protocol used by the instrument.
type APIProtocol struct {
	Type string `yaml:"type"`
}

// ChannelGroup describes a named group of channel io types.
type ChannelGroup struct {
	Name             string           `yaml:"name"`
	Description      string           `yaml:"description"`
	ChannelParameter ChannelParameter `yaml:"channel_parameter"`
	IoTypes          []IoType         `yaml:"io_types"`
}

// ChannelParameter describes the integer parameter used to select a channel.
type ChannelParameter struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Min         int    `yaml:"min"`
	Max         int    `yaml:"max"`
	Description string `yaml:"description"`
}

// IoType describes a single IO signal within a channel group.
type IoType struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Role        string `yaml:"role"` // "input", "output", or "setting"
	Description string `yaml:"description"`
	Suffix      string `yaml:"suffix"`
	Unit        string `yaml:"unit"`
}

// ParseInstrumentAPI parses a single instrument API YAML file.
func ParseInstrumentAPI(path string) (*InstrumentAPI, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read instrument API file %s: %w", path, err)
	}

	var api InstrumentAPI
	if err := yaml.Unmarshal(data, &api); err != nil {
		return nil, fmt.Errorf("failed to parse instrument API file %s: %w", path, err)
	}

	if api.Instrument.Identifier == "" {
		return nil, fmt.Errorf("instrument API file %s missing instrument identifier", path)
	}
	if api.Instrument.Vendor == "" {
		return nil, fmt.Errorf("instrument API file %s missing instrument vendor", path)
	}

	return &api, nil
}

// ParseInstrumentAPIs parses multiple instrument API YAML files and returns
// them as a slice.
func ParseInstrumentAPIs(paths []string) ([]InstrumentAPI, error) {
	apis := make([]InstrumentAPI, 0, len(paths))
	for _, path := range paths {
		api, err := ParseInstrumentAPI(path)
		if err != nil {
			return nil, err
		}
		apis = append(apis, *api)
	}
	return apis, nil
}
