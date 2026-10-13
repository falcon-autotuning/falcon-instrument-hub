//go:build cgo

package settings

import (
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeFloat(
	t *testing.T,
) {
	q := falconcore.Quantity{
		Value: 123.456,
		Unit:  falconcore.Volt,
	}

	out, err := encodeFloat(q)

	require.NoError(t, err)

	value, ok := out.Value.(float64)

	require.True(t, ok)

	assert.Equal(
		t,
		123.456,
		value,
	)
}

func TestEncodeInt(
	t *testing.T,
) {
	q := falconcore.Quantity{
		Value: 123.9,
		Unit:  falconcore.Dimensionless,
	}

	out, err := encodeInt(q)

	require.NoError(t, err)

	value, ok := out.Value.(int64)

	require.True(t, ok)

	assert.Equal(
		t,
		int64(123),
		value,
	)
}

func TestSetterCharacteristicDefinitions_NotEmpty(
	t *testing.T,
) {
	assert.NotZero(
		t,
		len(setterCharacteristicDefinitions),
	)
}

func TestSetterCharacteristicDefinitions_Valid(
	t *testing.T,
) {
	for characteristic, def := range setterCharacteristicDefinitions {

		assert.Equal(
			t,
			characteristic,
			def.Characteristic,
		)

		assert.NotEmpty(
			t,
			def.Script,
		)

		assert.NotEmpty(
			t,
			def.ValueName,
		)

		assert.NotNil(
			t,
			def.Encode,
		)

		assert.Contains(
			t,
			def.Script,
			"set_",
		)

		assert.Contains(
			t,
			def.Script,
			".lua",
		)
	}
}

func TestSetterCharacteristicDefinitions_SourceVoltage(
	t *testing.T,
) {
	def := setterCharacteristicDefinitions[instrumentcharacteristic.InstrumentCharacteristicSourceVoltage]

	assert.Equal(
		t,
		"set_source_voltage.lua",
		def.Script,
	)

	assert.Equal(
		t,
		"setVoltage",
		def.ValueName,
	)
}

func TestSetterCharacteristicDefinitions_NumberOfSamples(
	t *testing.T,
) {
	def := setterCharacteristicDefinitions[instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples]

	assert.Equal(
		t,
		"set_number_of_samples.lua",
		def.Script,
	)

	assert.Equal(
		t,
		"numberOfSamples",
		def.ValueName,
	)

	out, err := def.Encode(
		falconcore.Quantity{
			Value: 42,
		},
	)

	require.NoError(t, err)

	value, ok := out.Value.(int64)

	require.True(t, ok)

	assert.Equal(
		t,
		int64(42),
		value,
	)
}

func TestSetterCharacteristicDefinitions_FloatEncoders(
	t *testing.T,
) {
	tests := []instrumentcharacteristic.InstrumentCharacteristic{
		instrumentcharacteristic.InstrumentCharacteristicSampleRate,
		instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
		instrumentcharacteristic.InstrumentCharacteristicVoltageRampSlope,
		instrumentcharacteristic.InstrumentCharacteristicSourceCurrent,
		instrumentcharacteristic.InstrumentCharacteristicCurrentRampSlope,
		instrumentcharacteristic.InstrumentCharacteristicSourceFrequency,
		instrumentcharacteristic.InstrumentCharacteristicSinkFrequency,
		instrumentcharacteristic.InstrumentCharacteristicAcAmplitude,
		instrumentcharacteristic.InstrumentCharacteristicTemperature,
		instrumentcharacteristic.InstrumentCharacteristicMagnetStrength,
	}

	for _, characteristic := range tests {

		def := setterCharacteristicDefinitions[characteristic]

		out, err := def.Encode(
			falconcore.Quantity{
				Value: 12.5,
			},
		)

		require.NoError(
			t,
			err,
		)

		value, ok := out.Value.(float64)

		require.True(
			t,
			ok,
		)

		assert.Equal(
			t,
			12.5,
			value,
		)
	}
}
