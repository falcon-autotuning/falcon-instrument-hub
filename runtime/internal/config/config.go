//go:build cgo

package config

import (
	"fmt"
	"os"
	"path"

	"gopkg.in/yaml.v3"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
)

type Reducer string

const (
	ReducerNone Reducer = ""
	ReducerMin  Reducer = "min"
	ReducerMax  Reducer = "max"
)

func (r Reducer) Validate() error {
	switch r {
	case ReducerNone, ReducerMin, ReducerMax:
		return nil

	default:
		return fmt.Errorf(
			"unknown reducer %q",
			r,
		)
	}
}

type APIParseCharacteristic struct {
	Command string `yaml:"command,omitempty" json:"command,omitempty"`

	ParameterName string  `yaml:"parameterName,omitempty" json:"parameterName,omitempty"`
	Reducer       Reducer `yaml:"reducer,omitempty" json:"reducer,omitempty"`
}

func (c APIParseCharacteristic) Empty() bool {
	return c.Command == "" &&
		c.ParameterName == ""
}

func findParameter(
	api *InstrumentAPI,
	name string,
) (*IoType, bool) {
	for i := range api.IO {
		if api.IO[i].Name == name {
			return &api.IO[i], true
		}
	}

	for i := range api.ChannelGroups {
		for j := range api.ChannelGroups[i].IoTypes {
			io := &api.ChannelGroups[i].IoTypes[j]

			if io.Suffix == name {
				return io, true
			}
		}
	}

	return nil, false
}

func (c APIParseCharacteristic) Validate(
	api *InstrumentAPI,
) error {
	if err := c.Reducer.Validate(); err != nil {
		return err
	}

	switch {
	case c.Command != "":
		if c.ParameterName != "" {
			return fmt.Errorf(
				"cannot specify both command and parameterName",
			)
		}

		if c.Reducer != ReducerNone {
			return fmt.Errorf(
				"reducers are only valid with parameterName",
			)
		}

		if api != nil {
			if _, ok := api.Commands[CommandName(c.Command)]; !ok {
				return fmt.Errorf(
					"unknown API command %q",
					c.Command,
				)
			}
		}

		return nil

	case c.ParameterName != "":
		if api == nil {
			return fmt.Errorf(
				"unknown API parameter %q: no API provided",
				c.ParameterName,
			)
		}
		param, ok := findParameter(
			api,
			c.ParameterName,
		)
		if !ok {
			return fmt.Errorf(
				"unknown API parameter %q",
				c.ParameterName,
			)
		}
		switch c.Reducer {
		case ReducerMin:
			if param.Min == "" {
				return fmt.Errorf(
					"parameter %q does not define a minimum value",
					c.ParameterName,
				)
			}

		case ReducerMax:
			if param.Max == "" {
				return fmt.Errorf(
					"parameter %q does not define a maximum value",
					c.ParameterName,
				)
			}
		}

		return nil

	default:
		return fmt.Errorf(
			"either parameterName or command must be specified",
		)
	}
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
	Identifier     string                                            `yaml:"identifier" json:"identifier"`
	Characteristic instrumentcharacteristic.InstrumentCharacteristic `yaml:"-" json:"-"`

	ReadCommand  APIParseCharacteristic `yaml:"readCommand,omitempty" json:"readCommand"`
	WriteCommand APIParseCharacteristic `yaml:"writeCommand,omitempty" json:"writeCommand"`
}

