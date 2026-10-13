package settings

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
)

type ValueEncoder func(
	falconcore.Quantity,
) (instrumentserver.VariableValue, error)

type SetterCharacteristicDefinition struct {
	Characteristic instrumentcharacteristic.InstrumentCharacteristic

	Script string

	ValueName string

	Encode ValueEncoder
}

func encodeFloat(
	q falconcore.Quantity,
) (instrumentserver.VariableValue, error) {
	return instrumentserver.VariableValue{
		Value: q.Value,
	}, nil
}

func encodeInt(
	q falconcore.Quantity,
) (instrumentserver.VariableValue, error) {
	return instrumentserver.VariableValue{
		Value: int64(q.Value),
	}, nil
}

var setterCharacteristicDefinitions = map[instrumentcharacteristic.InstrumentCharacteristic]SetterCharacteristicDefinition{
	instrumentcharacteristic.InstrumentCharacteristicSampleRate: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSampleRate,
		Script:         "set_sample_rate.lua",
		ValueName:      "sampleRate",
		Encode:         encodeFloat,
	},

	instrumentcharacteristic.InstrumentCharacteristicSourceVoltage: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
		Script:         "set_source_voltage.lua",
		ValueName:      "setVoltage",
		Encode:         encodeFloat,
	},

	instrumentcharacteristic.InstrumentCharacteristicVoltageRampSlope: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicVoltageRampSlope,
		Script:         "set_slope.lua",
		ValueName:      "slope",
		Encode:         encodeFloat,
	},
	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicSourceCurrent: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceCurrent,
		Script:         "set_source_current.lua",
		ValueName:      "value",
		Encode:         encodeFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicCurrentRampSlope: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicCurrentRampSlope,
		Script:         "set_current_ramp_slope.lua",
		ValueName:      "value",
		Encode:         encodeFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicSourceFrequency: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceFrequency,
		Script:         "set_source_frequency.lua",
		ValueName:      "value",
		Encode:         encodeFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicSinkFrequency: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSinkFrequency,
		Script:         "set_sink_frequency.lua",
		ValueName:      "value",
		Encode:         encodeFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicAcAmplitude: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicAcAmplitude,
		Script:         "set_ac_amplitude.lua",
		ValueName:      "value",
		Encode:         encodeFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicTemperature: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicTemperature,
		Script:         "set_temperature.lua",
		ValueName:      "value",
		Encode:         encodeFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicMagnetStrength: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicMagnetStrength,
		Script:         "set_magnet_strength.lua",
		ValueName:      "value",
		Encode:         encodeFloat,
	},

	instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples,
		Script:         "set_number_of_samples.lua",
		ValueName:      "numberOfSamples",
		Encode:         encodeInt,
	},
}
