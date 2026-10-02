package ports

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortEntryRoleHelpers(t *testing.T) {
	t.Run("knob", func(t *testing.T) {
		p := PortEntry{
			Role: porttype.PortTypeKnob,
		}

		assert.True(t, p.IsKnob())
		assert.False(t, p.IsMeter())
		assert.False(t, p.IsSetting())
	})

	t.Run("meter", func(t *testing.T) {
		p := PortEntry{
			Role: porttype.PortTypeMeter,
		}

		assert.False(t, p.IsKnob())
		assert.True(t, p.IsMeter())
		assert.False(t, p.IsSetting())
	})

	t.Run("setting", func(t *testing.T) {
		p := PortEntry{
			Role: porttype.PortTypeSetting,
		}

		assert.False(t, p.IsKnob())
		assert.False(t, p.IsMeter())
		assert.True(t, p.IsSetting())
	})
}

func TestConnectWireMap(t *testing.T) {
	lib := PortLibrary{
		"source.voltage": {
			InstrumentName: "Source1",
			ChannelGroup:   "analog",
			Channel:        4,
			InstrumentType: instrument.DcVoltageSource,
			Role:           porttype.PortTypeKnob,
			Access:         access.Write,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
			Unit:           "V",
			Description:    "Voltage output",
		},
		"source.measure": {
			InstrumentName: "Source1",
			ChannelGroup:   "analog",
			Channel:        4,
			InstrumentType: instrument.DcVoltageSource,
			Role:           porttype.PortTypeMeter,
			Access:         access.Read,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
			Unit:           "V",
			Description:    "Voltage measurement",
		},
	}

	wireMap := &config.WireMap{
		Contents: []config.WiremapEntry{
			{
				PhysicalDeviceName: "P1",
				Instrument: config.WiremapInstrument{
					Name:         "Source1",
					ChannelGroup: "analog",
					Channel:      4,
				},
			},
		},
	}

	connected, err := ConnectWireMap(wireMap, lib)

	require.NoError(t, err)
	require.Len(t, connected, 2)

	for _, cp := range connected {
		assert.Equal(t, "P1", cp.DeviceName)
		assert.Equal(t, "Source1", cp.InstrumentName)
		assert.Equal(t, "analog", cp.ChannelGroup)
		assert.Equal(t, 4, cp.Channel)
	}
}

func TestConnectWireMapNoMatches(t *testing.T) {
	lib := PortLibrary{
		"other": {
			InstrumentName: "OtherInstrument",
			ChannelGroup:   "analog",
			Channel:        1,
		},
	}

	wireMap := &config.WireMap{
		Contents: []config.WiremapEntry{
			{
				PhysicalDeviceName: "P1",
				Instrument: config.WiremapInstrument{
					Name:         "Source1",
					ChannelGroup: "analog",
					Channel:      4,
				},
			},
		},
	}

	connected, err := ConnectWireMap(wireMap, lib)

	require.NoError(t, err)
	require.Empty(t, connected)
}

func TestNewConnectedPorts(t *testing.T) {
	ports := []ConnectedPort{
		{
			PortEntry: PortEntry{
				Role: porttype.PortTypeKnob,
			},
		},
		{
			PortEntry: PortEntry{
				Role: porttype.PortTypeMeter,
			},
		},
		{
			PortEntry: PortEntry{
				Role: porttype.PortTypeSetting,
			},
		},
	}

	out := newConnectedPorts(ports)

	require.Len(t, out.AllConnections, 3)
	require.Len(t, out.Knobs, 1)
	require.Len(t, out.Meters, 1)
	require.Len(t, out.Settings, 1)
}

func TestParseInstrumentAPI(t *testing.T) {
	content := `
api_version: "1.0.0"
instrument:
  vendor: Mock
  model: 1
  identifier: Source1
  instrument_type: dc_voltage_source
channel_groups:
  - name: analog
    io_types:
      - name: voltage
        role: output
        unit: V
`

	tmpFile := filepath.Join(t.TempDir(), "api.yml")

	require.NoError(
		t,
		os.WriteFile(tmpFile, []byte(content), 0o600),
	)

	api, err := ParseInstrumentAPI(tmpFile)

	require.NoError(t, err)
	require.NotNil(t, api)

	assert.Equal(t, "Mock", api.Instrument.Vendor)
	assert.Equal(t, "Source1", api.Instrument.Identifier)
	assert.Equal(t, "dc_voltage_source", api.Instrument.InstrumentType)
}

func TestParseInstrumentAPIs(t *testing.T) {
	dir := t.TempDir()

	content := `
instrument:
  vendor: Mock
  identifier: Source1
  instrument_type: dc_voltage_source
`

	p1 := filepath.Join(dir, "a.yml")
	p2 := filepath.Join(dir, "b.yml")

	require.NoError(t, os.WriteFile(p1, []byte(content), 0o600))
	require.NoError(t, os.WriteFile(p2, []byte(content), 0o600))

	apis, err := ParseInstrumentAPIs([]string{p1, p2})

	require.NoError(t, err)
	require.Len(t, apis, 2)
}

func TestParseInstrumentAPIMissingFile(t *testing.T) {
	_, err := ParseInstrumentAPI("/does/not/exist.yml")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read")
}

