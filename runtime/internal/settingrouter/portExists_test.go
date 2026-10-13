//go:build cgo

package settingrouter

import (
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testInstrumentPort() falconcore.InstrumentPort {
	return falconcore.InstrumentPort{
		DefaultName:    "P1",
		InstrumentName: "Source1",
		InstrumentType: instrument.VoltageSource,
		Access:         access.Readwrite,
		Scope:          scope.Local,
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
		PortType:       porttype.PortTypeKnob,
		Unit:           falconcore.Volt,
	}
}

func testConnectedPort() config.ConnectedPort {
	return config.ConnectedPort{
		PortEntry: config.PortEntry{
			InstrumentName: "Source1",
			InstrumentType: instrument.VoltageSource,
			Access:         access.Readwrite,
			Scope:          scope.Local,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
			Role:           porttype.PortTypeKnob,
			Unit:           falconcore.Volt,
		},
		PortName: "P1",
	}
}

func TestMatchesPort_True(t *testing.T) {
	assert.True(
		t,
		matchesPort(
			testConnectedPort(),
			testInstrumentPort(),
		),
	)
}

func TestMatchesPort_FalseInstrumentName(t *testing.T) {
	cfg := testConnectedPort()
	cfg.InstrumentName = "Other"

	assert.False(
		t,
		matchesPort(
			cfg,
			testInstrumentPort(),
		),
	)
}

func TestMatchesPort_FalsePortName(t *testing.T) {
	cfg := testConnectedPort()
	cfg.PortName = "Other"

	assert.False(
		t,
		matchesPort(
			cfg,
			testInstrumentPort(),
		),
	)
}

func TestMatchesPort_FalseCharacteristic(t *testing.T) {
	cfg := testConnectedPort()
	cfg.Characteristic = instrumentcharacteristic.InstrumentCharacteristicSourceCurrent

	assert.False(
		t,
		matchesPort(
			cfg,
			testInstrumentPort(),
		),
	)
}

func TestMatchesPort_FalseInstrumentType(t *testing.T) {
	cfg := testConnectedPort()
	cfg.InstrumentType = instrument.Amnmeter

	assert.False(
		t,
		matchesPort(
			cfg,
			testInstrumentPort(),
		),
	)
}

func TestMatchesPort_FalseRole(t *testing.T) {
	cfg := testConnectedPort()
	cfg.Role = porttype.PortTypeMeter

	assert.False(
		t,
		matchesPort(
			cfg,
			testInstrumentPort(),
		),
	)
}

func TestMatchesPort_FalseAccess(t *testing.T) {
	cfg := testConnectedPort()
	cfg.Access = access.Read

	assert.False(
		t,
		matchesPort(
			cfg,
			testInstrumentPort(),
		),
	)
}

func TestMatchesPort_FalseScope(t *testing.T) {
	cfg := testConnectedPort()
	cfg.Scope = scope.Global

	assert.False(
		t,
		matchesPort(
			cfg,
			testInstrumentPort(),
		),
	)
}

func TestMatchesPort_FalseUnit(t *testing.T) {
	cfg := testConnectedPort()
	cfg.Unit = falconcore.Ampere

	assert.False(
		t,
		matchesPort(
			cfg,
			testInstrumentPort(),
		),
	)
}

func TestPortExists_NotFound(t *testing.T) {
	assert.False(
		t,
		PortExists(
			testInstrumentPort(),
			&config.ConnectedPorts{},
		),
	)
}

func TestPortExists_UsesResolvePort(t *testing.T) {
	port := testInstrumentPort()

	ports := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			testConnectedPort(),
		},
	}

	assert.True(
		t,
		PortExists(
			port,
			ports,
		),
	)
}

func TestResolvePort_NilPorts(t *testing.T) {
	port := testInstrumentPort()

	actual, ok := ResolvePort(
		port,
		nil,
	)

	assert.False(t, ok)
	assert.Nil(t, actual)
}

func TestResolvePort_NoMatches(t *testing.T) {
	port := testInstrumentPort()

	actual, ok := ResolvePort(
		port,
		&config.ConnectedPorts{},
	)

	assert.False(t, ok)
	assert.Nil(t, actual)
}

func TestResolvePort_Match_NoPseudoName(t *testing.T) {
	port := testInstrumentPort()

	expected := testConnectedPort()

	actual, ok := ResolvePort(
		port,
		&config.ConnectedPorts{
			AllConnections: []config.ConnectedPort{
				expected,
			},
		},
	)

	require.True(t, ok)
	require.NotNil(t, actual)

	assert.Equal(
		t,
		expected.InstrumentName,
		actual.InstrumentName,
	)
}

func TestResolvePort_Match_PseudoName(t *testing.T) {
	conn := falconcore.Connection{
		Name: "P1",
	}

	port := testInstrumentPort()
	port.PseudoName = conn

	expected := testConnectedPort()
	expected.Handle = conn

	actual, ok := ResolvePort(
		port,
		&config.ConnectedPorts{
			AllConnections: []config.ConnectedPort{
				expected,
			},
		},
	)

	require.True(t, ok)
	require.NotNil(t, actual)

	assert.Equal(
		t,
		conn,
		actual.Handle,
	)
}

func TestResolvePort_PseudoNameMismatch(t *testing.T) {
	conn := falconcore.Connection{
		Name: "P1",
	}

	port := testInstrumentPort()
	port.PseudoName = conn

	cfg := testConnectedPort()
	cfg.Handle = falconcore.Connection{
		Name: "Different",
	}

	actual, ok := ResolvePort(
		port,
		&config.ConnectedPorts{
			AllConnections: []config.ConnectedPort{
				cfg,
			},
		},
	)

	assert.False(t, ok)
	assert.Nil(t, actual)
}
