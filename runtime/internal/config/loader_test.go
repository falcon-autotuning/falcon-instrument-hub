package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create valid device config
	deviceConfigPath := filepath.Join(tempDir, "device.yaml")
	deviceConfigContent := `
ScreeningGates: S1;S2
PlungerGates: P1;P2;P3;P4
Ohmics: O1;O2
BarrierGates: B1;B2;B3
ReservoirGates: R1;R2
num-unique-channels: 2

groups:
  group1:
    Name: I_O1
    NumDots: 4
    ScreeningGates: S1;S2
    ReservoirGates: R1;R2
    PlungerGates: P1;P2;P3;P4
    BarrierGates: B1;B2;B3
    Order: O1;R1;B1;P1;B2;P2;B3;P3;R2;O2
  group2:
    Name: I_O2
    NumDots: 3
    ScreeningGates: S1
    ReservoirGates: R1
    PlungerGates: P1;P2;P3
    BarrierGates: B1;B2
    Order: O1;R1;B1;P1;B2;P2;R2;O2

wiringDC:
  S1:
    resistance: 1000.0
    capacitance: 1e-12
  S2:
    resistance: 1000.0
    capacitance: 1e-12
  P1:
    resistance: 500.0
    capacitance: 2e-12
  P2:
    resistance: 500.0
    capacitance: 2e-12
  P3:
    resistance: 500.0
    capacitance: 2e-12
  P4:
    resistance: 500.0
    capacitance: 2e-12
  O1:
    resistance: 100.0
    capacitance: 1e-15
  O2:
    resistance: 100.0
    capacitance: 1e-15
  B1:
    resistance: 800.0
    capacitance: 5e-13
  B2:
    resistance: 800.0
    capacitance: 5e-13
  B3:
    resistance: 800.0
    capacitance: 5e-13
  R1:
    resistance: 1200.0
    capacitance: 8e-13
  R2:
    resistance: 1200.0
    capacitance: 8e-13
`
	err = os.WriteFile(deviceConfigPath, []byte(deviceConfigContent), 0644)
	require.NoError(t, err)

	// Create valid wiremap
	wiremapPath := filepath.Join(tempDir, "wiremap.yaml")
	wiremapContent := `
dac1.0: S1
dac1.1: P1
dac2.0: keithley1.ch1
keithley1.ch2: O1
`
	err = os.WriteFile(wiremapPath, []byte(wiremapContent), 0644)
	require.NoError(t, err)

	// Test successful loading
	cfg, err := LoadConfig(deviceConfigPath, wiremapPath)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, deviceConfigPath, cfg.DeviceConfigPath)
	assert.Equal(t, wiremapPath, cfg.WiremapPath)
	assert.NotNil(t, cfg.DeviceConfig)
	assert.NotNil(t, cfg.WireMap)

	// Verify device config content
	assert.Equal(t, "S1;S2", cfg.DeviceConfig.ScreeningGates)
	assert.Equal(t, "P1;P2;P3;P4", cfg.DeviceConfig.PlungerGates)
	assert.Equal(t, 2, cfg.DeviceConfig.NumUniqueChannels)
	assert.Len(t, cfg.DeviceConfig.Groups, 2)
	assert.Equal(t, "I_O1", cfg.DeviceConfig.Groups["group1"].Name)
	assert.Equal(t, 4, cfg.DeviceConfig.Groups["group1"].NumDots)
	assert.Len(t, cfg.DeviceConfig.WiringDC, 13)
	assert.Equal(t, 1000.0, cfg.DeviceConfig.WiringDC["S1"].Resistance)
	assert.Equal(t, 1e-12, cfg.DeviceConfig.WiringDC["S1"].Capacitance)

	// Verify wiremap content
	assert.Len(t, *cfg.WireMap, 4)
	assert.Equal(t, InstrumentConnection("S1"), (*cfg.WireMap)["dac1.0"])
	assert.Equal(t, InstrumentConnection("P1"), (*cfg.WireMap)["dac1.1"])
}

func TestLoadConfig_NonexistentFiles(t *testing.T) {
	_, err := LoadConfig("nonexistent.yaml", "nonexistent.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load device config")
}

func TestLoadDeviceConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "device_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("valid config", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "valid.yaml")
		content := `
ScreeningGates: S1;S2
PlungerGates: P1;P2
Ohmics: O1;O2
BarrierGates: B1;B2
ReservoirGates: R1;R2
num-unique-channels: 1

groups:
  group1:
    Name: TestGroup
    NumDots: 2
    ScreeningGates: S1
    ReservoirGates: R1
    PlungerGates: P1;P2
    BarrierGates: B1
    Order: O1;R1;B1;P1;P2;R2;O2

wiringDC:
  S1:
    resistance: 100.0
    capacitance: 1e-15
  S2:
    resistance: 100.0
    capacitance: 1e-15
  P1:
    resistance: 200.0
    capacitance: 2e-15
  P2:
    resistance: 200.0
    capacitance: 2e-15
  O1:
    resistance: 50.0
    capacitance: 5e-16
  O2:
    resistance: 50.0
    capacitance: 5e-16
  B1:
    resistance: 300.0
    capacitance: 3e-15
  B2:
    resistance: 300.0
    capacitance: 3e-15
  R1:
    resistance: 400.0
    capacitance: 4e-15
  R2:
    resistance: 400.0
    capacitance: 4e-15
`
		err = os.WriteFile(configPath, []byte(content), 0644)
		require.NoError(t, err)

		config, err := loadDeviceConfig(configPath)
		require.NoError(t, err)
		assert.Equal(t, "S1;S2", config.ScreeningGates)
		assert.Equal(t, 1, config.NumUniqueChannels)
		assert.Len(t, config.Groups, 1)
		assert.Equal(t, "TestGroup", config.Groups["group1"].Name)
		assert.Equal(t, 2, config.Groups["group1"].NumDots)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "invalid.yaml")
		content := `
invalid: yaml: content:
  - missing
    proper: structure
`
		err = os.WriteFile(configPath, []byte(content), 0644)
		require.NoError(t, err)

		_, err = loadDeviceConfig(configPath)
		assert.Error(t, err)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := loadDeviceConfig("nonexistent.yaml")
		assert.Error(t, err)
	})
}

func TestLoadWireMap(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "wiremap_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	t.Run("valid wiremap", func(t *testing.T) {
		wiremapPath := filepath.Join(tempDir, "valid.yaml")
		content := `
dac1.0: S1
dac1.1: P1
dac2.0: keithley1.ch1
keithley1.ch2: O1
multimeter.ch1: dac3.0
`
		err = os.WriteFile(wiremapPath, []byte(content), 0644)
		require.NoError(t, err)

		wireMap, err := loadWireMap(wiremapPath)
		require.NoError(t, err)
		assert.Len(t, *wireMap, 5)
		assert.Equal(t, InstrumentConnection("S1"), (*wireMap)["dac1.0"])
		assert.Equal(t, InstrumentConnection("P1"), (*wireMap)["dac1.1"])
		assert.Equal(
			t,
			InstrumentConnection("keithley1.ch1"),
			(*wireMap)["dac2.0"],
		)
		assert.Equal(t, InstrumentConnection("O1"), (*wireMap)["keithley1.ch2"])
		assert.Equal(
			t,
			InstrumentConnection("dac3.0"),
			(*wireMap)["multimeter.ch1"],
		)
	})

	t.Run("empty wiremap", func(t *testing.T) {
		wiremapPath := filepath.Join(tempDir, "empty.yaml")
		content := `{}`
		err = os.WriteFile(wiremapPath, []byte(content), 0644)
		require.NoError(t, err)

		wireMap, err := loadWireMap(wiremapPath)
		require.NoError(t, err)
		assert.Len(t, *wireMap, 0)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		wiremapPath := filepath.Join(tempDir, "invalid.yaml")
		content := `
invalid: yaml: [content
  missing: brackets
`
		err = os.WriteFile(wiremapPath, []byte(content), 0644)
		require.NoError(t, err)

		_, err = loadWireMap(wiremapPath)
		assert.Error(t, err)
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := loadWireMap("nonexistent.yaml")
		assert.Error(t, err)
	})
}
