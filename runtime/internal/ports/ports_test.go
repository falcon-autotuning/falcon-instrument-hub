package ports

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPortLibrary(t *testing.T) {
	apis := []InstrumentAPI{
		{
			Instrument: APIInstrument{
				Vendor:         "Mock",
				Identifier:     "Source1",
				InstrumentType: "dc_voltage_source",
			},
			Protocol: APIProtocol{
				Type: "MockVoltageSource",
			},
			ChannelGroups: []ChannelGroup{
				{
					Name: "analog",
					IoTypes: []IoType{
						{Name: "voltage", Role: "output", Unit: "V"},
						{Name: "measured_voltage", Role: "input", Unit: "V"},
					},
				},
			},
		},
	}

	lib := BuildPortLibrary(apis)

	require.Len(t, lib, 2)

	voltage := lib["Mock.Source1.analog.voltage"]
	assert.Equal(t, "output", voltage.Role)
	assert.Equal(t, "V", voltage.Unit)
	assert.Equal(t, "dc_voltage_source", voltage.InstrumentType)
	assert.True(t, voltage.IsKnob())
	assert.False(t, voltage.IsMeter())

	measured := lib["Mock.Source1.analog.measured_voltage"]
	assert.Equal(t, "input", measured.Role)
	assert.Equal(t, "dc_voltage_source", measured.InstrumentType)
	assert.False(t, measured.IsKnob())
	assert.True(t, measured.IsMeter())
}

func TestConnectWireMap(t *testing.T) {
	apis := []InstrumentAPI{
		{
			Instrument: APIInstrument{
				Vendor:         "Mock",
				Identifier:     "Source1",
				InstrumentType: "dc_voltage_source",
			},
			Protocol: APIProtocol{
				Type: "MockVoltageSource",
			},
			ChannelGroups: []ChannelGroup{
				{
					Name: "analog",
					IoTypes: []IoType{
						{Name: "voltage", Role: "output", Unit: "V"},
						{Name: "measured_voltage", Role: "input", Unit: "V"},
					},
				},
			},
		},
	}
	lib := BuildPortLibrary(apis)

	wireMap := map[string]string{
		"Source1.analog.4": "P1",
	}

	connected, err := ConnectWireMap(wireMap, lib)
	require.NoError(t, err)
	require.Len(t, connected, 2)

	knobs := 0
	meters := 0
	for _, cp := range connected {
		assert.Equal(t, "P1", cp.DeviceName)
		assert.Equal(t, "Source1", cp.InstrumentName)
		assert.Equal(t, "analog", cp.ChannelName)
		assert.Equal(t, 4, cp.ChannelIndex)
		if cp.IsKnob() {
			assert.Equal(t, "voltage", cp.IoTypeName)
			assert.Equal(t, "dc_voltage_source", cp.InstrumentType)
			knobs++
		}
		if cp.IsMeter() {
			assert.Equal(t, "measured_voltage", cp.IoTypeName)
			assert.Equal(t, "dc_voltage_source", cp.InstrumentType)
			meters++
		}
	}
	assert.Equal(t, 1, knobs)
	assert.Equal(t, 1, meters)
}

func TestBuildPortLibrary_UsesExplicitInstrumentTypes(t *testing.T) {
	apis := []InstrumentAPI{
		{
			Instrument: APIInstrument{
				Vendor:         "Mock",
				Identifier:     "Meter1",
				InstrumentType: "voltmeter",
			},
			Protocol: APIProtocol{
				Type: "MockMultimeter",
			},
			ChannelGroups: []ChannelGroup{
				{
					Name: "analog",
					IoTypes: []IoType{
						{Name: "current", Role: "input", Unit: "nA"},
						{Name: "voltage", Role: "input", Unit: "V"},
					},
				},
			},
		},
	}

	lib := BuildPortLibrary(apis)

	assert.Equal(t, "voltmeter", lib["Mock.Meter1.analog.current"].InstrumentType)
	assert.Equal(t, "voltmeter", lib["Mock.Meter1.analog.voltage"].InstrumentType)
}

func TestConnectWireMap_InvalidKey(t *testing.T) {
	lib := PortLibrary{}
	wireMap := map[string]string{
		"Source1.4": "P1", // missing channel name
	}
	_, err := ConnectWireMap(wireMap, lib)
	assert.Error(t, err)
}

