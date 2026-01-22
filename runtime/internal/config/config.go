package config

import (
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

// GetRPCBaseURL returns the base URL for the instrument-script-server RPC API
func (c *Config) GetRPCBaseURL() string {
	return fmt.Sprintf("http://%s:%d", c.InstrumentServerHost, c.InstrumentServerRPCPort)
}
