package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// InstrumentConfigFile represents the top-level structure of an instrument Configuration YAML file.
type InstrumentConfigFile struct {
	Name    string `yaml:"name"`
	API_ref string `yaml:"api_ref"`
}

func ParseInstrumentConfig(path string) (*InstrumentConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read instrument API file %s: %w", path, err)
	}

	var config InstrumentConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse instrument API file %s: %w", path, err)
	}

	if config.Name == "" {
		return nil, fmt.Errorf("instrument config file %s missing instrument name", path)
	}
	if config.API_ref == "" {
		return nil, fmt.Errorf("instrument config file %s missing instrument API reference", path)
	}
	return &config, nil
}

// InstrumentAPI represents the top-level structure of an instrument API YAML file.
type InstrumentAPI struct {
	APIVersion    string                  `yaml:"api_version"`
	Instrument    APIInstrument           `yaml:"instrument"`
	Protocol      APIProtocol             `yaml:"protocol"`
	ChannelGroups []ChannelGroup          `yaml:"channel_groups"`
	IO            []IoType                `yaml:"io"`
	Commands      map[CommandName]Command `yaml:"commands"`
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
	Role        string `yaml:"role"`
	Description string `yaml:"description"`
	Suffix      string `yaml:"suffix"`
	Min         string `yaml:"min,omitempty"`
	Max         string `yaml:"max,omitempty"`
	Unit        string `yaml:"unit"`
}
type (
	CommandName string
	Command     struct {
		ChannelGroup string `yaml:"channel_group"`
	}
)

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
