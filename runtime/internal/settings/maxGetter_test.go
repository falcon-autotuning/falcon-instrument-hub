package settings

import (
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMaxCharacteristicGetter(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	require.NotNil(t, g)
}

func TestMaxCharacteristicGetter_Name(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	assert.Equal(
		t,
		"MaxCharacteristicGetter",
		g.Name(),
	)
}

func TestMaxLimit_GetValue(t *testing.T) {
	io := &config.IoType{
		Max: "123",
		Min: "456",
	}

	assert.Equal(
		t,
		"123",
		maxLimit.getValue(io),
	)
}

func TestMaxCharacteristicGetter_CanGet_True(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	assert.True(
		t,
		g.CanGet(
			testLimitPort(),
			testLimitMetadata(),
			testLimitPorts(),
		),
	)
}

func TestMaxCharacteristicGetter_CanGet_False(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	assert.False(
		t,
		g.CanGet(
			testLimitPort(),
			testLimitMetadata(),
			&config.ConnectedPorts{},
		),
	)
}

func TestMaxCharacteristicGetter_Get_Success(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	q, err := g.Get(
		testLimitPort(),
		nil,
		nil,
		testLimitPorts(),
		testLimitMetadata(),
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		falconcore.Quantity{
			Value: 10,
			Unit:  falconcore.Volt,
		},
		q,
	)
}

func TestMaxCharacteristicGetter_Get_Error(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	_, err := g.Get(
		testLimitPort(),
		nil,
		nil,
		&config.ConnectedPorts{},
		testLimitMetadata(),
	)

	require.Error(t, err)
}

func TestMaxLimit_Metadata(t *testing.T) {
	assert.Equal(
		t,
		"max",
		maxLimit.name,
	)

	assert.Equal(
		t,
		config.ReducerMax,
		maxLimit.reducer,
	)

	_, ok := maxLimit.characteristics[instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage]

	assert.True(t, ok)
}
