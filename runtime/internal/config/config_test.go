package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.InstrumentServerRPCPort != 8555 {
		t.Errorf("Expected default port 8555, got %d", cfg.InstrumentServerRPCPort)
	}

	if cfg.InstrumentServerHost != "localhost" {
		t.Errorf("Expected default host 'localhost', got %s", cfg.InstrumentServerHost)
	}

	if cfg.InstrumentServerBinary != "instrument-server" {
		t.Errorf("Expected default binary 'instrument-server', got %s", cfg.InstrumentServerBinary)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("INSTRUMENT_SCRIPT_SERVER_RPC_PORT", "9000")
	os.Setenv("INSTRUMENT_SERVER_HOST", "example.com")
	os.Setenv("INSTRUMENT_SERVER_BINARY", "/usr/local/bin/instrument-server")
	defer func() {
		os.Unsetenv("INSTRUMENT_SCRIPT_SERVER_RPC_PORT")
		os.Unsetenv("INSTRUMENT_SERVER_HOST")
		os.Unsetenv("INSTRUMENT_SERVER_BINARY")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.InstrumentServerRPCPort != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.InstrumentServerRPCPort)
	}

	if cfg.InstrumentServerHost != "example.com" {
		t.Errorf("Expected host 'example.com', got %s", cfg.InstrumentServerHost)
	}

	if cfg.InstrumentServerBinary != "/usr/local/bin/instrument-server" {
		t.Errorf("Expected binary '/usr/local/bin/instrument-server', got %s", cfg.InstrumentServerBinary)
	}
}

func TestGetRPCBaseURL(t *testing.T) {
	cfg := &Config{
		InstrumentServerHost:    "testhost",
		InstrumentServerRPCPort: 1234,
	}

	expected := "http://testhost:1234"
	if got := cfg.GetRPCBaseURL(); got != expected {
		t.Errorf("Expected URL %s, got %s", expected, got)
	}
}

func TestLoadConfigInvalidPort(t *testing.T) {
	os.Setenv("INSTRUMENT_SCRIPT_SERVER_RPC_PORT", "invalid")
	defer os.Unsetenv("INSTRUMENT_SCRIPT_SERVER_RPC_PORT")

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error for invalid port, got nil")
	}
}
