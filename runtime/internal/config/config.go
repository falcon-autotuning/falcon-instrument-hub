package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Config holds the configuration for the instrument hub runtime
type Config struct {
	// InstrumentServerRPCPort is the port where instrument-script-server RPC is listening
	InstrumentServerRPCPort int
	// InstrumentServerHost is the host where instrument-script-server is running
	InstrumentServerHost string
	// InstrumentServerBinary is the path to the instrument-server executable
	InstrumentServerBinary string
	
	// HubConfig contains the full hub configuration if loaded from file
	HubConfig *HubConfig
}

// HubConfig represents the full configuration for the Falcon Instrument Hub
// This matches the JSON schema in runtime/config-schema.json
type HubConfig struct {
	// Wiremap defines physical connections between instruments and quantum devices
	Wiremap string `json:"wiremap"`
	
	// QuantumDotConfig describes the quantum device and hardware connections
	QuantumDotConfig string `json:"quantum-dot-config"`
	
	// InstConfig is the directory containing instrument configuration files
	InstConfig string `json:"inst-config"`
	
	// TealAPIs is the directory containing Teal API definitions
	TealAPIs string `json:"teal-apis"`
	
	// LuaLibraryTypes is the directory with Lua library types
	// These will be injected into INSTRUMENT_SCRIPT_SERVER_OPT_LUA_LIB
	LuaLibraryTypes string `json:"lua-library-types"`
	
	// UserMeasurementLuas is the directory containing measurement Lua scripts
	// The compiler selects scripts from here to issue to instrument-script-server
	UserMeasurementLuas string `json:"user-measurement-luas"`
	
	// LocalDatabase is the root directory for the HDF5 database
	LocalDatabase string `json:"local-database"`
	
	// NatsURL is the NATS message broker connection URL
	NatsURL string `json:"nats-url"`
	
	// InstrumentServerPort is the port for instrument-script-server HTTP/RPC
	InstrumentServerPort int `json:"instrument-server-port"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		InstrumentServerRPCPort: 8555,
		InstrumentServerHost:    "localhost",
		InstrumentServerBinary:  "instrument-server",
	}
}

// LoadConfig loads configuration from environment variables or uses defaults
func LoadConfig() (*Config, error) {
	cfg := DefaultConfig()

	// Override with environment variables if set
	if portStr := os.Getenv("INSTRUMENT_SCRIPT_SERVER_RPC_PORT"); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid INSTRUMENT_SCRIPT_SERVER_RPC_PORT: %w", err)
		}
		cfg.InstrumentServerRPCPort = port
	}

	if host := os.Getenv("INSTRUMENT_SERVER_HOST"); host != "" {
		cfg.InstrumentServerHost = host
	}

	if binary := os.Getenv("INSTRUMENT_SERVER_BINARY"); binary != "" {
		cfg.InstrumentServerBinary = binary
	}

	return cfg, nil
}

// LoadConfigFromFile loads the full hub configuration from a JSON file
func LoadConfigFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	var hubConfig HubConfig
	if err := json.Unmarshal(data, &hubConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	cfg.HubConfig = &hubConfig
	
	// Override instrument server port if specified in hub config
	if hubConfig.InstrumentServerPort > 0 {
		cfg.InstrumentServerRPCPort = hubConfig.InstrumentServerPort
	}
	
	// Set INSTRUMENT_SCRIPT_SERVER_OPT_LUA_LIB if LuaLibraryTypes is specified
	if hubConfig.LuaLibraryTypes != "" {
		if err := os.Setenv("INSTRUMENT_SCRIPT_SERVER_OPT_LUA_LIB", hubConfig.LuaLibraryTypes); err != nil {
			return nil, fmt.Errorf("failed to set INSTRUMENT_SCRIPT_SERVER_OPT_LUA_LIB: %w", err)
		}
	}
	
	return cfg, nil
}

// GetRPCBaseURL returns the base URL for the instrument-script-server RPC API
func (c *Config) GetRPCBaseURL() string {
	return fmt.Sprintf("http://%s:%d", c.InstrumentServerHost, c.InstrumentServerRPCPort)
}
