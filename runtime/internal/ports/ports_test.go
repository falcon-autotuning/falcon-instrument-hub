package ports_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
)

func TestBuildPortLibrary(t *testing.T) {
	apis := []ports.InstrumentAPI{
		{
			Instrument: ports.APIInstrument{
				Vendor:         "Mock",
				Identifier:     "Source1",
				InstrumentType: "dc_voltage_source",
			},
			Protocol: ports.APIProtocol{
				Type: "MockVoltageSource",
			},
			ChannelGroups: []ports.ChannelGroup{
				{
					Name: "analog",
					IoTypes: []ports.IoType{
						{Name: "voltage", Role: "output", Unit: "V"},
						{Name: "measured_voltage", Role: "input", Unit: "V"},
					},
				},
			},
		},
	}

	lib := ports.BuildPortLibrary(apis)

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
	apis := []ports.InstrumentAPI{
		{
			Instrument: ports.APIInstrument{
				Vendor:         "Mock",
				Identifier:     "Source1",
				InstrumentType: "dc_voltage_source",
			},
			Protocol: ports.APIProtocol{
				Type: "MockVoltageSource",
			},
			ChannelGroups: []ports.ChannelGroup{
				{
					Name: "analog",
					IoTypes: []ports.IoType{
						{Name: "voltage", Role: "output", Unit: "V"},
						{Name: "measured_voltage", Role: "input", Unit: "V"},
					},
				},
			},
		},
	}
	lib := ports.BuildPortLibrary(apis)

	wireMap := map[string]string{
		"Source1.analog.4": "P1",
	}

	connected, err := ports.ConnectWireMap(wireMap, lib)
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
	apis := []ports.InstrumentAPI{
		{
			Instrument: ports.APIInstrument{
				Vendor:         "Mock",
				Identifier:     "Meter1",
				InstrumentType: "voltmeter",
			},
			Protocol: ports.APIProtocol{
				Type: "MockMultimeter",
			},
			ChannelGroups: []ports.ChannelGroup{
				{
					Name: "analog",
					IoTypes: []ports.IoType{
						{Name: "current", Role: "input", Unit: "nA"},
						{Name: "voltage", Role: "input", Unit: "V"},
					},
				},
			},
		},
	}

	lib := ports.BuildPortLibrary(apis)

	assert.Equal(t, "voltmeter", lib["Mock.Meter1.analog.current"].InstrumentType)
	assert.Equal(t, "voltmeter", lib["Mock.Meter1.analog.voltage"].InstrumentType)
}

func TestConnectWireMap_InvalidKey(t *testing.T) {
	lib := ports.PortLibrary{}
	wireMap := map[string]string{
		"Source1.4": "P1", // missing channel name
	}
	_, err := ports.ConnectWireMap(wireMap, lib)
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

	api, err := ports.ParseInstrumentAPI(tmpFile)
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
	_, err := ports.ParseInstrumentAPI(tmpFile)
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
	_, err := ports.ParseInstrumentAPI(tmpFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported instrument.instrument_type")
}
