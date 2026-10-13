//go:build cgo

package settings

import (
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLimitPort() falconcore.InstrumentPort {
	return falconcore.InstrumentPort{
		DefaultName:    "P1",
		InstrumentName: "DAC1",
		InstrumentType: instrument.DcVoltageSource,
		Access:         access.Read,
		Scope:          scope.Global,
		PortType:       porttype.PortTypeKnob,
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage,
		Unit:           falconcore.Volt,
	}
}

func testLimitPorts() *config.ConnectedPorts {
	return &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			{
				PortEntry: config.PortEntry{
					InstrumentName: "DAC1",
					InstrumentType: instrument.DcVoltageSource,
					Role:           porttype.PortTypeKnob,
					Access:         access.Read,
					Scope:          scope.Global,
					Characteristic: instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage,
					Unit:           falconcore.Volt,
				},
				PortName: "P1",
			},
		},
	}
}

func testLimitMetadata() map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata {
	return map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata{
		"DAC1": {
			Characteristics: []config.InstrumentCharacteristic{
				{
					Characteristic: instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage,
					ReadCommand: config.APIParseCharacteristic{
						Command:       "READ",
						ParameterName: "voltage",
						Reducer:       config.ReducerMax,
					},
				},
			},
			API: &config.InstrumentAPI{
				IO: []config.IoType{
					{
						Name: "voltage",
						Unit: "V",
						Max:  "10",
					},
				},
			},
		},
	}
}

func testLimit() limitType {
	return limitType{
		name:    "max",
		reducer: config.ReducerMax,
		characteristics: map[instrumentcharacteristic.InstrumentCharacteristic]struct{}{
			instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage: {},
		},
		getValue: func(io *config.IoType) string {
			return io.Max
		},
	}
}

func TestResolveLimitCharacteristic(t *testing.T) {
	out, err := resolveLimitCharacteristic(
		testLimitPort(),
		testLimitMetadata(),
		testLimitPorts(),
		testLimit(),
	)

	require.NoError(t, err)

	assert.Equal(t, "10", out.Value)
	assert.Equal(t, falconcore.Volt, out.Unit)
}

func TestResolveLimitCharacteristic_PortMissing(t *testing.T) {
	_, err := resolveLimitCharacteristic(
		testLimitPort(),
		testLimitMetadata(),
		&config.ConnectedPorts{},
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not present")
}

func TestResolveLimitCharacteristic_NoReadAccess(t *testing.T) {
	port := testLimitPort()
	port.Access = access.Write

	ports := testLimitPorts()
	ports.AllConnections[0].Access = access.Write

	_, err := resolveLimitCharacteristic(
		port,
		testLimitMetadata(),
		ports,
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must have read access")
}

func TestResolveLimitCharacteristic_UnsupportedCharacteristic(t *testing.T) {
	port := testLimitPort()
	port.Characteristic = instrumentcharacteristic.InstrumentCharacteristicSourceVoltage

	ports := testLimitPorts()
	ports.AllConnections[0].Characteristic = port.Characteristic

	_, err := resolveLimitCharacteristic(
		port,
		testLimitMetadata(),
		ports,
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestResolveLimitCharacteristic_MetadataMissing(t *testing.T) {
	_, err := resolveLimitCharacteristic(
		testLimitPort(),
		map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata{},
		testLimitPorts(),
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "instrument metadata not found")
}

func TestResolveLimitCharacteristic_CharacteristicNotFound(t *testing.T) {
	meta := testLimitMetadata()

	meta["DAC1"] = settingrouter.InstrumentMetadata{
		API: meta["DAC1"].API,
	}

	_, err := resolveLimitCharacteristic(
		testLimitPort(),
		meta,
		testLimitPorts(),
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "characteristic")
}

func TestResolveLimitCharacteristic_WrongReducer(t *testing.T) {
	meta := testLimitMetadata()

	m := meta["DAC1"]

	m.Characteristics[0].ReadCommand.Reducer = config.ReducerMin

	meta["DAC1"] = m

	_, err := resolveLimitCharacteristic(
		testLimitPort(),
		meta,
		testLimitPorts(),
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "configured as max getter")
}

func TestResolveLimitCharacteristic_EmptyCommand(t *testing.T) {
	meta := testLimitMetadata()

	m := meta["DAC1"]

	m.Characteristics[0].ReadCommand.Command = ""

	meta["DAC1"] = m

	_, err := resolveLimitCharacteristic(
		testLimitPort(),
		meta,
		testLimitPorts(),
		testLimit(),
	)

	require.Error(t, err)
}

func TestResolveLimitCharacteristic_NoAPI(t *testing.T) {
	meta := testLimitMetadata()

	m := meta["DAC1"]
	m.API = nil

	meta["DAC1"] = m

	_, err := resolveLimitCharacteristic(
		testLimitPort(),
		meta,
		testLimitPorts(),
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no API definition")
}

func TestResolveLimitCharacteristic_ParameterNotFound(t *testing.T) {
	meta := testLimitMetadata()

	m := meta["DAC1"]
	m.API.IO = nil

	meta["DAC1"] = m

	_, err := resolveLimitCharacteristic(
		testLimitPort(),
		meta,
		testLimitPorts(),
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parameter")
}

func TestResolveLimitCharacteristic_MissingValue(t *testing.T) {
	meta := testLimitMetadata()

	m := meta["DAC1"]
	m.API.IO[0].Max = ""

	meta["DAC1"] = m

	_, err := resolveLimitCharacteristic(
		testLimitPort(),
		meta,
		testLimitPorts(),
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no max value")
}

func TestResolveLimitCharacteristic_BadUnit(t *testing.T) {
	meta := testLimitMetadata()

	m := meta["DAC1"]
	m.API.IO[0].Unit = "BAD"

	meta["DAC1"] = m

	_, err := resolveLimitCharacteristic(
		testLimitPort(),
		meta,
		testLimitPorts(),
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported unit")
}

func TestGetLimitQuantity(t *testing.T) {
	q, err := getLimitQuantity(
		testLimitPort(),
		testLimitMetadata(),
		testLimitPorts(),
		testLimit(),
	)

	require.NoError(t, err)

	assert.Equal(t, 10.0, q.Value)
	assert.Equal(t, falconcore.Volt, q.Unit)
}

func TestGetLimitQuantity_ParseError(t *testing.T) {
	meta := testLimitMetadata()

	m := meta["DAC1"]
	m.API.IO[0].Max = "abc"

	meta["DAC1"] = m

	_, err := getLimitQuantity(
		testLimitPort(),
		meta,
		testLimitPorts(),
		testLimit(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse max value")
}
