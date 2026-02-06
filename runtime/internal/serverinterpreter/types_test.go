package serverinterpreter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstrumentTarget_Serialize(t *testing.T) {
	tests := []struct {
		name     string
		target   InstrumentTarget
		expected string
	}{
		{
			name:     "id only",
			target:   InstrumentTarget{Id: "DAC1", Channel: 0},
			expected: "DAC1",
		},
		{
			name:     "id with channel",
			target:   InstrumentTarget{Id: "DAC1", Channel: 3},
			expected: "DAC1:3",
		},
		{
			name:     "empty id",
			target:   InstrumentTarget{Id: "", Channel: 0},
			expected: "",
		},
		{
			name:     "channel 1",
			target:   InstrumentTarget{Id: "GPI1", Channel: 1},
			expected: "GPI1:1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.target.Serialize()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseMeasurementRequestJSON(t *testing.T) {
	t.Run("simple set_voltage request", func(t *testing.T) {
		jsonStr := `{
			"message": "test message",
			"measurementName": "set_voltage_test",
			"setters": [
				{"id": "DAC1", "channel": 0},
				{"id": "DAC1", "channel": 1}
			],
			"setVoltages": {
				"DAC1": 1.5,
				"DAC1:1": 2.5
			}
		}`

		parsed, err := ParseMeasurementRequestJSON(jsonStr)
		require.NoError(t, err)

		assert.Equal(t, "test message", parsed.Message)
		assert.Equal(t, "set_voltage_test", parsed.MeasurementName)
		assert.Len(t, parsed.Setters, 2)
		assert.Equal(t, "DAC1", parsed.Setters[0].Id)
		assert.Equal(t, 0, parsed.Setters[0].Channel)
		assert.Equal(t, "DAC1", parsed.Setters[1].Id)
		assert.Equal(t, 1, parsed.Setters[1].Channel)
		assert.Equal(t, 1.5, parsed.SetVoltages["DAC1"])
		assert.Equal(t, 2.5, parsed.SetVoltages["DAC1:1"])
	})

	t.Run("get_voltage request", func(t *testing.T) {
		jsonStr := `{
			"measurementName": "get_voltage_test",
			"getters": [
				{"id": "DMM1", "channel": 0}
			]
		}`

		parsed, err := ParseMeasurementRequestJSON(jsonStr)
		require.NoError(t, err)

		assert.Equal(t, "get_voltage_test", parsed.MeasurementName)
		assert.Len(t, parsed.Getters, 1)
		assert.Equal(t, "DMM1", parsed.Getters[0].Id)
	})

	t.Run("combined get/set request", func(t *testing.T) {
		jsonStr := `{
			"measurementName": "measure_get_set_test",
			"setters": [{"id": "DAC1", "channel": 0}],
			"getters": [{"id": "DMM1", "channel": 0}],
			"setVoltages": {"DAC1": 3.3}
		}`

		parsed, err := ParseMeasurementRequestJSON(jsonStr)
		require.NoError(t, err)

		assert.Equal(t, "measure_get_set_test", parsed.MeasurementName)
		assert.Len(t, parsed.Setters, 1)
		assert.Len(t, parsed.Getters, 1)
		assert.Equal(t, 3.3, parsed.SetVoltages["DAC1"])
	})

	t.Run("invalid JSON", func(t *testing.T) {
		jsonStr := `{invalid json`

		_, err := ParseMeasurementRequestJSON(jsonStr)
		assert.Error(t, err)
	})

	t.Run("empty JSON", func(t *testing.T) {
		jsonStr := `{}`

		parsed, err := ParseMeasurementRequestJSON(jsonStr)
		require.NoError(t, err)

		assert.Empty(t, parsed.Message)
		assert.Empty(t, parsed.MeasurementName)
		assert.Empty(t, parsed.Setters)
		assert.Empty(t, parsed.Getters)
	})
}

func TestParsedMeasurementRequest_ToSetVoltageRequests(t *testing.T) {
	t.Run("matching setters and voltages", func(t *testing.T) {
		parsed := &ParsedMeasurementRequest{
			Setters: []InstrumentTarget{
				{Id: "DAC1", Channel: 0},
				{Id: "DAC1", Channel: 1},
			},
			SetVoltages: map[string]float64{
				"DAC1":   1.5,
				"DAC1:1": 2.5,
			},
		}

		requests := parsed.ToSetVoltageRequests()

		assert.Len(t, requests, 2)
		assert.Equal(t, "DAC1", requests[0].Setter.Id)
		assert.Equal(t, 0, requests[0].Setter.Channel)
		assert.Equal(t, 1.5, requests[0].SetVoltage)
		assert.Equal(t, "DAC1", requests[1].Setter.Id)
		assert.Equal(t, 1, requests[1].Setter.Channel)
		assert.Equal(t, 2.5, requests[1].SetVoltage)
	})

	t.Run("setter without matching voltage", func(t *testing.T) {
		parsed := &ParsedMeasurementRequest{
			Setters: []InstrumentTarget{
				{Id: "DAC1", Channel: 0},
			},
			SetVoltages: map[string]float64{
				"DAC2": 1.5, // Different ID
			},
		}

		requests := parsed.ToSetVoltageRequests()

		assert.Empty(t, requests)
	})

	t.Run("empty setters", func(t *testing.T) {
		parsed := &ParsedMeasurementRequest{
			Setters:     nil,
			SetVoltages: map[string]float64{"DAC1": 1.5},
		}

		requests := parsed.ToSetVoltageRequests()

		assert.Empty(t, requests)
	})
}

func TestParsedMeasurementRequest_ToGetVoltageRequests(t *testing.T) {
	t.Run("multiple getters", func(t *testing.T) {
		parsed := &ParsedMeasurementRequest{
			Getters: []InstrumentTarget{
				{Id: "DMM1", Channel: 0},
				{Id: "DMM2", Channel: 1},
			},
		}

		requests := parsed.ToGetVoltageRequests()

		assert.Len(t, requests, 2)
		assert.Equal(t, "DMM1", requests[0].Getter.Id)
		assert.Equal(t, 0, requests[0].Getter.Channel)
		assert.Equal(t, "DMM2", requests[1].Getter.Id)
		assert.Equal(t, 1, requests[1].Getter.Channel)
	})

	t.Run("empty getters", func(t *testing.T) {
		parsed := &ParsedMeasurementRequest{}

		requests := parsed.ToGetVoltageRequests()

		assert.Empty(t, requests)
	})
}

func TestSetVoltageRequest_JSON(t *testing.T) {
	req := SetVoltageRequest{
		Setter:     InstrumentTarget{Id: "DAC1", Channel: 2},
		SetVoltage: 3.14159,
	}

	jsonData, err := json.Marshal(req)
	require.NoError(t, err)

	var parsed SetVoltageRequest
	err = json.Unmarshal(jsonData, &parsed)
	require.NoError(t, err)

	assert.Equal(t, req.Setter.Id, parsed.Setter.Id)
	assert.Equal(t, req.Setter.Channel, parsed.Setter.Channel)
	assert.InDelta(t, req.SetVoltage, parsed.SetVoltage, 0.00001)
}

func TestGetVoltageRequest_JSON(t *testing.T) {
	req := GetVoltageRequest{
		Getter: InstrumentTarget{Id: "DMM1", Channel: 0},
	}

	jsonData, err := json.Marshal(req)
	require.NoError(t, err)

	var parsed GetVoltageRequest
	err = json.Unmarshal(jsonData, &parsed)
	require.NoError(t, err)

	assert.Equal(t, req.Getter.Id, parsed.Getter.Id)
	assert.Equal(t, req.Getter.Channel, parsed.Getter.Channel)
}

func TestMeasurementResponse_JSON(t *testing.T) {
	resp := MeasurementResponse{
		Instrument: "DMM1",
		Verb:       "GET_VOLTAGE",
		Type:       "float",
		Value:      2.718,
	}

	jsonData, err := json.Marshal(resp)
	require.NoError(t, err)

	var parsed MeasurementResponse
	err = json.Unmarshal(jsonData, &parsed)
	require.NoError(t, err)

	assert.Equal(t, resp.Instrument, parsed.Instrument)
	assert.Equal(t, resp.Verb, parsed.Verb)
	assert.Equal(t, resp.Type, parsed.Type)
	// Value will be parsed as float64
	assert.InDelta(t, 2.718, parsed.Value.(float64), 0.001)
}

func TestExecutionResult_ToSerializedResponse(t *testing.T) {
	result := &ExecutionResult{
		JobID:  "job_123",
		Status: "completed",
		Results: []MeasurementResponse{
			{
				Instrument: "DMM1",
				Verb:       "GET_VOLTAGE",
				Type:       "float",
				Value:      1.234,
			},
		},
	}

	serialized, err := result.ToSerializedResponse()
	require.NoError(t, err)

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(serialized), &parsed)
	require.NoError(t, err)

	assert.Equal(t, "job_123", parsed["job_id"])
	assert.Equal(t, "completed", parsed["status"])
}

func TestRPCRequest_JSON(t *testing.T) {
	req := RPCRequest{
		Command: "submit_measure",
		Params: SubmitMeasureParams{
			ScriptPath: "/path/to/script.lua",
		},
	}

	jsonData, err := json.Marshal(req)
	require.NoError(t, err)

	// Verify structure
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonData, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "submit_measure", parsed["command"])
	params := parsed["params"].(map[string]interface{})
	assert.Equal(t, "/path/to/script.lua", params["script_path"])
}