func (c *InstrumentCharacteristic) Validate(
	api *InstrumentAPI,
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

	if c.ReadCommand.Empty() &&
		c.WriteCommand.Empty() {
		return fmt.Errorf(
			"at least one of readCommand or writeCommand must be specified",
		)
	}

	for name, command := range map[string]APIParseCharacteristic{
		"readCommand":  c.ReadCommand,
		"writeCommand": c.WriteCommand,
	} {
		if command.Empty() {
			continue
		}

		if err := command.Validate(api); err != nil {
			return fmt.Errorf(
				"%s: %w",
				name,
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
	ConfigPath         string                `yaml:"config" json:"config"`
	PluginPath         string                `yaml:"plugin" json:"plugin"`
	InstrumentTypeName string                `yaml:"type" json:"type"`
	InstrumentType     instrument.Instrument `yaml:"-" json:"-"`

	Characteristics []InstrumentCharacteristic `yaml:"characteristics" json:"characteristics"`

	ConfigFile *InstrumentConfigFile `yaml:"-" json:"-"`
	API        *InstrumentAPI        `yaml:"-" json:"-"`
}

func (i *InstrumentConfig) Validate() error {
	if i.ConfigPath == "" {
		return fmt.Errorf("config is required")
	}

	if i.PluginPath == "" {
		return fmt.Errorf("plugin is required")
	}

	var err error

	i.InstrumentType, err = ParseInstrumentType(
		i.InstrumentTypeName,
	)
	if err != nil {
		return err
	}

	return nil
}

func (i *InstrumentConfig) Resolve() error {
	if i.API != nil {
		return nil
	}
	instrumentConfig, err := ParseInstrumentConfig(
		i.ConfigPath,
	)
	if err != nil {
		return err
	}

	api, err := ParseInstrumentAPI(
		path.Join(path.Dir(i.ConfigPath), instrumentConfig.API_ref),
	)
	if err != nil {
		return err
	}
	i.ConfigFile = instrumentConfig
	i.API = api
	validCharacteristics := ValidCharacteristics[i.InstrumentType]

	for j := range i.Characteristics {
		characteristic := &i.Characteristics[j]

		if err := characteristic.Validate(
			i.API,
			validCharacteristics,
		); err != nil {
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
	RPCPort     int                `yaml:"rpc-port" json:"rpc-port"`
	AutoStart   bool               `yaml:"autostart" json:"autostart"`
	Instruments []InstrumentConfig `yaml:"instruments" json:"instruments"`
	ISSBinary   string             `yaml:"-" json:"-"`
}

func (s *InstrumentServerConfig) Validate() error {
	if len(s.Instruments) == 0 {
		return fmt.Errorf("at least one instrument is required")
	}

	for i := range s.Instruments {
		instrument := &s.Instruments[i]

		if err := instrument.Validate(); err != nil {
			return fmt.Errorf(
				"instruments[%d]: %w",
				i,
				err,
			)
		}

		if err := instrument.Resolve(); err != nil {
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
	// Logical to physical connection mappings
	Wiremap []WiremapEntry `yaml:"wiremap" json:"wiremap"`
	// Global Path to the quantum dot config describing the device loaded
	QuantumDotConfig string `yaml:"quantum-dot-config" json:"quantum-dot-config"`
	// NATSURL for establishing communications i.e. nats://derek:pass@localhost:4222
	NATSURL string `yaml:"nats-url" json:"nats-url"`
	// Global Path to the local runtime database for collected data
	LocalDatabase string `yaml:"local-database" json:"local-database"`
	// Global Path for location of all logging to be deposited
	WorkingDirectory string `yaml:"working-directory" json:"working-directory"`
	// Global Path to measurement scripts directory
	UserMeasurementLuasDir string `yaml:"user-measurement-luas" json:"user-measurement-luas"`
	// Configuration for the Instrument Script Server
	InstrumentServer InstrumentServerConfig `yaml:"instrument-server" json:"instrument-server"`
	RuntimePaths     RuntimePaths           `yaml:"-" json:"-"`
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
			return fmt.Errorf("could not get the current working directory: %s", err)
		}
	}

	if _, err := os.Stat(c.WorkingDirectory); os.IsNotExist(err) {
		return fmt.Errorf("working directory does not exist: %s", c.WorkingDirectory)
	}

	if err := c.InstrumentServer.Validate(); err != nil {
		return err
	}

	return nil
}
