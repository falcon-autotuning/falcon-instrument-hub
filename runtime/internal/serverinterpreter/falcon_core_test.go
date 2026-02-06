package serverinterpreter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFalconMeasurementRequest tests JSON parsing without falcon-core.
func TestFalconMeasurementRequest_ParseSimpleJSON(t *testing.T) {
	jsonStr := `{
		"message": "test_message",
		"measurementName": "DC_GetSet_Test",
		"setters": [
			{"id": "DAC1", "channel": 0},
			{"id": "DAC1", "channel": 1}
		],
		"getters": [
			{"id": "DMM1", "channel": 0}
		],
		"setVoltages": {
			"DAC1": 1.5,
			"DAC1:1": 2.0
		}
	}`

	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	require.NotNil(t, req)
	defer req.Close()

	// Test Message
	msg, err := req.Message()
	require.NoError(t, err)
	assert.Equal(t, "test_message", msg)

	// Test MeasurementName
	name, err := req.MeasurementName()
	require.NoError(t, err)
	assert.Equal(t, "DC_GetSet_Test", name)
}

func TestFalconMeasurementRequest_ExtractGetters(t *testing.T) {
	jsonStr := `{
		"getters": [
			{"id": "DMM1", "channel": 0, "is_meter": true, "description": "Digital Multimeter"},
			{"id": "OSC1", "channel": 1, "instrument_type": "oscilloscope"}
		]
	}`

	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	defer req.Close()

	getters, err := req.ExtractGetters()
	require.NoError(t, err)
	assert.Len(t, getters, 2)

	// First getter
	assert.Equal(t, "DMM1", getters[0].DefaultName)
	assert.True(t, getters[0].IsMeter)
	assert.Equal(t, "Digital Multimeter", getters[0].Description)

	// Second getter
	assert.Equal(t, "OSC1", getters[1].DefaultName)
	assert.Equal(t, "oscilloscope", getters[1].InstrumentType)
}

func TestFalconMeasurementRequest_ExtractSettersFromSimpleFormat(t *testing.T) {
	jsonStr := `{
		"setters": [
			{"id": "DAC1", "channel": 0, "is_knob": true},
			{"id": "DAC2", "channel": 1}
		]
	}`

	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	defer req.Close()

	setters, err := req.ExtractSetters()
	require.NoError(t, err)
	assert.Len(t, setters, 2)

	assert.Equal(t, "DAC1", setters[0].DefaultName)
	assert.True(t, setters[0].IsKnob)
	assert.Equal(t, "DAC2", setters[1].DefaultName)
}

func TestFalconMeasurementRequest_ExtractSettersFromWaveforms(t *testing.T) {
	jsonStr := `{
		"waveforms": [
			{
				"transforms": [
					{
						"port": {
							"default_name": "DAC1",
							"instrument_facing_name": "Channel_A",
							"is_knob": true
						}
					}
				]
			},
			{
				"transforms": [
					{
						"port": {
							"default_name": "DAC2",
							"instrument_facing_name": "Channel_B",
							"is_knob": true
						}
					}
				]
			}
		]
	}`

	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	defer req.Close()

	setters, err := req.ExtractSetters()
	require.NoError(t, err)
	assert.Len(t, setters, 2)

	assert.Equal(t, "DAC1", setters[0].DefaultName)
	assert.Equal(t, "Channel_A", setters[0].InstrumentFacingName)
	assert.True(t, setters[0].IsKnob)

	assert.Equal(t, "DAC2", setters[1].DefaultName)
	assert.Equal(t, "Channel_B", setters[1].InstrumentFacingName)
}

func TestFalconMeasurementRequest_ToJSON(t *testing.T) {
	originalJSON := `{"message":"test","data":123}`

	req, err := NewFalconMeasurementRequestFromJSON(originalJSON)
	require.NoError(t, err)
	defer req.Close()

	resultJSON, err := req.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, originalJSON, resultJSON)
}

