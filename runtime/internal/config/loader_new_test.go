package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigCGO(t *testing.T) {
	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create valid device config
	deviceConfigPath := filepath.Join(tempDir, "device.yaml")
	deviceConfigContent := `
ScreeningGates: "S1;S2;S3"
PlungerGates: "P1;P2;P3"
Ohmics: "O1;O2;O3;O4"
BarrierGates: "B1;B2;B3;B4;B5"
ReservoirGates: "R1;R2;R3;R4"
num-unique-channels: 2
groups:
  group1:
    Name: "I_O1"
    NumDots: 2
    ScreeningGates: "S1;S2"
    ReservoirGates: "R1;R2"
    PlungerGates: "P1;P2"
    BarrierGates: "B1;B2;B3"
    Order: "O1;R1;B1;P1;B2;P2;B3;R2;O2"
  group2:
    Name: "I_O3"
    NumDots: 1
    ScreeningGates: "S2;S3"
    ReservoirGates: "R3;R4"
    PlungerGates: "P3"
    BarrierGates: "B4;B5"
    Order: "O3;R3;B4;P3;B5;R4;O4"
adjacency:
  S2: "P1;P2;P3;P4;R1;R2;R3;R4;B1;B2;B3;B4;B5"
  S1: "P1;P2;R1;R2;B1;B2;B3"
  S3: "P3;B4;B5;R3;R4"
  B1: "R1;P1"
  B2: "P1;P2"
  B3: "P2;R2"
  B4: "P3;R3"
  B5: "R4;P3"
  O3: "R3"
  O4: "R4"
  O1: "R1"
  O2: "R2"
max_safe_diff: 1.0
safe_voltage_bounds: [-1.0, 1.0]
wiringDC:
  S1: { resistance: 1000.0, capacitance: 1e-12 }
  S2: { resistance: 1000.0, capacitance: 1e-12 }
  S3: { resistance: 1000.0, capacitance: 1e-12 }
  P1: { resistance: 1000.0, capacitance: 1e-12 }
  P2: { resistance: 1000.0, capacitance: 1e-12 }
  P3: { resistance: 1000.0, capacitance: 1e-12 }
  O1: { resistance: 1000.0, capacitance: 1e-12 }
  O2: { resistance: 1000.0, capacitance: 1e-12 }
  O3: { resistance: 1000.0, capacitance: 1e-12 }
  O4: { resistance: 1000.0, capacitance: 1e-12 }
  R1: { resistance: 1000.0, capacitance: 1e-12 }
  R2: { resistance: 1000.0, capacitance: 1e-12 }
  R3: { resistance: 1000.0, capacitance: 1e-12 }
  R4: { resistance: 1000.0, capacitance: 1e-12 }
  B1: { resistance: 1000.0, capacitance: 1e-12 }
  B2: { resistance: 1000.0, capacitance: 1e-12 }
  B3: { resistance: 1000.0, capacitance: 1e-12 }
  B4: { resistance: 1000.0, capacitance: 1e-12 }
  B5: { resistance: 1000.0, capacitance: 1e-12 }
`
	err = os.WriteFile(deviceConfigPath, []byte(deviceConfigContent), 0644)
	require.NoError(t, err)

	// Create valid wiremap
	wiremapPath := filepath.Join(tempDir, "wiremap.yaml")
	wiremapContent := `
wiremap:
- name: S1
  instrument:
    name: Source1
    channel_name: analog
    index: 1
- name: S2
  instrument:
    name: Source1
    channel_name: analog
    index: 2
- name: S3
  instrument:
    name: Source1
    channel_name: analog
    index: 3
- name: P1
  instrument:
    name: Source1
    channel_name: analog
    index: 4
- name: P2
  instrument:
    name: Source1
    channel_name: analog
    index: 5
- name: P3
  instrument:
    name: Source1
    channel_name: analog
    index: 6
- name: B1
  instrument:
    name: Source1
    channel_name: analog
    index: 7
- name: B2
  instrument:
    name: Source1
    channel_name: analog
    index: 8
- name: B3
  instrument:
    name: Source1
    channel_name: analog
    index: 9
- name: B4
  instrument:
    name: Source1
    channel_name: analog
    index: 10
- name: B5
  instrument:
    name: Source1
    channel_name: analog
    index: 11
- name: R1
  instrument:
    name: Source1
    channel_name: analog
    index: 12
- name: R2
  instrument:
    name: Source1
    channel_name: analog
    index: 13
- name: R3
  instrument:
    name: Source1
    channel_name: analog
    index: 14
- name: R4
  instrument:
    name: Source1
    channel_name: analog
    index: 15
- name: O1
  instrument:
    name: Source1
    channel_name: analog
    index: 16
- name: O2
  instrument:
    name: Meter1
    channel_name: analog
    index: 1
- name: O3
  instrument:
    name: Source1
    channel_name: analog
    index: 17
- name: O4
  instrument:
    name: Meter1
    channel_name: analog
    index: 2
`
	err = os.WriteFile(wiremapPath, []byte(wiremapContent), 0644)
	require.NoError(t, err)

	// Test successful loading
	cfg, err := LoadConfigCGO(deviceConfigPath, wiremapPath)
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, deviceConfigPath, cfg.DeviceConfigPath)
	assert.Equal(t, wiremapPath, cfg.WiremapPath)
	assert.NotNil(t, cfg.DeviceConfig)
	assert.NotNil(t, cfg.WireMap)

	// Verify device config content
	assert.Equal(t, "S1;S2;S3", cfg.DeviceConfig.ScreeningGates)
	assert.Equal(t, "P1;P2;P3", cfg.DeviceConfig.PlungerGates)
	assert.Equal(t, 2, cfg.DeviceConfig.NumUniqueChannels)
	assert.Len(t, cfg.DeviceConfig.Groups, 2)
	// falcon-core returns the gname key as the group Name field
	assert.Equal(t, "group1", cfg.DeviceConfig.Groups["group1"].Name)
	assert.Equal(t, 2, cfg.DeviceConfig.Groups["group1"].NumDots)
	assert.Len(t, cfg.DeviceConfig.WiringDC, 19)
	assert.Equal(t, 1000.0, cfg.DeviceConfig.WiringDC["S1"].Resistance)
	assert.Equal(t, 1e-12, cfg.DeviceConfig.WiringDC["S1"].Capacitance)

	// Verify wiremap content
	// Keys are "instrumentName.channelName.index" matching the new wiremap format.
	assert.Len(t, *cfg.WireMap, 19)
	assert.Equal(t, InstrumentConnection("S1"), (*cfg.WireMap)["Source1.analog.1"])
	assert.Equal(t, InstrumentConnection("P1"), (*cfg.WireMap)["Source1.analog.4"])
	assert.Equal(t, InstrumentConnection("O2"), (*cfg.WireMap)["Meter1.analog.1"])
	assert.Equal(t, InstrumentConnection("O4"), (*cfg.WireMap)["Meter1.analog.2"])
}
