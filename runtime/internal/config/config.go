//go:build cgo

package config

import (
	"fmt"
	"os"
	"os/exec"

	"gopkg.in/yaml.v3"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
)

var execCommand = exec.Command

type APIParseCharacteristic struct {
	Command       string `yaml:"command,omitempty"`
	ParameterName string `yaml:"parameterName,omitempty"`
	Max           bool   `yaml:"max,omitempty"`
	Min           bool   `yaml:"min,omitempty"`
}

func (c APIParseCharacteristic) Validate() error {
	if c.Command != "" {
		return nil
	}

	if c.ParameterName != "" {
		if c.Min || c.Max {
			return nil
		}

		return fmt.Errorf(
			"parameter parsing requires min, max, or both to be specified",
		)
	}

	return fmt.Errorf(
		"either parameterName or command must be specified",
	)
}

var validCharacteristicNames = map[string]instrumentcharacteristic.InstrumentCharacteristic{
	"sample_rate":     instrumentcharacteristic.InstrumentCharacteristicSampleRate,
	"max_sample_rate": instrumentcharacteristic.InstrumentCharacteristicMaxSampleRate,
	"min_sample_rate": instrumentcharacteristic.InstrumentCharacteristicMinSampleRate,

	"source_voltage":     instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
	"max_source_voltage": instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage,
	"min_source_voltage": instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage,

	"voltage_ramp_slope":     instrumentcharacteristic.InstrumentCharacteristicVoltageRampSlope,
	"max_voltage_ramp_slope": instrumentcharacteristic.InstrumentCharacteristicMaxVoltageRampSlope,
	"min_voltage_ramp_slope": instrumentcharacteristic.InstrumentCharacteristicMinVoltageRampSlope,

	"source_current":     instrumentcharacteristic.InstrumentCharacteristicSourceCurrent,
	"max_source_current": instrumentcharacteristic.InstrumentCharacteristicMaxSourceCurrent,
	"min_source_current": instrumentcharacteristic.InstrumentCharacteristicMinSourceCurrent,

	"current_ramp_slope":     instrumentcharacteristic.InstrumentCharacteristicCurrentRampSlope,
	"max_current_ramp_slope": instrumentcharacteristic.InstrumentCharacteristicMaxCurrentRampSlope,
	"min_current_ramp_slope": instrumentcharacteristic.InstrumentCharacteristicMinCurrentRampSlope,

	"source_frequency":     instrumentcharacteristic.InstrumentCharacteristicSourceFrequency,
	"max_source_frequency": instrumentcharacteristic.InstrumentCharacteristicMaxSourceFrequency,
	"min_source_frequency": instrumentcharacteristic.InstrumentCharacteristicMinSourceFrequency,

	"sink_frequency":     instrumentcharacteristic.InstrumentCharacteristicSinkFrequency,
	"max_sink_frequency": instrumentcharacteristic.InstrumentCharacteristicMaxSinkFrequency,
	"min_sink_frequency": instrumentcharacteristic.InstrumentCharacteristicMinSinkFrequency,

	"ac_amplitude":     instrumentcharacteristic.InstrumentCharacteristicAcAmplitude,
	"max_ac_amplitude": instrumentcharacteristic.InstrumentCharacteristicMaxAcAmplitude,
	"min_ac_amplitude": instrumentcharacteristic.InstrumentCharacteristicMinAcAmplitude,

	"number_of_samples":     instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples,
	"max_number_of_samples": instrumentcharacteristic.InstrumentCharacteristicMaxNumberOfSamples,
	"min_number_of_samples": instrumentcharacteristic.InstrumentCharacteristicMinNumberOfSamples,

	"temperature": instrumentcharacteristic.InstrumentCharacteristicTemperature,

	"magnet_strength":     instrumentcharacteristic.InstrumentCharacteristicMagnetStrength,
	"max_magnet_strength": instrumentcharacteristic.InstrumentCharacteristicMaxMagnetStrength,
	"min_magnet_strength": instrumentcharacteristic.InstrumentCharacteristicMinMagnetStrength,
}

