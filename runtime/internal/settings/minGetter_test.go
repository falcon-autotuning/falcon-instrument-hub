package settings

import (
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMinCharacteristicGetter(t *testing.T) {
	g := NewMinCharacteristicGetter()

	require.NotNil(t, g)
}

func TestMinCharacteristicGetter_Name(t *testing.T) {
	g := NewMinCharacteristicGetter()

	assert.Equal(
		t,
		"MinCharacteristicGetter",
		g.Name(),
	)
}

func TestMinLimit_Metadata(t *testing.T) {
	assert.Equal(
		t,
		"min",
		minLimit.name,
	)

	assert.Equal(
		t,
		config.ReducerMin,
		minLimit.reducer,
	)

	_, ok := minLimit.characteristics[instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage]

	assert.True(t, ok)
}

func TestMinLimit_GetValue(t *testing.T) {
	io := &config.IoType{
		Min: "-123",
		Max: "456",
	}

	assert.Equal(
		t,
		"-123",
		minLimit.getValue(io),
	)
}

func TestMinCharacteristicGetter_CanGet_True(t *testing.T) {
	g := NewMinCharacteristicGetter()

	port := testLimitPort()
	port.Characteristic = instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage

	ports := testLimitPorts()
	ports.AllConnections[0].Characteristic = instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage

	metadata := testLimitMetadata()

	meta := metadata["DAC1"]

	meta.Characteristics[0].Characteristic = instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage

	meta.Characteristics[0].ReadCommand.Reducer = config.ReducerMin

	meta.API.IO[0].Min = "-10"

	metadata["DAC1"] = meta

	assert.True(
		t,
		g.CanGet(
			port,
			metadata,
			ports,
			"",
		),
	)
}

func TestMinCharacteristicGetter_CanGet_False(t *testing.T) {
	g := NewMinCharacteristicGetter()

	assert.False(
		t,
		g.CanGet(
			testLimitPort(),
			testLimitMetadata(),
			nil,
			"",
		),
	)
}

func TestMinCharacteristicGetter_Get_Success(t *testing.T) {
	g := NewMinCharacteristicGetter()

	port := testLimitPort()
	port.Characteristic = instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage

	ports := testLimitPorts()
	ports.AllConnections[0].Characteristic = instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage

	metadata := testLimitMetadata()

	meta := metadata["DAC1"]

	meta.Characteristics[0].Characteristic = instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage

	meta.Characteristics[0].ReadCommand.Reducer = config.ReducerMin

	meta.API.IO[0].Min = "-10"

	metadata["DAC1"] = meta

	q, err := g.Get(
		port,
		nil,
		nil,
		ports,
		metadata,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		falconcore.Quantity{
			Value: -10,
			Unit:  falconcore.Volt,
		},
		q,
	)
}

func TestMinCharacteristicGetter_Get_Error(t *testing.T) {
	g := NewMinCharacteristicGetter()

	_, err := g.Get(
		testLimitPort(),
		nil,
		nil,
		nil,
		testLimitMetadata(),
	)

	require.Error(t, err)
}
