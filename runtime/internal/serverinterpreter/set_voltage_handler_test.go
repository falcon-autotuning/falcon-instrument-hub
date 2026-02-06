package serverinterpreter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetVoltageCommand_Serialize(t *testing.T) {
	tests := []struct {
		name     string
		target   InstrumentPortTarget
		expected string
	}{
		{
			name: "no channel",
			target: InstrumentPortTarget{
				InstrumentID: "DAC1",
			},
			expected: "DAC1",
		},
		{
			name: "with channel 0",
			target: InstrumentPortTarget{
				InstrumentID: "DAC1",
				Channel:      intPtr(0),
			},
			expected: "DAC1",
		},
		{
			name: "with channel 1",
			target: InstrumentPortTarget{
				InstrumentID: "DAC1",
				Channel:      intPtr(1),
			},
			expected: "DAC1:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.target.Serialize()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func TestSetVoltageCommand_ParseJSON(t *testing.T) {
	jsonStr := `{
		"setter": {
			"instrument_id": "DAC1",
			"channel": 0
		},
		"setVoltage": 1.5
	}`

	var cmd SetVoltageCommand
	err := json.Unmarshal([]byte(jsonStr), &cmd)
	require.NoError(t, err)

	require.NotNil(t, cmd.Setter)
	assert.Equal(t, "DAC1", cmd.Setter.InstrumentID)
	assert.Equal(t, 0, cmd.Setter.GetChannel())
	assert.Equal(t, 1.5, cmd.SetVoltage)
}

func TestSetVoltageLuaGenerator_GenerateCallStatement(t *testing.T) {
	gen := NewSetVoltageLuaGenerator()

	cmd := SetVoltageCommand{
		Setter: &InstrumentPortTarget{
			InstrumentID: "DAC1",
			Channel:      intPtr(0),
		},
		SetVoltage: 2.5,
	}

	lua := gen.GenerateCallStatement(cmd)

	assert.Contains(t, lua, "DAC1.SET_VOLTAGE")
	assert.Contains(t, lua, "channel = 0")
	assert.Contains(t, lua, "voltage = 2.5")
}

func TestSetVoltageLuaGenerator_GenerateParallelBlock(t *testing.T) {
	gen := NewSetVoltageLuaGenerator()

	cmds := []SetVoltageCommand{
		{
			Setter: &InstrumentPortTarget{
				InstrumentID: "DAC1",
			},
			SetVoltage: 1.0,
		},
		{
			Setter: &InstrumentPortTarget{
				InstrumentID: "DAC2",
			},
			SetVoltage: 2.0,
		},
	}

	lua := gen.GenerateParallelBlock(cmds)

	assert.Contains(t, lua, "ctx:parallel(function()")
	assert.Contains(t, lua, "DAC1.SET_VOLTAGE")
	assert.Contains(t, lua, "DAC2.SET_VOLTAGE")
	assert.Contains(t, lua, "end)")
}

func TestSetVoltageLuaGenerator_SingleCommand_NoParallel(t *testing.T) {
	gen := NewSetVoltageLuaGenerator()

	cmds := []SetVoltageCommand{
		{
			Setter: &InstrumentPortTarget{
				InstrumentID: "DAC1",
			},
			SetVoltage: 1.5,
		},
	}

	lua := gen.GenerateParallelBlock(cmds)

	// Should not wrap in parallel for single command
	assert.NotContains(t, lua, "ctx:parallel")
	assert.Contains(t, lua, "DAC1.SET_VOLTAGE")
}

func TestSetVoltageFromMeasurementRequest_ExtractCommands(t *testing.T) {
	jsonStr := `{
		"measurementName": "voltage_sweep",
		"setters": [
			{"id": "DAC1", "channel": 0},
			{"id": "DAC2", "channel": 1}
		],
		"setVoltages": {
			"DAC1": 1.5,
			"DAC2:1": 2.0
		}
	}`

	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	defer req.Close()

	// Create extractor (bridge not needed for extraction)
	extractor := &SetVoltageFromMeasurementRequest{}
	cmds, err := extractor.ExtractCommands(req)
	require.NoError(t, err)

	assert.Len(t, cmds, 2)

	// First command
	assert.Equal(t, "DAC1", cmds[0].Setter.InstrumentID)
	assert.Equal(t, 1.5, cmds[0].SetVoltage)

	// Second command
	assert.Equal(t, "DAC2", cmds[1].Setter.InstrumentID)
}

func TestSetVoltageFromMeasurementRequest_ExtractFromWaveforms(t *testing.T) {
	jsonStr := `{
		"waveforms": [
			{
				"constant_value": 3.3,
				"transforms": [
					{
						"port": {
							"default_name": "DAC1"
						}
					}
				]
			}
		]
	}`

	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	defer req.Close()

	extractor := &SetVoltageFromMeasurementRequest{}
	cmds, err := extractor.ExtractCommands(req)
	require.NoError(t, err)

	assert.Len(t, cmds, 1)
	assert.Equal(t, "DAC1", cmds[0].Setter.InstrumentID)
	assert.Equal(t, 3.3, cmds[0].SetVoltage)
}

func TestSetVoltageResult_ToJSON(t *testing.T) {
	actualVoltage := 1.499
	result := SetVoltageResult{
		Success: true,
		Setter: &InstrumentPortTarget{
			InstrumentID: "DAC1",
		},
		SetVoltage:    1.5,
		ActualVoltage: &actualVoltage,
	}

	jsonStr, err := result.ToJSON()
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &parsed))

	assert.True(t, parsed["success"].(bool))
	assert.Equal(t, 1.5, parsed["setVoltage"])
	assert.InDelta(t, 1.499, parsed["actualVoltage"], 0.001)
}

