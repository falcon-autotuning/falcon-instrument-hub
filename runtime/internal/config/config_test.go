//go:build cgo

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIParseCharacteristicValidate_Command(t *testing.T) {
	c := APIParseCharacteristic{
		Command: "MEAS:VOLT?",
	}

	require.NoError(t, c.Validate())
}

func TestAPIParseCharacteristicValidate_ParameterNameWithMin(t *testing.T) {
	c := APIParseCharacteristic{
		ParameterName: "voltage",
		Min:           true,
	}

	require.NoError(t, c.Validate())
}

func TestAPIParseCharacteristicValidate_ParameterNameWithMax(t *testing.T) {
	c := APIParseCharacteristic{
		ParameterName: "voltage",
		Max:           true,
	}

	require.NoError(t, c.Validate())
}

func TestAPIParseCharacteristicValidate_ParameterNameWithoutMinOrMax(t *testing.T) {
	c := APIParseCharacteristic{
		ParameterName: "voltage",
	}

	err := c.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parameter parsing requires")
}

func TestAPIParseCharacteristicValidate_Empty(t *testing.T) {
	var c APIParseCharacteristic

	err := c.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "either parameterName or command")
}

func TestParseCharacteristic(t *testing.T) {
	got, err := ParseCharacteristic("sample_rate")

	require.NoError(t, err)
	assert.Equal(
		t,
		instrumentcharacteristic.InstrumentCharacteristicSampleRate,
		got,
	)
}

func TestParseCharacteristicUnknown(t *testing.T) {
	_, err := ParseCharacteristic("banana")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown characteristic")
}

func dcVoltageCharacteristicSet() CharacteristicSet {
	return ValidCharacteristics[instrument.DcVoltageSource]
}

func TestInstrumentCharacteristicValidate_ReadCommand(t *testing.T) {
	c := InstrumentCharacteristic{
		Identifier: "source_voltage",
		ReadCommand: APIParseCharacteristic{
			Command: "VOLT?",
		},
	}

	err := c.Validate(dcVoltageCharacteristicSet())

	require.NoError(t, err)

	assert.Equal(
		t,
		instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
		c.Characteristic,
	)
}

func TestInstrumentCharacteristicValidate_UnknownCharacteristic(t *testing.T) {
	c := InstrumentCharacteristic{
		Identifier: "garbage",
		ReadCommand: APIParseCharacteristic{
			Command: "x",
		},
	}

	err := c.Validate(dcVoltageCharacteristicSet())

	require.Error(t, err)
}

func TestInstrumentCharacteristicValidate_NotValidForInstrument(t *testing.T) {
	c := InstrumentCharacteristic{
		Identifier: "sample_rate",
		ReadCommand: APIParseCharacteristic{
			Command: "RATE?",
		},
	}

	err := c.Validate(dcVoltageCharacteristicSet())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid for this instrument")
}

func TestInstrumentCharacteristicValidate_NoCommands(t *testing.T) {
	c := InstrumentCharacteristic{
		Identifier: "source_voltage",
	}

	err := c.Validate(dcVoltageCharacteristicSet())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")
}

func TestInstrumentCharacteristicValidate_ReadValidationFailure(t *testing.T) {
	c := InstrumentCharacteristic{
		Identifier: "source_voltage",
		ReadCommand: APIParseCharacteristic{
			ParameterName: "voltage",
		},
	}

	err := c.Validate(dcVoltageCharacteristicSet())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "readCommand")
}

func TestInstrumentCharacteristicValidate_WriteValidationFailure(t *testing.T) {
	c := InstrumentCharacteristic{
		Identifier: "source_voltage",
		WriteCommand: APIParseCharacteristic{
			ParameterName: "voltage",
		},
	}

	err := c.Validate(dcVoltageCharacteristicSet())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "writeCommand")
}

func TestParseInstrumentType(t *testing.T) {
	got, err := ParseInstrumentType("dc_voltage_source")

	require.NoError(t, err)
	assert.Equal(t, instrument.DcVoltageSource, got)
}

func TestParseInstrumentTypeUnknown(t *testing.T) {
	_, err := ParseInstrumentType("banana")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown instrument type")
}

func validInstrumentConfig() InstrumentConfig {
	return InstrumentConfig{
		ConfigPath:         "/tmp/config.yml",
		PluginPath:         "/tmp/plugin.so",
		InstrumentTypeName: "dc_voltage_source",
		Characteristics: []InstrumentCharacteristic{
			{
				Identifier: "source_voltage",
				ReadCommand: APIParseCharacteristic{
					Command: "VOLT?",
				},
			},
		},
	}
}

