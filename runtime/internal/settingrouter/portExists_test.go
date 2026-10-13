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

func TestPortExists_NilPorts(t *testing.T) {
	assert.False(
		t,
		PortExists(
			testInstrumentPort(),
			nil,
		),
	)
}

func TestPortExists_NoMatches(t *testing.T) {
	port := testInstrumentPort()

	ports := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{},
	}

	assert.False(
		t,
		PortExists(
			port,
			ports,
		),
	)
}

func TestPortExists_MatchingPort_NoPseudoName(t *testing.T) {
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

func TestPortExists_PseudoNameMatch(t *testing.T) {
	conn := falconcore.Connection{
		Name: "P1",
	}

	port := testInstrumentPort()
	port.PseudoName = conn

	cfg := testConnectedPort()
	cfg.Handle = conn

	ports := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			cfg,
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

func TestPortExists_PseudoNameMismatch(t *testing.T) {
	conn := falconcore.Connection{
		Name: "P1",
	}

	port := testInstrumentPort()
	port.PseudoName = conn

	cfg := testConnectedPort()
	cfg.Handle = falconcore.Connection{
		Name: "Different",
	}

	ports := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			cfg,
		},
	}

	assert.False(
		t,
		PortExists(
			port,
			ports,
		),
	)
}
