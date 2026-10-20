package settings

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func measurementResult(
	unit string,
	value any,
) dispatcher.MeasurementResult {
	return dispatcher.MeasurementResult{
		Results: []instrumentserver.CallResult{
			{
				Return: []instrumentserver.ReturnValue{
					{
						Unit: unit,
						Value: instrumentserver.VariableValue{
							Value: value,
						},
					},
				},
			},
		},
	}
}

func TestDecodeSingleFloat_ResultError(
	t *testing.T,
) {
	expected := errors.New("boom")

	_, err := decodeSingleFloat(
		dispatcher.MeasurementResult{
			Err: expected,
		},
		falconcore.Volt,
	)

	assert.ErrorIs(t, err, expected)
}

func TestDecodeSingleFloat_Float64(
	t *testing.T,
) {
	q, err := decodeSingleFloat(
		measurementResult(
			"V",
			float64(7.5),
		),
		falconcore.Volt,
	)

	require.NoError(t, err)

	assert.Equal(t, 7.5, q.Value)
	assert.Equal(t, falconcore.Volt, q.Unit)
}

func TestDecodeSingleFloat_Float32(
	t *testing.T,
) {
	q, err := decodeSingleFloat(
		measurementResult(
			"V",
			float32(7.5),
		),
		falconcore.Volt,
	)

	require.NoError(t, err)

	assert.Equal(t, 7.5, q.Value)
}

func TestDecodeSingleFloat_WrongUnit(
	t *testing.T,
) {
	_, err := decodeSingleFloat(
		measurementResult(
			"A",
			float64(7),
		),
		falconcore.Volt,
	)

	require.Error(t, err)
}

func TestDecodeSingleFloat_WrongType(
	t *testing.T,
) {
	_, err := decodeSingleFloat(
		measurementResult(
			"V",
			"hello",
		),
		falconcore.Volt,
	)

	require.Error(t, err)
}

func TestDecodeSingleFloat_InvalidUnit(
	t *testing.T,
) {
	_, err := decodeSingleFloat(
		measurementResult(
			"not-a-unit",
			float64(1),
		),
		falconcore.Volt,
	)

	require.Error(t, err)
}

func TestDecodeSingleInt_Int(
	t *testing.T,
) {
	q, err := decodeSingleInt(
		measurementResult(
			"",
			int64(123),
		),
		falconcore.Dimensionless,
	)

	require.NoError(t, err)
	assert.Equal(t, 123.0, q.Value)
}

func TestDecodeSingleInt_Uint(
	t *testing.T,
) {
	q, err := decodeSingleInt(
		measurementResult(
			"",
			uint32(123),
		),
		falconcore.Dimensionless,
	)

	require.NoError(t, err)
	assert.Equal(t, 123.0, q.Value)
}

func TestDecodeSingleInt_ResultError(
	t *testing.T,
) {
	expected := errors.New("boom")

	_, err := decodeSingleInt(
		dispatcher.MeasurementResult{
			Err: expected,
		},
		falconcore.Dimensionless,
	)

	assert.ErrorIs(t, err, expected)
}

func TestDecodeSingleInt_NoResults(
	t *testing.T,
) {
	_, err := decodeSingleInt(
		dispatcher.MeasurementResult{},
		falconcore.Dimensionless,
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"measurement returned no results",
	)
}

func TestDecodeSingleInt_WrongType(
	t *testing.T,
) {
	_, err := decodeSingleInt(
		measurementResult(
			"",
			"hello",
		),
		falconcore.Dimensionless,
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"no numeric value found",
	)
}

func TestDecodeSingleInt_WrongUnit(
	t *testing.T,
) {
	_, err := decodeSingleInt(
		measurementResult(
			"V",
			int64(123),
		),
		falconcore.Ampere,
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"no numeric value",
	)
}

func TestCharacteristicDefinitions_Valid(
	t *testing.T,
) {
	for characteristic, def := range getterCharacteristicDefinitions {

		assert.Equal(
			t,
			characteristic,
			def.Characteristic,
		)

		assert.NotEmpty(
			t,
			def.Script,
		)

		assert.NotNil(
			t,
			def.Decode,
		)

		assert.Contains(
			t,
			def.Script,
			".lua",
		)
	}
}

func TestCharacteristicDefinitions_NumberOfSamplesUsesIntDecoder(
	t *testing.T,
) {
	def := getterCharacteristicDefinitions[instrumentcharacteristic.InstrumentCharacteristicNumberOfSamples]

	q, err := def.Decode(
		measurementResult(
			"",
			int64(42),
		),
		falconcore.Dimensionless,
	)

	require.NoError(t, err)

	assert.Equal(t, 42.0, q.Value)
}

func TestCharacteristicDefinitions_NotEmpty(
	t *testing.T,
) {
	assert.NotZero(
		t,
		len(getterCharacteristicDefinitions),
	)
}

func TestDecodeSingleInt_InvalidUnit(
	t *testing.T,
) {
	_, err := decodeSingleInt(
		measurementResult(
			"not-a-unit",
			int64(123),
		),
		falconcore.Dimensionless,
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"no numeric value found",
	)
}