func ParseCharacteristic(
	s string,
) (instrumentcharacteristic.InstrumentCharacteristic, error) {
	v, ok := validCharacteristicNames[s]
	if !ok {
		return 0, fmt.Errorf("unknown characteristic %q", s)
	}
	return v, nil
}

type CharacteristicSet map[instrumentcharacteristic.InstrumentCharacteristic]struct{}

var ValidCharacteristics = map[instrument.Instrument]CharacteristicSet{
	instrument.DcVoltageSource: {
		instrumentcharacteristic.InstrumentCharacteristicSourceVoltage:       {},
		instrumentcharacteristic.InstrumentCharacteristicVoltageRampSlope:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage:    {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxVoltageRampSlope: {},
		instrumentcharacteristic.InstrumentCharacteristicMinVoltageRampSlope: {},
	},

	instrument.VoltageSource: {
		instrumentcharacteristic.InstrumentCharacteristicSourceVoltage:       {},
		instrumentcharacteristic.InstrumentCharacteristicVoltageRampSlope:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage:    {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxVoltageRampSlope: {},
		instrumentcharacteristic.InstrumentCharacteristicMinVoltageRampSlope: {},
	},

	instrument.CurrentSource: {
		instrumentcharacteristic.InstrumentCharacteristicSourceCurrent:       {},
		instrumentcharacteristic.InstrumentCharacteristicCurrentRampSlope:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceCurrent:    {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceCurrent:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxCurrentRampSlope: {},
		instrumentcharacteristic.InstrumentCharacteristicMinCurrentRampSlope: {},
	},

	instrument.DcCurrentSource: {
		instrumentcharacteristic.InstrumentCharacteristicSourceCurrent:       {},
		instrumentcharacteristic.InstrumentCharacteristicCurrentRampSlope:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceCurrent:    {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceCurrent:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxCurrentRampSlope: {},
		instrumentcharacteristic.InstrumentCharacteristicMinCurrentRampSlope: {},
	},

	instrument.HfVoltageSource: {
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage:   {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage:   {},
		instrumentcharacteristic.InstrumentCharacteristicAcAmplitude:        {},
		instrumentcharacteristic.InstrumentCharacteristicMaxAcAmplitude:     {},
		instrumentcharacteristic.InstrumentCharacteristicMinAcAmplitude:     {},
		instrumentcharacteristic.InstrumentCharacteristicSourceFrequency:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceFrequency: {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceFrequency: {},
	},

	instrument.HfCurrentSource: {
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceCurrent:   {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceCurrent:   {},
		instrumentcharacteristic.InstrumentCharacteristicAcAmplitude:        {},
		instrumentcharacteristic.InstrumentCharacteristicMaxAcAmplitude:     {},
		instrumentcharacteristic.InstrumentCharacteristicMinAcAmplitude:     {},
		instrumentcharacteristic.InstrumentCharacteristicSourceFrequency:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceFrequency: {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceFrequency: {},
	},

	instrument.Amnmeter: {
		instrumentcharacteristic.InstrumentCharacteristicSampleRate:         {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSampleRate:      {},
		instrumentcharacteristic.InstrumentCharacteristicMinSampleRate:      {},
		instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxNumberOfSamples: {},
		instrumentcharacteristic.InstrumentCharacteristicMinNumberOfSamples: {},
	},

	instrument.Voltmeter: {
		instrumentcharacteristic.InstrumentCharacteristicSampleRate:         {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSampleRate:      {},
		instrumentcharacteristic.InstrumentCharacteristicMinSampleRate:      {},
		instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxNumberOfSamples: {},
		instrumentcharacteristic.InstrumentCharacteristicMinNumberOfSamples: {},
	},

	instrument.Lockin: {
		instrumentcharacteristic.InstrumentCharacteristicAcAmplitude:        {},
		instrumentcharacteristic.InstrumentCharacteristicMaxAcAmplitude:     {},
		instrumentcharacteristic.InstrumentCharacteristicMinAcAmplitude:     {},
		instrumentcharacteristic.InstrumentCharacteristicSourceFrequency:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceFrequency: {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceFrequency: {},
		instrumentcharacteristic.InstrumentCharacteristicSinkFrequency:      {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSinkFrequency:   {},
		instrumentcharacteristic.InstrumentCharacteristicMinSinkFrequency:   {},
		instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxNumberOfSamples: {},
		instrumentcharacteristic.InstrumentCharacteristicMinNumberOfSamples: {},
	},

	instrument.Magnet: {
		instrumentcharacteristic.InstrumentCharacteristicMagnetStrength:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxMagnetStrength: {},
		instrumentcharacteristic.InstrumentCharacteristicMinMagnetStrength: {},
	},

	instrument.Thermometer: {
		instrumentcharacteristic.InstrumentCharacteristicTemperature: {},
	},

	instrument.Discrete: {
		instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples: {},
	},

	instrument.Fpga: {
		instrumentcharacteristic.InstrumentCharacteristicSampleRate:         {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSampleRate:      {},
		instrumentcharacteristic.InstrumentCharacteristicMinSampleRate:      {},
		instrumentcharacteristic.InstrumentCharacteristicAcAmplitude:        {},
		instrumentcharacteristic.InstrumentCharacteristicMaxAcAmplitude:     {},
		instrumentcharacteristic.InstrumentCharacteristicMinAcAmplitude:     {},
		instrumentcharacteristic.InstrumentCharacteristicSourceFrequency:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSourceFrequency: {},
		instrumentcharacteristic.InstrumentCharacteristicMinSourceFrequency: {},
		instrumentcharacteristic.InstrumentCharacteristicSinkFrequency:      {},
		instrumentcharacteristic.InstrumentCharacteristicMaxSinkFrequency:   {},
		instrumentcharacteristic.InstrumentCharacteristicMinSinkFrequency:   {},
		instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples:    {},
		instrumentcharacteristic.InstrumentCharacteristicMaxNumberOfSamples: {},
		instrumentcharacteristic.InstrumentCharacteristicMinNumberOfSamples: {},
	},
}

type InstrumentCharacteristic struct {
	Identifier string `yaml:"identifier"`

	Characteristic instrumentcharacteristic.InstrumentCharacteristic `yaml:"-"`

	ReadCommand  APIParseCharacteristic `yaml:"readCommand,omitempty"`
	WriteCommand APIParseCharacteristic `yaml:"writeCommand,omitempty"`
}

func (c *InstrumentCharacteristic) Validate(
	validChars CharacteristicSet,
) error {
	parsed, err := ParseCharacteristic(c.Identifier)
	if err != nil {
		return err
	}

	if _, ok := validChars[parsed]; !ok {
		return fmt.Errorf(
			"characteristic %q is not valid for this instrument type",
			c.Identifier,
		)
	}

	c.Characteristic = parsed

	if c.ReadCommand.Command == "" &&
		c.ReadCommand.ParameterName == "" &&
		c.WriteCommand.Command == "" &&
		c.WriteCommand.ParameterName == "" {
		return fmt.Errorf(
			"at least one of readCommand or writeCommand must be specified",
		)
	}

	if c.ReadCommand.Command != "" ||
		c.ReadCommand.ParameterName != "" {
		if err := c.ReadCommand.Validate(); err != nil {
			return fmt.Errorf(
				"readCommand: %w",
				err,
			)
		}
	}

	if c.WriteCommand.Command != "" ||
		c.WriteCommand.ParameterName != "" {
		if err := c.WriteCommand.Validate(); err != nil {
			return fmt.Errorf(
				"writeCommand: %w",
				err,
			)
		}
	}

	return nil
}

var validInstrumentTypes = map[string]instrument.Instrument{
	"dc_voltage_source": instrument.DcVoltageSource,
	"amnmeter":          instrument.Amnmeter,
	"magnet":            instrument.Magnet,
	"lockin":            instrument.Lockin,
	"voltage_source":    instrument.VoltageSource,
	"current_source":    instrument.CurrentSource,
	"hf_voltage_source": instrument.HfVoltageSource,
	"dc_current_source": instrument.DcCurrentSource,
	"hf_current_source": instrument.HfCurrentSource,
	"thermometer":       instrument.Thermometer,
	"voltmeter":         instrument.Voltmeter,
	"fpga":              instrument.Fpga,
	"clock":             instrument.Clock,
	"discrete":          instrument.Discrete,
}

func ParseInstrumentType(s string) (instrument.Instrument, error) {
	v, ok := validInstrumentTypes[s]
	if !ok {
		return 0, fmt.Errorf("unknown instrument type %q", s)
	}
	return v, nil
}

type InstrumentConfig struct {
	ConfigPath         string                     `yaml:"config"`
	PluginPath         string                     `yaml:"plugin"`
	InstrumentTypeName string                     `yaml:"type"`
	InstrumentType     instrument.Instrument      `yaml:"-"`
	Characteristics    []InstrumentCharacteristic `yaml:"characteristics"`
}

func (i *InstrumentConfig) Validate() error {
	if i.ConfigPath == "" {
		return fmt.Errorf("config is required")
	}

	if i.PluginPath == "" {
		return fmt.Errorf("plugin is required")
	}
	var err error
	i.InstrumentType, err = ParseInstrumentType(i.InstrumentTypeName)
	if err != nil {
		return err
	}
	validCharacteristics := ValidCharacteristics[i.InstrumentType]
	for j, characteristic := range i.Characteristics {
		if err := characteristic.Validate(validCharacteristics); err != nil {
			return fmt.Errorf(
				"characteristics[%d]: %w",
				j,
				err,
			)
		}
	}

	return nil
}

type InstrumentServerConfig struct {
	RPCPort     int                `yaml:"rpc-port"`
	AutoStart   bool               `yaml:"autostart"`
	Instruments []InstrumentConfig `yaml:"instruments"`
	ISSBinary   string             `yaml:"-"`
}

func (s *InstrumentServerConfig) Validate() error {
	if len(s.Instruments) == 0 {
		return fmt.Errorf("at least one instrument is required")
	}

	for i := range s.Instruments {
		if err := s.Instruments[i].Validate(); err != nil {
			return fmt.Errorf(
				"instruments[%d]: %w",
				i,
				err,
			)
		}
	}

	return nil
}

type RuntimePaths struct {
	Logs      string
	Data      string
	DataCache string
}

type HubConfig struct {
	Wiremap                []WiremapEntry         `yaml:"wiremap"`
	QuantumDotConfig       string                 `yaml:"quantum-dot-config"`
	NATSURL                string                 `yaml:"nats-url"`
	LocalDatabase          string                 `yaml:"local-database"`
	WorkingDirectory       string                 `yaml:"working-directory"`
	UserMeasurementLuasDir string                 `yaml:"user-measurement-luas"`
	InstrumentServer       InstrumentServerConfig `yaml:"instrument-server"`
	RuntimePaths           RuntimePaths           `yaml:"-"`
}

func DefaultConfig() HubConfig {
	return HubConfig{
		InstrumentServer: InstrumentServerConfig{
			RPCPort:   8555,
			AutoStart: true,
		},
	}
}

func LoadConfig(path string) (*HubConfig, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func Validate(c *HubConfig) error {
	if c.QuantumDotConfig != "" {
		if _, err := os.Stat(c.QuantumDotConfig); os.IsNotExist(err) {
			return fmt.Errorf("device config file does not exist: %s", c.QuantumDotConfig)
		}
	}

	if c.UserMeasurementLuasDir != "" {
		if _, err := os.Stat(c.UserMeasurementLuasDir); os.IsNotExist(err) {
			return fmt.Errorf("the measurement luas dir does not exist: %s", c.UserMeasurementLuasDir)
		}
	}

	if err := ResolveWiremap(
		c.Wiremap,
		c.QuantumDotConfig,
	); err != nil {
		return err
	}

	if c.WorkingDirectory == "" {
		var err error
		c.WorkingDirectory, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("Could not get the current working directory: %s", err)
		}
	}
	if _, err := os.Stat(c.WorkingDirectory); os.IsNotExist(err) {
		return fmt.Errorf("working directory does not exist: %s", c.WorkingDirectory)
	}

	if err := c.InstrumentServer.Validate(); err != nil {
		return err
	}

	// TODO: Validate APIParseCharacteristic using ParseInstrumentAPI

	return nil
}
