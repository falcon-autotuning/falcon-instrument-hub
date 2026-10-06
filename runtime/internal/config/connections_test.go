package config

import (
	"fmt"
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

	wireMap := WireMap{
		{
			PhysicalDeviceName: "P1",
			Instrument: WiremapInstrument{
				Name:         "Source1",
				ChannelGroup: "analog",
				Channel:      4,
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

	wireMap := WireMap{
		{
			PhysicalDeviceName: "P1",
			Instrument: WiremapInstrument{
				Name:         "Source1",
				ChannelGroup: "analog",
				Channel:      4,
			},
		},
	}

	connected, err := ConnectWireMap(wireMap, lib)

	require.NoError(t, err)
	require.Empty(t, connected)
}

func TestNewConnectedPortsFromConnections(t *testing.T) {
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

	out := newConnectedPortsFromConnections(ports)

	require.Len(t, out.AllConnections, 3)
	require.Len(t, out.Knobs, 1)
	require.Len(t, out.Meters, 1)
	require.Len(t, out.Settings, 1)
}

func writeFile(
	t *testing.T,
	dir string,
	name string,
	content string,
) string {
	t.Helper()

	path := filepath.Join(dir, name)

	require.NoError(
		t,
		os.WriteFile(path, []byte(content), 0o600),
	)

	return path
}

func TestBuildPortLibrary(t *testing.T) {
	dir := t.TempDir()

	apiPath := writeFile(
		t,
		dir,
		"api.yaml",
		`
api_version: 1.0.0

instrument:
  vendor: Mock
  model: 1
  identifier: Mock1

protocol:
  type: VISA

channel_groups:
  - name: analog
    channel_parameter:
      min: 1
      max: 2

    io_types:
      - suffix: voltage
        role: output
        unit: V

      - suffix: measured_voltage
        role: input
        unit: V

io:
  - name: analog1_voltage
    role: output

  - name: analog1_measured_voltage
    role: input

  - name: analog2_voltage
    role: output

  - name: analog2_measured_voltage
    role: input
`,
	)

	cfgPath := writeFile(
		t,
		dir,
		"instrument.yaml",
		fmt.Sprintf(`
name: Source1
api_ref: %s
`, filepath.Base(apiPath)),
	)

	library, err := BuildPortLibrary(
		[]InstrumentConfig{
			{
				ConfigPath:     cfgPath,
				InstrumentType: instrument.DcVoltageSource,
			},
		},
	)

	require.NoError(t, err)

	assert.Len(t, library, 4)

	assert.Contains(
		t,
		library,
		PortName("Source1.analog.1.voltage"),
	)

	assert.Contains(
		t,
		library,
		PortName("Source1.analog.2.measured_voltage"),
	)

	knob := library["Source1.analog.1.voltage"]

	assert.Equal(
		t,
		instrument.DcVoltageSource,
		knob.InstrumentType,
	)

	assert.Equal(
		t,
		porttype.PortTypeKnob,
		knob.Role,
	)

	assert.Equal(
		t,
		access.Write,
		knob.Access,
	)

	assert.Equal(
		t,
		"V",
		knob.Unit,
	)

	meter := library["Source1.analog.2.measured_voltage"]

	assert.Equal(
		t,
		porttype.PortTypeMeter,
		meter.Role,
	)

	assert.Equal(
		t,
		access.Read,
		meter.Access,
	)
}

func TestBuildPortLibraryRejectsUnsupportedRole(t *testing.T) {
	dir := t.TempDir()

	apiPath := writeFile(
		t,
		dir,
		"api.yaml",
		`
api_version: 1.0.0

instrument:
  vendor: Mock
  model: 1
  identifier: Mock1

protocol:
  type: VISA

channel_groups:
  - name: analog
    channel_parameter:
      min: 1
      max: 1

    io_types:
      - suffix: trigger
        role: banana

io:
  - name: analog1_trigger
    role: banana
`,
	)

	cfgPath := writeFile(
		t,
		dir,
		"instrument.yaml",
		fmt.Sprintf(`
name: Source1
api_ref: %s
`, filepath.Base(apiPath)),
	)

	_, err := BuildPortLibrary(
		[]InstrumentConfig{
			{
				ConfigPath:     cfgPath,
				InstrumentType: instrument.DcVoltageSource,
			},
		},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"unknown IO role",
	)
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
      - suffix: voltage
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

func TestParseInstrumentAPIMissingIdentifier(t *testing.T) {
	content := `
instrument:
  vendor: Mock
`

	tmpFile := filepath.Join(t.TempDir(), "api.yml")

	require.NoError(
		t,
		os.WriteFile(tmpFile, []byte(content), 0o600),
	)

	_, err := ParseInstrumentAPI(tmpFile)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"missing instrument identifier",
	)
}

func TestParseInstrumentAPIMissingVendor(t *testing.T) {
	content := `
instrument:
  identifier: Source1
`

	tmpFile := filepath.Join(t.TempDir(), "api.yml")

	require.NoError(
		t,
		os.WriteFile(tmpFile, []byte(content), 0o600),
	)

	_, err := ParseInstrumentAPI(tmpFile)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"missing instrument vendor",
	)
}

func TestParseInstrumentConfig(t *testing.T) {
	content := `
name: Scope1
api_ref: api.yaml
`

	tmpFile := filepath.Join(t.TempDir(), "config.yml")

	require.NoError(
		t,
		os.WriteFile(tmpFile, []byte(content), 0o600),
	)

	cfg, err := ParseInstrumentConfig(tmpFile)

	require.NoError(t, err)

	assert.Equal(t, "Scope1", cfg.Name)
	assert.Equal(t, "api.yaml", cfg.API_ref)
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
				PortName:   "matching",
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

func TestParseIORole(t *testing.T) {
	roles := []IORole{
		IORoleInput,
		IORoleOutput,
		IORoleInOut,
		IORoleTriggerIn,
		IORoleTriggerOut,
		IORoleClockIn,
		IORoleClockOut,
		IORoleSetting,
	}

	for _, role := range roles {
		got, err := ParseIORole(string(role))
		require.NoError(t, err)
		assert.Equal(t, role, got)
	}
}

func TestParseIORole_Unknown(t *testing.T) {
	_, err := ParseIORole("banana")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown IO role")
}

func TestPortAttributesFromRole(t *testing.T) {
	tests := []struct {
		role   IORole
		ptype  porttype.PortType
		access access.Access
	}{
		{IORoleInput, porttype.PortTypeMeter, access.Read},
		{IORoleOutput, porttype.PortTypeKnob, access.Write},
		{IORoleInOut, porttype.PortTypeKnob, access.Readwrite},
		{IORoleTriggerIn, porttype.PortTypeMeter, access.Read},
		{IORoleTriggerOut, porttype.PortTypeKnob, access.Write},
		{IORoleClockIn, porttype.PortTypeMeter, access.Read},
		{IORoleClockOut, porttype.PortTypeKnob, access.Write},
		{IORoleSetting, porttype.PortTypeSetting, access.Readwrite},
	}

	for _, tt := range tests {
		p, a, err := portAttributesFromRole(tt.role)

		require.NoError(t, err)
		assert.Equal(t, tt.ptype, p)
		assert.Equal(t, tt.access, a)
	}
}

func TestPortAttributesFromRole_Unsupported(t *testing.T) {
	_, _, err := portAttributesFromRole(IORole("garbage"))

	require.Error(t, err)
}

func TestAddPort(t *testing.T) {
	lib := PortLibrary{}

	err := addPort(
		lib,
		"test",
		PortEntry{},
	)

	require.NoError(t, err)

	_, ok := lib["test"]
	assert.True(t, ok)
}

func TestAddPort_Duplicate(t *testing.T) {
	lib := PortLibrary{
		"test": {},
	}

	err := addPort(
		lib,
		"test",
		PortEntry{},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestAddIOPort(t *testing.T) {
	lib := PortLibrary{}

	err := addIOPort(
		lib,
		"Source1",
		instrument.DcVoltageSource,
		"port",
		"analog",
		1,
		IoType{
			Suffix: "voltage",
			Role:   "output",
			Unit:   "V",
		},
		"context",
	)

	require.NoError(t, err)

	entry := lib["port"]

	assert.Equal(t, access.Write, entry.Access)
	assert.Equal(t, porttype.PortTypeKnob, entry.Role)
}

func TestAddIOPort_NoName(t *testing.T) {
	err := addIOPort(
		PortLibrary{},
		"Source1",
		instrument.DcVoltageSource,
		"port",
		"",
		0,
		IoType{},
		"context",
	)

	require.Error(t, err)
}

func TestAddIOPort_InvalidRole(t *testing.T) {
	err := addIOPort(
		PortLibrary{},
		"Source1",
		instrument.DcVoltageSource,
		"port",
		"",
		0,
		IoType{
			Name: "foo",
			Role: "garbage",
		},
		"context",
	)

	require.Error(t, err)
}

func TestBuildExpandedIONames(t *testing.T) {
	api := &InstrumentAPI{
		ChannelGroups: []ChannelGroup{
			{
				Name: "analog",
				ChannelParameter: ChannelParameter{
					Min: 1,
					Max: 2,
				},
				IoTypes: []IoType{
					{
						Suffix: "waveform",
					},
				},
			},
		},
	}

	names := buildExpandedIONames(api)

	_, ok1 := names["analog1_waveform"]
	_, ok2 := names["analog2_waveform"]

	assert.True(t, ok1)
	assert.True(t, ok2)
}

func TestParameterLocation(t *testing.T) {
	api := &InstrumentAPI{
		ChannelGroups: []ChannelGroup{
			{
				Name: "analog",
				ChannelParameter: ChannelParameter{
					Name: "analog",
				},
			},
		},
	}

	tests := []struct {
		name         string
		param        string
		scope        scope.Scope
		channelGroup string
	}{
		{
			name:         "local",
			param:        "analog",
			scope:        scope.Local,
			channelGroup: "analog",
		},
		{
			name:         "global",
			param:        "foo",
			scope:        scope.Global,
			channelGroup: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, group, err := parameterLocation(
				api,
				tt.param,
			)

			require.NoError(t, err)
			assert.Equal(t, tt.scope, s)
			assert.Equal(t, tt.channelGroup, group)
		})
	}
}

func TestCharacteristicAccess(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		a := characteristicAccess(
			InstrumentCharacteristic{
				ReadCommand: APIParseCharacteristic{
					Command: "READ",
				},
			},
		)

		assert.Equal(t, access.Read, a)
	})

	t.Run("write", func(t *testing.T) {
		a := characteristicAccess(
			InstrumentCharacteristic{
				WriteCommand: APIParseCharacteristic{
					Command: "WRITE",
				},
			},
		)

		assert.Equal(t, access.Write, a)
	})

	t.Run("readwrite", func(t *testing.T) {
		a := characteristicAccess(
			InstrumentCharacteristic{
				ReadCommand: APIParseCharacteristic{
					Command: "READ",
				},
				WriteCommand: APIParseCharacteristic{
					Command: "WRITE",
				},
			},
		)

		assert.Equal(t, access.Readwrite, a)
	})

	t.Run("default", func(t *testing.T) {
		a := characteristicAccess(
			InstrumentCharacteristic{},
		)

		assert.Equal(t, access.Readwrite, a)
	})
}

func TestInternalBuildPortLibrary_ExpansionMismatch(t *testing.T) {
	api := &InstrumentAPI{
		ChannelGroups: []ChannelGroup{
			{
				Name: "analog",
				ChannelParameter: ChannelParameter{
					Min: 1,
					Max: 2,
				},
				IoTypes: []IoType{
					{
						Suffix: "waveform",
					},
				},
			},
		},
		IO: []IoType{
			{
				Name: "analog1_waveform",
			},
		},
	}

	expanded := buildExpandedIONames(api)

	require.Len(t, expanded, 2)

	_, ok := expanded["analog1_waveform"]
	assert.True(t, ok)

	_, ok = expanded["analog2_waveform"]
	assert.True(t, ok)
}

func TestCharacteristicLocation(t *testing.T) {
	tests := []struct {
		name           string
		api            *InstrumentAPI
		characteristic InstrumentCharacteristic
		scope          scope.Scope
		channelGroup   string
	}{
		{
			name: "global parameter",
			api:  &InstrumentAPI{},
			characteristic: InstrumentCharacteristic{
				ReadCommand: APIParseCharacteristic{
					ParameterName: "global_rate",
				},
			},
			scope: scope.Global,
		},
		{
			name: "local parameter",
			api: &InstrumentAPI{
				ChannelGroups: []ChannelGroup{
					{
						Name: "analog",
						ChannelParameter: ChannelParameter{
							Name: "analog",
						},
					},
				},
			},
			characteristic: InstrumentCharacteristic{
				ReadCommand: APIParseCharacteristic{
					ParameterName: "analog",
				},
			},
			scope:        scope.Local,
			channelGroup: "analog",
		},
		{
			name: "local command",
			api: &InstrumentAPI{
				Commands: map[CommandName]Command{
					"GET_SAMPLE_RATE": {
						ChannelGroup: "analog",
					},
				},
			},
			characteristic: InstrumentCharacteristic{
				ReadCommand: APIParseCharacteristic{
					Command: "GET_SAMPLE_RATE",
				},
			},
			scope:        scope.Local,
			channelGroup: "analog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, group, err := characteristicLocation(
				tt.api,
				tt.characteristic,
			)

			require.NoError(t, err)
			assert.Equal(t, tt.scope, s)
			assert.Equal(t, tt.channelGroup, group)
		})
	}
}