func TestInstrumentConfigValidate(t *testing.T) {
	cfg := validInstrumentConfig()

	require.NoError(t, cfg.Validate())

	assert.Equal(
		t,
		instrument.DcVoltageSource,
		cfg.InstrumentType,
	)
}

func TestInstrumentConfigValidate_NoConfigPath(t *testing.T) {
	cfg := validInstrumentConfig()
	cfg.ConfigPath = ""

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
}

func TestInstrumentConfigValidate_NoPluginPath(t *testing.T) {
	cfg := validInstrumentConfig()
	cfg.PluginPath = ""

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is required")
}

func TestInstrumentConfigValidate_InvalidType(t *testing.T) {
	cfg := validInstrumentConfig()
	cfg.InstrumentTypeName = "nonsense"

	err := cfg.Validate()

	require.Error(t, err)
}

func TestInstrumentConfigValidate_CharacteristicError(t *testing.T) {
	cfg := validInstrumentConfig()

	cfg.Characteristics[0].Identifier = "sample_rate"

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "characteristics[0]")
}

func TestInstrumentServerConfigValidate(t *testing.T) {
	cfg := InstrumentServerConfig{
		Instruments: []InstrumentConfig{
			validInstrumentConfig(),
		},
	}

	require.NoError(t, cfg.Validate())
}

func TestInstrumentServerConfigValidate_NoInstruments(t *testing.T) {
	var cfg InstrumentServerConfig

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one instrument")
}

func TestInstrumentServerConfigValidate_WrapsErrors(t *testing.T) {
	cfg := InstrumentServerConfig{
		Instruments: []InstrumentConfig{
			{},
		},
	}

	err := cfg.Validate()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "instruments[0]")
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 8555, cfg.InstrumentServer.RPCPort)
	assert.True(t, cfg.InstrumentServer.AutoStart)
}

func TestLoadConfig(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cfg.yaml")

	require.NoError(t, os.WriteFile(file, []byte(`
instrument-server:
  instruments:
    - config: config.yml
      plugin: plugin.so
      type: dc_voltage_source
`), 0600))

	cfg, err := LoadConfig(file)

	require.NoError(t, err)
	require.NotNil(t, cfg)
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig("/does/not/exist")

	require.Error(t, err)
}

func TestLoadConfigBadYaml(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cfg.yaml")

	require.NoError(
		t,
		os.WriteFile(file, []byte(":\n:\n:\n"), 0600),
	)

	_, err := LoadConfig(file)

	require.Error(t, err)
}

func testWiremapEntries() []WiremapEntry {
	return []WiremapEntry{
		{
			PhysicalDeviceName: "P1",
			Instrument: WiremapInstrument{
				Name:         "Source1",
				ChannelGroup: "analog",
				Channel:      1,
			},
		},
		{
			PhysicalDeviceName: "P2",
			Instrument: WiremapInstrument{
				Name:         "Source1",
				ChannelGroup: "analog",
				Channel:      2,
			},
		},
	}
}

func TestResolveWiremap(t *testing.T) {
	tmpDir := t.TempDir()

	deviceConfig := writeTestDeviceConfig(t, tmpDir)

	wiremap := testWiremapEntries()

	err := ResolveWiremap(
		wiremap,
		deviceConfig,
	)

	require.NoError(t, err)

	for _, entry := range wiremap {
		assert.NotNil(t, entry.Gate)

		name, err := entry.Gate.Name()
		require.NoError(t, err)

		assert.Equal(
			t,
			entry.PhysicalDeviceName,
			name,
		)
	}
}

func TestValidate_WiremapUnknownGate(t *testing.T) {
	tmpDir := t.TempDir()

	deviceConfig := writeTestDeviceConfig(t, tmpDir)

	cfg := &HubConfig{
		QuantumDotConfig: deviceConfig,
		WorkingDirectory: tmpDir,
		Wiremap: []WiremapEntry{
			{
				PhysicalDeviceName: "BAD_GATE",
			},
		},
		InstrumentServer: InstrumentServerConfig{
			Instruments: []InstrumentConfig{
				validInstrumentConfig(),
			},
		},
	}

	err := Validate(cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "BAD_GATE")
}

func TestValidate_PopulatesWorkingDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	deviceConfig := writeTestDeviceConfig(t, tmpDir)

	cfg := &HubConfig{
		QuantumDotConfig: deviceConfig,
		InstrumentServer: InstrumentServerConfig{
			Instruments: []InstrumentConfig{
				validInstrumentConfig(),
			},
		},
	}

	err := Validate(cfg)

	require.NoError(t, err)
	assert.NotEmpty(t, cfg.WorkingDirectory)
}