func TestFalconMeasurementResponse(t *testing.T) {
	jsonStr := `{"message": "success", "data": [1.0, 2.0, 3.0]}`

	resp, err := NewFalconMeasurementResponseFromJSON(jsonStr)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Close()

	msg, err := resp.Message()
	require.NoError(t, err)
	assert.Equal(t, "success", msg)

	resultJSON, err := resp.ToJSON()
	require.NoError(t, err)
	// Should be valid JSON
	var parsed interface{}
	require.NoError(t, json.Unmarshal([]byte(resultJSON), &parsed))
}

func TestExtractWaveformDataFromRequest(t *testing.T) {
	jsonStr := `{
		"getters": [
			{"id": "DMM1", "channel": 0}
		],
		"setters": [
			{"id": "DAC1", "channel": 0}
		],
		"time_domain": {
			"domain": {
				"bounds": [0.0, 0.01]
			}
		}
	}`

	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	defer req.Close()

	waveformData, getters, err := ExtractWaveformDataFromRequest(req)
	require.NoError(t, err)
	require.NotNil(t, waveformData)
	assert.Len(t, getters, 1)

	// Verify time domain was extracted
	assert.Equal(t, 0.0, waveformData.TimeDomain.Min)
	assert.Equal(t, 0.01, waveformData.TimeDomain.Max)

	// Verify axis domains were extracted
	assert.Len(t, waveformData.AxisDomains, 1)
}

func TestGettersToJSONList(t *testing.T) {
	jsonStr := `{
		"getters": [
			{"id": "DMM1", "channel": 0},
			{"id": "OSC1", "channel": 1}
		]
	}`

	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	defer req.Close()

	portJSONs, err := GettersToJSONList(req)
	require.NoError(t, err)
	assert.Len(t, portJSONs, 2)

	// Each should be valid JSON
	for _, pj := range portJSONs {
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(pj), &parsed))
		assert.Contains(t, parsed, "id")
	}
}

func TestSettersToJSONList(t *testing.T) {
	jsonStr := `{
		"setters": [
			{"id": "DAC1", "channel": 0},
			{"id": "DAC2", "channel": 1}
		]
	}`

	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	defer req.Close()

	portJSONs, err := SettersToJSONList(req)
	require.NoError(t, err)
	assert.Len(t, portJSONs, 2)

	// Each should be valid JSON
	for _, pj := range portJSONs {
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(pj), &parsed))
		assert.Contains(t, parsed, "id")
	}
}

func TestExtractWaveformDataFromRequest_NilRequest(t *testing.T) {
	_, _, err := ExtractWaveformDataFromRequest(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestFalconMeasurementRequest_InvalidJSON(t *testing.T) {
	_, err := NewFalconMeasurementRequestFromJSON("not valid json")
	assert.Error(t, err)
}

// TestFalconCoreAsFallback verifies that the pure-Go implementation
// can be used as a functional fallback when falcon-core is not available.
func TestFalconCoreAsFallback(t *testing.T) {
	// Create a complete measurement request JSON
	jsonStr := `{
		"message": "DC_Sweep_Test", 
		"measurementName": "voltage_sweep",
		"setters": [
			{"id": "DAC1", "channel": 0, "is_knob": true}
		],
		"getters": [
			{"id": "DMM1", "channel": 0, "is_meter": true}
		],
		"setVoltages": {"DAC1": 1.5},
		"time_domain": {
			"domain": {"bounds": [0, 0.001]}
		}
	}`

	// Verify all functionality works
	req, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	require.NoError(t, err)
	defer req.Close()

	// Message
	msg, err := req.Message()
	require.NoError(t, err)
	assert.Equal(t, "DC_Sweep_Test", msg)

	// Name
	name, err := req.MeasurementName()
	require.NoError(t, err)
	assert.Equal(t, "voltage_sweep", name)

	// Getters
	gList, err := GettersToJSONList(req)
	require.NoError(t, err)
	assert.Len(t, gList, 1)

	// Setters
	sList, err := SettersToJSONList(req)
	require.NoError(t, err)
	assert.Len(t, sList, 1)

	// Waveform extraction
	wf, getters, err := ExtractWaveformDataFromRequest(req)
	require.NoError(t, err)
	assert.NotNil(t, wf)
	assert.Len(t, getters, 1)
}