func TestFindChannelGroup(t *testing.T) {
	api := &InstrumentAPI{
		ChannelGroups: []ChannelGroup{
			{
				Name: "analog",
			},
		},
	}

	t.Run("found", func(t *testing.T) {
		group, err := findChannelGroup(
			api,
			"analog",
		)

		require.NoError(t, err)
		assert.Equal(t, "analog", group.Name)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := findChannelGroup(
			api,
			"missing",
		)

		require.Error(t, err)
	})
}

func TestChannelParameters(t *testing.T) {
	params := channelParameters(
		&InstrumentAPI{
			ChannelGroups: []ChannelGroup{
				{
					ChannelParameter: ChannelParameter{
						Name: "analog",
					},
				},
				{
					ChannelParameter: ChannelParameter{
						Name: "digital",
					},
				},
			},
		},
	)

	_, ok := params["analog"]
	assert.True(t, ok)

	_, ok = params["digital"]
	assert.True(t, ok)
}

func TestBuildPortLibrary_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()

	apiPath := filepath.Join(tmpDir, "api.yaml")
	cfgPath := filepath.Join(tmpDir, "instrument.yaml")

	require.NoError(
		t,
		os.WriteFile(
			apiPath,
			[]byte(`
api_version: 1.0.0

instrument:
  vendor: Keysight
  model: 123
  identifier: GPI1
  description: Test Scope

protocol:
  type: VISA

channel_groups:
  - name: analog
    description: Analog channels
    channel_parameter:
      name: analog
      type: int
      min: 1
      max: 4
    io_types:
      - suffix: waveform
        type: float
        role: output
        unit: V
      - suffix: sample_rate
        type: float
        role: setting
        unit: Hz

io:
  - name: timebase
    type: float
    role: setting
    unit: s

  - name: ext_trigger
    type: bool
    role: trigger-in

  - name: analog1_waveform
    type: float
    role: output

  - name: analog1_sample_rate
    type: float
    role: setting

  - name: analog2_waveform
    type: float
    role: output

  - name: analog2_sample_rate
    type: float
    role: setting

  - name: analog3_waveform
    type: float
    role: output

  - name: analog3_sample_rate
    type: float
    role: setting

  - name: analog4_waveform
    type: float
    role: output

  - name: analog4_sample_rate
    type: float
    role: setting

commands:
  GET_SAMPLE_RATE:
    channel_group: analog
    extra: whee

  SET_SAMPLE_RATE:
    channel_group: analog
    no: crash
`),
			0o600,
		),
	)

	require.NoError(
		t,
		os.WriteFile(
			cfgPath,
			[]byte(fmt.Sprintf(`
name: Scope1
api_ref: %s
`, filepath.Base(apiPath))),
			0o600,
		),
	)

	lib, err := BuildPortLibrary([]InstrumentConfig{
		{
			ConfigPath:     cfgPath,
			InstrumentType: instrument.Voltmeter,
			Characteristics: []InstrumentCharacteristic{
				{
					Identifier: "sample_rate",
					Characteristic: instrumentcharacteristic.
						InstrumentCharacteristicSampleRate,
					ReadCommand: APIParseCharacteristic{
						Command: "GET_SAMPLE_RATE",
					},
					WriteCommand: APIParseCharacteristic{
						Command: "SET_SAMPLE_RATE",
					},
				},
			},
		},
	})

	require.NoError(t, err)

	wf1, ok := lib["Scope1.analog.1.waveform"]
	require.True(t, ok)

	assert.Equal(t, porttype.PortTypeKnob, wf1.Role)
	assert.Equal(t, access.Write, wf1.Access)

	wf4, ok := lib["Scope1.analog.4.waveform"]
	require.True(t, ok)

	assert.Equal(t, porttype.PortTypeKnob, wf4.Role)

	timebase, ok := lib["Scope1.timebase"]
	require.True(t, ok)

	assert.Equal(t, porttype.PortTypeSetting, timebase.Role)
	assert.Equal(t, access.Readwrite, timebase.Access)

	trigger, ok := lib["Scope1.ext_trigger"]
	require.True(t, ok)

	assert.Equal(t, porttype.PortTypeMeter, trigger.Role)
	assert.Equal(t, access.Read, trigger.Access)

	sampleRate1, ok := lib["Scope1.characteristic.analog.1.sample_rate"]
	require.True(t, ok)

	assert.Equal(
		t,
		instrumentcharacteristic.InstrumentCharacteristicSampleRate,
		sampleRate1.Characteristic,
	)

	assert.Equal(t, scope.Local, sampleRate1.Scope)
	assert.Equal(t, access.Readwrite, sampleRate1.Access)

	sampleRate4, ok := lib["Scope1.characteristic.analog.4.sample_rate"]
	require.True(t, ok)

	assert.Equal(
		t,
		instrumentcharacteristic.InstrumentCharacteristicSampleRate,
		sampleRate4.Characteristic,
	)

	assert.Equal(t, scope.Local, sampleRate4.Scope)

	// -----------------------------------------------------------------
	// Ensure duplicates from expanded IOs were ignored.
	//
	// 4 waveform ports
	// 4 characteristic ports
	// 2 global ports
	// 4 sample_rate ports
	//
	// = 14 total ports
	// -----------------------------------------------------------------

	assert.Len(t, lib, 14)
}

func TestBuildPortLibrary_ExpansionMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	apiPath := filepath.Join(tmpDir, "api.yaml")
	cfgPath := filepath.Join(tmpDir, "instrument.yaml")

	require.NoError(
		t,
		os.WriteFile(
			apiPath,
			[]byte(`
api_version: 1.0.0

instrument:
  vendor: Keysight
  model: 123
  identifier: GPI1

protocol:
  type: VISA

channel_groups:
  - name: analog
    channel_parameter:
      name: analog
      type: int
      min: 1
      max: 2

    io_types:
      - suffix: waveform
        type: float
        role: output

io:
  - name: analog1_waveform
    type: float
    role: output
`),
			0o600,
		),
	)

	require.NoError(
		t,
		os.WriteFile(
			cfgPath,
			[]byte(fmt.Sprintf(`
name: Scope1
api_ref: %s
`, filepath.Base(apiPath))),
			0o600,
		),
	)

	_, err := BuildPortLibrary(
		[]InstrumentConfig{
			{
				ConfigPath:     cfgPath,
				InstrumentType: instrument.Voltmeter,
			},
		},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"API expansion mismatch",
	)

	assert.Contains(
		t,
		err.Error(),
		"analog2_waveform",
	)
}
