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