func TestParseInstrumentAPIInvalidYAML(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "bad.yml")

	require.NoError(
		t,
		os.WriteFile(tmpFile, []byte(":\n:\n:\n"), 0o600),
	)

	_, err := ParseInstrumentAPI(tmpFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}

func TestParseInstrumentAPIRequiresInstrumentType(t *testing.T) {
	content := `
instrument:
  vendor: Mock
  identifier: Source1
`

	tmpFile := filepath.Join(t.TempDir(), "api.yml")

	require.NoError(
		t,
		os.WriteFile(tmpFile, []byte(content), 0o600),
	)

	_, err := ParseInstrumentAPI(tmpFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing instrument.instrument_type")
}

func TestParseInstrumentAPIRejectsUnsupportedInstrumentType(t *testing.T) {
	content := `
instrument:
  vendor: Mock
  identifier: Source1
  instrument_type: nonsense_type
`

	tmpFile := filepath.Join(t.TempDir(), "api.yml")

	require.NoError(
		t,
		os.WriteFile(tmpFile, []byte(content), 0o600),
	)

	_, err := ParseInstrumentAPI(tmpFile)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported instrument.instrument_type")
}

func newTestUnit(t *testing.T) *symbolunit.Handle {
	t.Helper()

	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, unit.Close())
	})

	return unit
}

func TestResolveConnectedPort_Success(t *testing.T) {
	pseudo, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer pseudo.Close()

	units := newTestUnit(t)

	port, err := instrumentport.NewPort(
		"Device1",
		"Source1",
		scope.Local,
		access.Readwrite,
		instrumentcharacteristic.InstrumentCharacteristicNone,
		porttype.PortTypeKnob,
		pseudo,
		instrument.DcVoltageSource,
		units,
		"test port",
	)
	require.NoError(t, err)
	defer port.Close()

	ports := ConnectedPorts{
		AllConnections: []ConnectedPort{
			{
				PortEntry: PortEntry{
					InstrumentName: "Source1",
					InstrumentType: instrument.DcVoltageSource,
					Role:           porttype.PortTypeKnob,
					Access:         access.Readwrite,
					Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
				},

				DeviceName: "Device1",
			},
		},
	}

	resolved, err := ports.ResolveConnectedPort(port)

	require.NoError(t, err)
	assert.Equal(t, PortName("matching"), resolved.PortName)
}

func TestResolveConnectedPort_NoMatch(t *testing.T) {
	pseudo, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer pseudo.Close()

	units := newTestUnit(t)

	port, err := instrumentport.NewPort(
		"Device1",
		"Source1",
		scope.Local,
		access.Readwrite,
		instrumentcharacteristic.InstrumentCharacteristicNone,
		porttype.PortTypeKnob,
		pseudo,
		instrument.DcVoltageSource,
		units,
		"test port",
	)
	require.NoError(t, err)
	defer port.Close()

	ports := ConnectedPorts{}

	_, err = ports.ResolveConnectedPort(port)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "No connected port")
}

func TestResolveConnectedPort_Ambiguous(t *testing.T) {
	pseudo, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer pseudo.Close()

	units := newTestUnit(t)

	port, err := instrumentport.NewPort(
		"Device1",
		"Source1",
		scope.Local,
		access.Readwrite,
		instrumentcharacteristic.InstrumentCharacteristicNone,
		porttype.PortTypeKnob,
		pseudo,
		instrument.DcVoltageSource,
		units,
		"test port",
	)
	require.NoError(t, err)
	defer port.Close()

	cp := ConnectedPort{
		PortEntry: PortEntry{
			InstrumentName: "Source1",
			InstrumentType: instrument.DcVoltageSource,
			Role:           porttype.PortTypeKnob,
			Access:         access.Readwrite,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
		},

		DeviceName: "Device1",
	}

	ports := ConnectedPorts{
		AllConnections: []ConnectedPort{
			cp,
			cp,
		},
	}

	_, err = ports.ResolveConnectedPort(port)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Ambiguous connected port")
}

func TestResolveConnectedPort_IgnoresNonMatchingConnections(t *testing.T) {
	pseudo, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer pseudo.Close()

	units := newTestUnit(t)

	port, err := instrumentport.NewPort(
		"Device1",
		"Source1",
		scope.Local,
		access.Readwrite,
		instrumentcharacteristic.InstrumentCharacteristicNone,
		porttype.PortTypeKnob,
		pseudo,
		instrument.DcVoltageSource,
		units,
		"test port",
	)
	require.NoError(t, err)
	defer port.Close()

	ports := ConnectedPorts{
		AllConnections: []ConnectedPort{
			{
				PortEntry: PortEntry{
					InstrumentName: "Source1",
					InstrumentType: instrument.DcVoltageSource,
					Role:           porttype.PortTypeKnob,
					Access:         access.Readwrite,
					Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
				},
				DeviceName: "WrongDevice",
			},
			{
				PortEntry: PortEntry{
					InstrumentName: "Source1",
					InstrumentType: instrument.DcVoltageSource,
					Role:           porttype.PortTypeKnob,
					Access:         access.Readwrite,
					Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
				},
				PortName:   "correct",
				DeviceName: "Device1",
			},
		},
	}

	resolved, err := ports.ResolveConnectedPort(port)

	require.NoError(t, err)
	assert.Equal(t, PortName("correct"), resolved.PortName)
}