func TestParseInstrumentAPI(t *testing.T) {
	content := `
api_version: "1.0.0"
instrument:
  vendor: Mock
  model: 1
  identifier: Source1
  instrument_type: dc_voltage_source
  description: Mock voltage source
channel_groups:
  - name: analog
    io_types:
      - name: voltage
        role: output
        unit: V
      - name: measured_voltage
        role: input
        unit: V
`
	tmpFile := filepath.Join(t.TempDir(), "source-api.yml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))

	api, err := ParseInstrumentAPI(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "Mock", api.Instrument.Vendor)
	assert.Equal(t, "Source1", api.Instrument.Identifier)
	require.Len(t, api.ChannelGroups, 1)
	assert.Equal(t, "analog", api.ChannelGroups[0].Name)
	require.Len(t, api.ChannelGroups[0].IoTypes, 2)
	assert.Equal(t, "dc_voltage_source", api.Instrument.InstrumentType)
}

func TestParseInstrumentAPI_RequiresInstrumentType(t *testing.T) {
	content := `instrument:
  vendor: Mock
  identifier: Source1
channel_groups:
  - name: analog
    io_types:
      - name: voltage
        role: output
`
	tmpFile := filepath.Join(t.TempDir(), "source-api.yml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))
	_, err := ParseInstrumentAPI(tmpFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing instrument.instrument_type")
}

func TestParseInstrumentAPI_RejectsUnsupportedInstrumentType(t *testing.T) {
	content := `instrument:
  vendor: Mock
  identifier: Source1
  instrument_type: arbitrary_type
channel_groups:
  - name: analog
    io_types:
      - name: voltage
        role: output
`
	tmpFile := filepath.Join(t.TempDir(), "source-api.yml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o600))
	_, err := ParseInstrumentAPI(tmpFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported instrument.instrument_type")
}

func TestResolveConnectedPort(t *testing.T) {
	h := newConnectedPorts(connectedromTestWireMap(t))

	tests := []struct {
		name         string
		deviceName   string
		ioTypeName   string
		role         string
		wantPortName PortName
		wantChannel  int
	}{
		{
			name:         "meter slope setting",
			deviceName:   "O1",
			ioTypeName:   "slope",
			role:         "setting",
			wantPortName: "Mock.Meter1.analog.slope",
			wantChannel:  1,
		},
		{
			name:         "meter trigger level setting",
			deviceName:   "O1",
			ioTypeName:   "trigger_level",
			role:         "setting",
			wantPortName: "Mock.Meter1.analog.trigger_level",
			wantChannel:  1,
		},
		{
			name:         "meter voltage input",
			deviceName:   "O1",
			ioTypeName:   "voltage",
			role:         "input",
			wantPortName: "Mock.Meter1.analog.voltage",
			wantChannel:  1,
		},
		{
			name:         "source voltage output",
			deviceName:   "P1",
			ioTypeName:   "voltage",
			role:         "output",
			wantPortName: "Mock.Source1.analog.voltage",
			wantChannel:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.ResolveConnectedPort(tt.deviceName, tt.ioTypeName, tt.role)
			if err != nil {
				t.Fatalf("ResolveConnectedPort returned error: %v", err)
			}
			if got.PortName != tt.wantPortName {
				t.Fatalf("PortName = %q, want %q", got.PortName, tt.wantPortName)
			}
			if got.ChannelIndex != tt.wantChannel {
				t.Fatalf("ChannelIndex = %d, want %d", got.ChannelIndex, tt.wantChannel)
			}
			if got.DeviceName != tt.deviceName {
				t.Fatalf("DeviceName = %q, want %q", got.DeviceName, tt.deviceName)
			}
			if got.IoTypeName != tt.ioTypeName {
				t.Fatalf("IoTypeName = %q, want %q", got.IoTypeName, tt.ioTypeName)
			}
			if got.Role != tt.role {
				t.Fatalf("Role = %q, want %q", got.Role, tt.role)
			}
		})
	}
}

func connectedromTestWireMap(t *testing.T) []ConnectedPort {
	t.Helper()

	apis := []InstrumentAPI{
		{
			Instrument: APIInstrument{
				Vendor:         "Mock",
				Identifier:     "Meter1",
				InstrumentType: "voltmeter",
			},
			Protocol: APIProtocol{
				Type: "MockMultimeter",
			},
			ChannelGroups: []ChannelGroup{
				{
					Name: "analog",
					IoTypes: []IoType{
						{Name: "voltage", Role: "input", Unit: "V"},
						{Name: "stream", Role: "input", Unit: "V"},
						{Name: "slope", Role: "setting"},
						{Name: "trigger_level", Role: "setting", Unit: "V"},
					},
				},
			},
		},
		{
			Instrument: APIInstrument{
				Vendor:         "Mock",
				Identifier:     "Source1",
				InstrumentType: "dc_voltage_source",
			},
			Protocol: APIProtocol{
				Type: "MockVoltageSource",
			},
			ChannelGroups: []ChannelGroup{
				{
					Name: "analog",
					IoTypes: []IoType{
						{Name: "voltage", Role: "output", Unit: "V"},
					},
				},
			},
		},
	}

	wireMap := map[string]string{
		"Meter1.analog.1":  "O1",
		"Source1.analog.4": "P1",
	}

	connected, err := ConnectWireMap(wireMap, BuildPortLibrary(apis))
	if err != nil {
		t.Fatalf("ConnectWireMap returned error: %v", err)
	}
	return connected
}

func TestResolveConnectedPortErrors(t *testing.T) {
	h := newConnectedPorts(
		[]ConnectedPort{
			{
				PortName:   "Mock.Meter1.analog.slope",
				DeviceName: "O1",
				IoTypeName: "slope",
				Role:       "setting",
			},
			{
				PortName:   "Mock.OtherMeter.analog.slope",
				DeviceName: "O1",
				IoTypeName: "slope",
				Role:       "setting",
			},
		},
	)

	if _, err := h.ResolveConnectedPort("O1", "voltage", "input"); err == nil {
		t.Fatal("expected no-match error")
	}

	_, err := h.ResolveConnectedPort("O1", "slope", "setting")
	if err == nil {
		t.Fatal("expected ambiguous-match error")
	}
	if !strings.Contains(err.Error(), "ambiguous connected port") {
		t.Fatalf("error = %q, want ambiguous connected port", err.Error())
	}
}
