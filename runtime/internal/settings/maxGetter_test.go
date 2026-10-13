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

func testMaxPort() falconcore.InstrumentPort {
	return falconcore.InstrumentPort{
		DefaultName:    "P1",
		InstrumentName: "DAC1",
		InstrumentType: instrument.Instrument(0),
		Unit:           falconcore.Volt,
		Access:         access.Read,
		Scope:          scope.Global,
		PortType:       porttype.PortTypeKnob,
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage,
	}
}

func testConnectedPorts() *config.ConnectedPorts {
	return &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			{
				PortEntry: config.PortEntry{
					InstrumentName: "DAC1",
					InstrumentType: instrument.Instrument(0),
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

func testMetadata() map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata {
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
						Max:  "10",
						Unit: "V",
					},
				},
			},
		},
	}
}

func TestNewMaxCharacteristicGetter(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	require.NotNil(t, g)
	assert.Equal(t, "MaxCharacteristicGetter", g.Name())
}

func TestSupportsMaxGetter(t *testing.T) {
	assert.True(t,
		supportsMaxGetter(
			config.InstrumentCharacteristic{
				ReadCommand: config.APIParseCharacteristic{
					Command: "READ",
					Reducer: config.ReducerMax,
				},
			},
		),
	)

	assert.False(t,
		supportsMaxGetter(
			config.InstrumentCharacteristic{},
		),
	)
}

func TestFindCharacteristic(t *testing.T) {
	c, err := findCharacteristic(
		testMaxPort(),
		testMetadata(),
	)

	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestFindCharacteristic_NoInstrument(t *testing.T) {
	_, err := findCharacteristic(
		testMaxPort(),
		map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata{},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "instrument metadata not found")
}

func TestFindCharacteristic_NoCharacteristic(t *testing.T) {
	meta := testMetadata()

	meta["DAC1"] = settingrouter.InstrumentMetadata{
		Characteristics: nil,
	}

	_, err := findCharacteristic(
		testMaxPort(),
		meta,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "characteristic")
}

func TestResolveMaxCharacteristic(t *testing.T) {
	out, err := resolveMaxCharacteristic(
		testMaxPort(),
		testMetadata(),
		testConnectedPorts(),
	)

	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, "10", out.IoType.Max)
}

func TestResolveMaxCharacteristic_PortMissing(t *testing.T) {
	_, err := resolveMaxCharacteristic(
		testMaxPort(),
		testMetadata(),
		&config.ConnectedPorts{},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not present in connected ports")
}

func TestResolveMaxCharacteristic_NoReadAccess(t *testing.T) {
	port := testMaxPort()
	port.Access = access.Write

	ports := testConnectedPorts()
	ports.AllConnections[0].Access = access.Write

	_, err := resolveMaxCharacteristic(
		port,
		testMetadata(),
		ports,
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"must have read access",
	)
}

func TestResolveMaxCharacteristic_UnsupportedCharacteristic(t *testing.T) {
	port := testMaxPort()
	port.Characteristic = instrumentcharacteristic.
		InstrumentCharacteristicSourceVoltage

	ports := testConnectedPorts()
	ports.AllConnections[0].Characteristic = port.Characteristic

	_, err := resolveMaxCharacteristic(
		port,
		testMetadata(),
		ports,
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"unsupported",
	)
}

func TestResolveMaxCharacteristic_MetadataMissing(t *testing.T) {
	_, err := resolveMaxCharacteristic(
		testMaxPort(),
		map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata{},
		testConnectedPorts(),
	)

	require.Error(t, err)
}

func TestResolveMaxCharacteristic_NotConfigured(t *testing.T) {
	meta := testMetadata()

	m := meta["DAC1"]

	m.Characteristics[0].ReadCommand.Reducer = config.ReducerMin

	meta["DAC1"] = m

	_, err := resolveMaxCharacteristic(
		testMaxPort(),
		meta,
		testConnectedPorts(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestResolveMaxCharacteristic_NoAPI(t *testing.T) {
	meta := testMetadata()

	m := meta["DAC1"]
	m.API = nil
	meta["DAC1"] = m

	_, err := resolveMaxCharacteristic(
		testMaxPort(),
		meta,
		testConnectedPorts(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no API definition")
}

func TestResolveMaxCharacteristic_ParameterNotFound(t *testing.T) {
	meta := testMetadata()

	m := meta["DAC1"]
	m.API.IO = nil
	meta["DAC1"] = m

	_, err := resolveMaxCharacteristic(
		testMaxPort(),
		meta,
		testConnectedPorts(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parameter")
}

func TestResolveMaxCharacteristic_NoMax(t *testing.T) {
	meta := testMetadata()

	m := meta["DAC1"]
	m.API.IO[0].Max = ""
	meta["DAC1"] = m

	_, err := resolveMaxCharacteristic(
		testMaxPort(),
		meta,
		testConnectedPorts(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no max value")
}

func TestResolveMaxCharacteristic_BadUnit(t *testing.T) {
	meta := testMetadata()

	m := meta["DAC1"]
	m.API.IO[0].Unit = "BAD_UNIT"
	meta["DAC1"] = m

	_, err := resolveMaxCharacteristic(
		testMaxPort(),
		meta,
		testConnectedPorts(),
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported unit")
}

func TestMaxCharacteristicGetter_CanGet(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	assert.True(
		t,
		g.CanGet(
			testMaxPort(),
			testMetadata(),
			testConnectedPorts(),
		),
	)
}

func TestMaxCharacteristicGetter_CannotGet(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	assert.False(
		t,
		g.CanGet(
			testMaxPort(),
			testMetadata(),
			&config.ConnectedPorts{},
		),
	)
}

func TestMaxCharacteristicGetter_Get(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	q, err := g.Get(
		testMaxPort(),
		nil,
		nil,
		testConnectedPorts(),
		testMetadata(),
	)

	require.NoError(t, err)

	assert.Equal(t, 10.0, q.Value)
	assert.Equal(t, falconcore.Volt, q.Unit)
}

func TestMaxCharacteristicGetter_Get_ParseError(t *testing.T) {
	g := NewMaxCharacteristicGetter()

	meta := testMetadata()

	m := meta["DAC1"]
	m.API.IO[0].Max = "not-a-number"
	meta["DAC1"] = m

	_, err := g.Get(
		testMaxPort(),
		nil,
		nil,
		testConnectedPorts(),
		meta,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse max value")
}
