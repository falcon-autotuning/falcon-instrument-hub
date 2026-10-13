package settings

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

type ResultDecoder func(
	dispatcher.MeasurementResult,
	falconcore.Unit,
) (falconcore.Quantity, error)

type CharacteristicDefinition struct {
	Characteristic instrumentcharacteristic.InstrumentCharacteristic

	Script string

	Decode ResultDecoder
}

func decodeSingleFloat(
	result dispatcher.MeasurementResult,
	unit falconcore.Unit,
) (falconcore.Quantity, error) {
	if result.Err != nil {
		return falconcore.Quantity{}, result.Err
	}

	if len(result.Results) == 0 {
		return falconcore.Quantity{},
			fmt.Errorf("measurement returned no results")
	}

	for _, call := range result.Results {
		for _, ret := range call.Return {

			returnUnit, err := falconcore.SymbolToUnit(
				ret.Unit,
			)
			if err != nil {
				continue
			}

			if returnUnit != unit {
				continue
			}

			switch value := ret.Value.Value.(type) {

			case float64:
				return falconcore.Quantity{
					Value: value,
					Unit:  unit,
				}, nil

			case float32:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			}
		}
	}

	return falconcore.Quantity{},
		fmt.Errorf(
			"no numeric value found with unit %v+",
			unit,
		)
}

func decodeSingleInt(
	result dispatcher.MeasurementResult,
	unit falconcore.Unit,
) (falconcore.Quantity, error) {
	if result.Err != nil {
		return falconcore.Quantity{}, result.Err
	}

	if len(result.Results) == 0 {
		return falconcore.Quantity{},
			fmt.Errorf("measurement returned no results")
	}

	for _, call := range result.Results {
		for _, ret := range call.Return {

			returnUnit, err := falconcore.SymbolToUnit(
				ret.Unit,
			)
			if err != nil {
				continue
			}

			if returnUnit != unit {
				continue
			}

			switch value := ret.Value.Value.(type) {

			case int:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			case int8:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			case int16:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			case int32:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			case int64:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			case uint:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			case uint8:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			case uint16:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			case uint32:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil

			case uint64:
				return falconcore.Quantity{
					Value: float64(value),
					Unit:  unit,
				}, nil
			}
		}
	}

	return falconcore.Quantity{},
		fmt.Errorf(
			"no numeric value found with unit %v+",
			unit,
		)
}

var getterCharacteristicDefinitions = map[instrumentcharacteristic.InstrumentCharacteristic]CharacteristicDefinition{
	instrumentcharacteristic.InstrumentCharacteristicSampleRate: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSampleRate,
		Script:         "get_sample_rate.lua",
		Decode:         decodeSingleFloat,
	},

	instrumentcharacteristic.InstrumentCharacteristicSourceVoltage: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
		Script:         "get_source_voltage.lua",
		Decode:         decodeSingleFloat,
	},

	instrumentcharacteristic.InstrumentCharacteristicVoltageRampSlope: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicVoltageRampSlope,
		Script:         "get_slope.lua",
		Decode:         decodeSingleFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicSourceCurrent: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceCurrent,
		Script:         "get_source_current.lua",
		Decode:         decodeSingleFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicCurrentRampSlope: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicCurrentRampSlope,
		Script:         "get_current_ramp_slope.lua",
		Decode:         decodeSingleFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicSourceFrequency: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceFrequency,
		Script:         "get_source_frequency.lua",
		Decode:         decodeSingleFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicSinkFrequency: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSinkFrequency,
		Script:         "get_sink_frequency.lua",
		Decode:         decodeSingleFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicAcAmplitude: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicAcAmplitude,
		Script:         "get_ac_amplitude.lua",
		Decode:         decodeSingleFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicTemperature: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicTemperature,
		Script:         "get_temperature.lua",
		Decode:         decodeSingleFloat,
	},

	// NOTE: Script not fully implemented yet
	instrumentcharacteristic.InstrumentCharacteristicMagnetStrength: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicMagnetStrength,
		Script:         "get_magnet_strength.lua",
		Decode:         decodeSingleFloat,
	},

	instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples: {
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples,
		Script:         "get_number_of_samples.lua",
		Decode:         decodeSingleInt,
	},
}
