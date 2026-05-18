// Package serverinterpreter provides hub configuration loading and management.
//
// This file implements loading for the instrument_hub_config.yaml which tells
// the hub where to find device configs, wire maps, databases, and other resources.
package serverinterpreter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// HubConfig represents the instrument hub configuration.
// This is loaded from instrument_hub_config.yaml.
type HubConfig struct {
	// Wiremap is the path to the wire mapping configuration
	Wiremap string `yaml:"wiremap" json:"wiremap"`

	// QuantumDotConfig is the path to the quantum dot device configuration
	QuantumDotConfig string `yaml:"quantum-dot-config" json:"quantum-dot-config"`

	// InstConfig is the path to instrument configuration files
	InstConfig string `yaml:"inst-config" json:"inst-config"`

	// InstPlugins is a semicolon-separated list of plugin paths, one per entry
	// in InstConfig (positional). An empty entry means no plugin for that instrument.
	InstPlugins string `yaml:"inst-plugins" json:"inst-plugins"`

	// TealAPIs is the path to Teal API definitions
	TealAPIs string `yaml:"teal-apis" json:"teal-apis"`

	// LuaLibraryTypes is the path to Lua type definitions
	LuaLibraryTypes string `yaml:"lua-library-types" json:"lua-library-types"`

	// UserMeasurementLuas is the path to user-defined Lua measurement scripts
	UserMeasurementLuas string `yaml:"user-measurement-luas" json:"user-measurement-luas"`

	// LocalDatabase is the path to the local HDF5 database directory
	LocalDatabase string `yaml:"local-database" json:"local-database"`

	// NATSUrl is the NATS server URL
	NATSUrl string `yaml:"nats-url" json:"nats-url"`

	// InstrumentServerPort is the port for the instrument script server
	InstrumentServerPort int `yaml:"instrument-server-port" json:"instrument-server-port"`

	// InstAPIs is a semicolon-separated list of paths to instrument API YAML files.
	InstAPIs string `yaml:"inst-apis" json:"inst-apis"`

	// configPath stores the path this config was loaded from
	configPath string
}

// LoadHubConfig loads a hub configuration from a YAML file.
func LoadHubConfig(path string) (*HubConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read hub config: %w", err)
	}

	var config HubConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse hub config: %w", err)
	}

	config.configPath = path
	return &config, nil
}

// ParseHubConfig parses hub configuration from YAML bytes.
func ParseHubConfig(data []byte) (*HubConfig, error) {
	var config HubConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse hub config: %w", err)
	}
	return &config, nil
}

// ConfigDir returns the directory containing the config file.
func (c *HubConfig) ConfigDir() string {
	if c.configPath == "" {
		return ""
	}
	return filepath.Dir(c.configPath)
}

// ResolvePath resolves a path relative to the config file location.
// If the path is absolute, it's returned as-is.
func (c *HubConfig) ResolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if c.configPath == "" {
		return p
	}
	return filepath.Join(filepath.Dir(c.configPath), p)
}

// GetDatabasePath returns the full path for a database file.
func (c *HubConfig) GetDatabasePath(filename string) string {
	if c.LocalDatabase == "" {
		return filename
	}
	return filepath.Join(c.ResolvePath(c.LocalDatabase), filename)
}

// GetNATSUrl returns the NATS URL, with a default if not specified.
func (c *HubConfig) GetNATSUrl() string {
	if c.NATSUrl == "" {
		return "nats://localhost:4222"
	}
	return c.NATSUrl
}

// GetInstrumentServerPort returns the instrument server port, with a default.
func (c *HubConfig) GetInstrumentServerPort() int {
	if c.InstrumentServerPort == 0 {
		return 5555
	}
	return c.InstrumentServerPort
}

// GetInstAPIPaths parses InstAPIs (semicolon-separated) into a slice of
// resolved file paths. Empty entries are skipped.
func (c *HubConfig) GetInstAPIPaths() []string {
	if c.InstAPIs == "" {
		return nil
	}
	var paths []string
	for _, p := range strings.Split(c.InstAPIs, ";") {
		p = strings.TrimSpace(p)
		if p != "" {
			paths = append(paths, c.ResolvePath(p))
		}
	}
	return paths
}

// Validate checks that required configuration is present.
func (c *HubConfig) Validate() error {
	if c.LocalDatabase == "" {
		return fmt.Errorf("local-database path is required")
	}
	return nil
}

// EnsureDatabaseDir creates the database directory if it doesn't exist.
func (c *HubConfig) EnsureDatabaseDir() error {
	path := c.ResolvePath(c.LocalDatabase)
	return os.MkdirAll(path, 0755)
}