func TestSetVoltageHandler_WithMockServer(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := mock.URL()[7:] // remove http://
	var host string
	var port int
	_ = json.Unmarshal([]byte(`"`+urlParts+`"`), &host)
	
	// Parse host:port
	for i := len(urlParts) - 1; i >= 0; i-- {
		if urlParts[i] == ':' {
			host = urlParts[:i]
			json.Unmarshal([]byte(urlParts[i+1:]), &port)
			break
		}
	}

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
		ScriptOutputDir:  t.TempDir(),
	}

	bridge, err := NewBridge(config)
	require.NoError(t, err)

	handler := NewSetVoltageHandler(bridge)

	cmd := SetVoltageCommand{
		Setter: &InstrumentPortTarget{
			InstrumentID: "DAC1",
			Channel:      intPtr(0),
		},
		SetVoltage: 2.5,
	}

	result, err := handler.Execute(cmd)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.JobID, "mock_job_")
	assert.Equal(t, "completed", result.Status)
}

func TestSetVoltageHandler_ExecuteFromJSON(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := mock.URL()[7:]
	var host string
	var port int
	for i := len(urlParts) - 1; i >= 0; i-- {
		if urlParts[i] == ':' {
			host = urlParts[:i]
			json.Unmarshal([]byte(urlParts[i+1:]), &port)
			break
		}
	}

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
		ScriptOutputDir:  t.TempDir(),
	}

	bridge, err := NewBridge(config)
	require.NoError(t, err)

	handler := NewSetVoltageHandler(bridge)

	jsonStr := `{
		"setter": {
			"instrument_id": "DAC1",
			"channel": 0
		},
		"setVoltage": 1.75
	}`

	result, err := handler.ExecuteFromJSON(jsonStr)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "completed", result.Status)
}

func TestSetVoltageHandler_NilSetter(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := mock.URL()[7:]
	var host string
	var port int
	for i := len(urlParts) - 1; i >= 0; i-- {
		if urlParts[i] == ':' {
			host = urlParts[:i]
			json.Unmarshal([]byte(urlParts[i+1:]), &port)
			break
		}
	}

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
		ScriptOutputDir:  t.TempDir(),
	}

	bridge, err := NewBridge(config)
	require.NoError(t, err)

	handler := NewSetVoltageHandler(bridge)

	cmd := SetVoltageCommand{
		Setter:     nil,
		SetVoltage: 1.0,
	}

	_, err = handler.Execute(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "setter is required")
}