func TestLoadConfig_FullConfig(t *testing.T) {
	tmp := t.TempDir()

	const (
		wiremapPath         = "wiremap.yaml"
		quantumDotConfig    = "quantum_dot.yaml"
		natsURL             = "nats://localhost:4222"
		localDatabase       = "/tmp/database"
		workingDirectory    = "/tmp/workdir"
		userMeasurementLuas = "/tmp/scripts"

		rpcPort = 9000

		instrument1Config = "instrument1.yaml"
		instrument1Plugin = "plugin1.so"

		instrument2Config = "instrument2.yaml"
		instrument2Plugin = "plugin2.so"
	)
	autostart := true

	cfgFile := filepath.Join(tmp, "hub.yaml")

	err := os.WriteFile(
		cfgFile,
		[]byte(fmt.Sprintf(
			`
wiremap: %s
quantum-dot-config: %s
nats-url: %s
local-database: %s
working-directory: %s
user-measurement-luas: %s

instrument-server:
  rpc-port: %d
  autostart: %t 

  instruments:
    - config: %s
      plugin: %s

    - config: %s
      plugin: %s
`,
			wiremapPath,
			quantumDotConfig,
			natsURL,
			localDatabase,
			workingDirectory,
			userMeasurementLuas,
			rpcPort,
			autostart,
			instrument1Config,
			instrument1Plugin,
			instrument2Config,
			instrument2Plugin,
		)),
		0644,
	)
	require.NoError(t, err)

	cfg, err := LoadConfig(cfgFile)
	require.NoError(t, err)

	assert.Equal(t, wiremapPath, cfg.Wiremap)
	assert.Equal(t, quantumDotConfig, cfg.QuantumDotConfig)
	assert.Equal(t, natsURL, cfg.NATSURL)
	assert.Equal(t, localDatabase, cfg.LocalDatabase)
	assert.Equal(t, workingDirectory, cfg.WorkingDirectory)
	assert.Equal(t, userMeasurementLuas, cfg.UserMeasurementLuasDir)

	assert.Equal(t, rpcPort, cfg.InstrumentServer.RPCPort)
	assert.Equal(t, autostart, cfg.InstrumentServer.AutoStart)

	require.Len(t, cfg.InstrumentServer.Instruments, 2)

	assert.Equal(
		t,
		instrument1Config,
		cfg.InstrumentServer.Instruments[0].ConfigPath,
	)
	assert.Equal(
		t,
		instrument1Plugin,
		cfg.InstrumentServer.Instruments[0].PluginPath,
	)

	assert.Equal(
		t,
		instrument2Config,
		cfg.InstrumentServer.Instruments[1].ConfigPath,
	)
	assert.Equal(
		t,
		instrument2Plugin,
		cfg.InstrumentServer.Instruments[1].PluginPath,
	)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()

	cfgFile := filepath.Join(tmp, "hub.yaml")

	err := os.WriteFile(
		cfgFile,
		[]byte(`
instrument-server:
  instruments:
    - config: foo
      plugin: bad
    - :
`),
		0644,
	)
	require.NoError(t, err)

	_, err = LoadConfig(cfgFile)

	require.Error(t, err)
}

func TestLoadConfig_FileDoesNotExist(t *testing.T) {
	_, err := LoadConfig("does_not_exist.yaml")

	require.Error(t, err)
}

func TestValidate_NoInstruments(t *testing.T) {
	cfg := DefaultConfig()

	tmpDir := t.TempDir()

	cfg.WorkingDirectory = tmpDir

	err := Validate(&cfg)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"at least one instrument is required",
	)
}

func TestValidate_MissingPlugin(t *testing.T) {
	cfg := DefaultConfig()

	cfg.WorkingDirectory = t.TempDir()

	cfg.InstrumentServer.Instruments = []InstrumentConfig{
		{
			ConfigPath: "config.yaml",
		},
	}

	err := Validate(&cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is required")
}

func TestValidate_MissingWorkingDirectory(t *testing.T) {
	cfg := DefaultConfig()

	cfg.WorkingDirectory = filepath.Join(
		t.TempDir(),
		"does-not-exist",
	)

	cfg.InstrumentServer.Instruments = []InstrumentConfig{
		{
			ConfigPath: "instrument.yaml", PluginPath: "plugin.so",
		},
	}

	err := Validate(&cfg)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"working directory does not exist",
	)
}
