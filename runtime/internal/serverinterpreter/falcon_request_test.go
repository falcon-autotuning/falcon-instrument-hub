// Package serverinterpreter provides tests for falcon measurement request types.
//
// These tests validate that the hub can correctly parse and process measurement
// requests for each script type defined in falcon-measurement-lib/schemas/scripts.
package serverinterpreter

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Common Types from falcon-measurement-lib (test versions with JSON tags)
// These types extend the base types in falcon_request_router.go with JSON tags
// for wire format serialization.
// =============================================================================

// FalconInstrumentTargetJSON matches falcon-measurement-lib/schemas/lib/instrument_target.json
type FalconInstrumentTargetJSON struct {
	ID      string `json:"id"`
	Channel int    `json:"channel,omitempty"`
}

// FalconDomainJSON matches falcon-measurement-lib/schemas/lib/domain.json (with JSON tags)
// Note: Use FalconDomain from falcon_request_router.go for production code
type FalconDomainJSON struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// FalconMeasurementResponseJSON matches falcon-measurement-lib/schemas/lib/measurement_response.json
// This is distinct from the FalconMeasurementResponse used internally
type FalconMeasurementResponseJSON struct {
	Instrument string      `json:"instrument"`
	Verb       string      `json:"verb"`
	Type       string      `json:"type"`
	Value      interface{} `json:"value"`
}

// =============================================================================
// Script Request Types - Basic Voltage Operations
// =============================================================================

// SetVoltageRequestJSON matches set_voltage.json (JSON wire format)
type SetVoltageRequestJSON struct {
	Setter     FalconInstrumentTargetJSON `json:"setter"`
	SetVoltage float64                    `json:"setVoltage"`
}

// GetVoltageRequestSchema matches get_voltage.json
type GetVoltageRequestJSON struct {
	Getter FalconInstrumentTargetJSON `json:"getter"`
}

// SetManyVoltagesRequestJSON matches set_many_voltages.json
type SetManyVoltagesRequestJSON struct {
	Setters     []FalconInstrumentTargetJSON `json:"setters"`
	SetVoltages map[string]float64           `json:"setVoltages"`
}

// GetManyVoltagesRequestJSON matches get_many_voltages.json
type GetManyVoltagesRequestJSON struct {
	Getters []FalconInstrumentTargetJSON `json:"getters"`
}

// GetAllVoltagesRequestJSON matches get_all_voltages.json
type GetAllVoltagesRequestJSON struct {
	Getters []FalconInstrumentTargetJSON `json:"getters"`
}

// RampRequestJSON matches ramp.json
type RampRequestJSON struct {
	Setters     []FalconInstrumentTargetJSON `json:"setters"`
	SetVoltages map[string]float64           `json:"setVoltages"`
}

// =============================================================================
// Script Request Types - Measurement Operations
// =============================================================================

// MeasureGetSetRequestJSON matches measure_get_set.json
type MeasureGetSetRequestJSON struct {
	Setters     []FalconInstrumentTargetJSON `json:"setters"`
	Getters     []FalconInstrumentTargetJSON `json:"getters"`
	SetVoltages map[string]float64           `json:"setVoltages"`
	SampleRate  float64                      `json:"sampleRate,omitempty"`
	NumPoints   int                          `json:"numPoints,omitempty"`
}

// MeasureCurrentRequestJSON matches measure_current.json
type MeasureCurrentRequestJSON struct {
	Getters    []FalconInstrumentTargetJSON `json:"getters"`
	SampleRate float64                      `json:"sampleRate,omitempty"`
}

// MeasureLeakageRequestJSON matches measure_leakage.json
type MeasureLeakageRequestJSON struct {
	Getter  FalconInstrumentTargetJSON `json:"getter"`
	Voltage float64                    `json:"voltage"`
}

// MeasureIlluminationRequestJSON matches measure_illumination.json
type MeasureIlluminationRequestJSON struct {
	Getters          []FalconInstrumentTargetJSON `json:"getters"`
	SampleRate       float64                      `json:"sampleRate,omitempty"`
	IlluminationTime float64                      `json:"illuminationTime"`
}

// =============================================================================
// Script Request Types - Buffered 1D/2D Sweeps
// =============================================================================

// Measure1DBufferedRequestJSON matches measure_1D_buffered.json
type Measure1DBufferedRequestJSON struct {
	Setters           []FalconInstrumentTargetJSON `json:"setters,omitempty"`
	BufferedSetters   []FalconInstrumentTargetJSON `json:"bufferedSetters,omitempty"`
	BufferedGetters   []FalconInstrumentTargetJSON `json:"bufferedGetters"`
	SetVoltageDomains map[string]FalconDomain      `json:"setVoltageDomains,omitempty"`
	SampleRate        float64                      `json:"sampleRate"`
	NumPoints         int                          `json:"numPoints"`
	NumSteps          int                          `json:"numSteps"`
}

// Measure2DBufferedRequestJSON matches measure_2D_buffered.json
type Measure2DBufferedRequestJSON struct {
	Setters            []FalconInstrumentTargetJSON `json:"setters,omitempty"`
	BufferedXSetters   []FalconInstrumentTargetJSON `json:"bufferedXSetters,omitempty"`
	BufferedYSetters   []FalconInstrumentTargetJSON `json:"bufferedYSetters,omitempty"`
	BufferedGetters    []FalconInstrumentTargetJSON `json:"bufferedGetters"`
	SetXVoltageDomains map[string]FalconDomain      `json:"setXVoltageDomains,omitempty"`
	SetYVoltageDomains map[string]FalconDomain      `json:"setYVoltageDomains,omitempty"`
	SampleRate         float64                      `json:"sampleRate"`
	NumPoints          int                          `json:"numPoints"`
	NumXSteps          int                          `json:"numXSteps"`
	NumYSteps          int                          `json:"numYSteps"`
}

// =============================================================================
// Script Request Types - ADC/DAC Configuration
// =============================================================================

// SetSampleRateRequestJSON matches set_sample_rate.json
type SetSampleRateRequestJSON struct {
	Getter     FalconInstrumentTargetJSON `json:"getter"`
	SampleRate float64                    `json:"sampleRate"`
}

// GetSampleRateRequestJSON matches get_sample_rate.json
type GetSampleRateRequestJSON struct {
	Getter FalconInstrumentTargetJSON `json:"getter"`
}

// SetNumberOfSamplesRequestJSON matches set_number_of_samples.json
type SetNumberOfSamplesRequestJSON struct {
	Getter          FalconInstrumentTargetJSON `json:"getter"`
	NumberOfSamples float64                    `json:"numberOfSamples"`
}

// GetNumberOfSamplesRequestJSON matches get_number_of_samples.json
type GetNumberOfSamplesRequestJSON struct {
	Getter FalconInstrumentTargetJSON `json:"getter"`
}

// SetSlopeRequestJSON matches set_slope.json
type SetSlopeRequestJSON struct {
	Setter FalconInstrumentTargetJSON `json:"setter"`
	Slope  float64                    `json:"slope"`
}

// GetSlopeRequestJSON matches get_slope.json
type GetSlopeRequestJSON struct {
	Getter FalconInstrumentTargetJSON `json:"getter"`
}

// SetTriggerLeaderRequestJSON matches set_trigger_leader.json
type SetTriggerLeaderRequestJSON struct {
	Getter FalconInstrumentTargetJSON `json:"getter"`
}

// GetTriggerLeaderRequestJSON matches get_trigger_leader.json
type GetTriggerLeaderRequestJSON struct {
	Getter FalconInstrumentTargetJSON `json:"getter"`
}

// =============================================================================
// Test: Basic Voltage Operations
// =============================================================================

func TestFalconRequest_SetVoltage(t *testing.T) {
	// Simulate falcon sending a set_voltage request
	reqJSON := `{
		"setter": {"id": "QDAC1", "channel": 1},
		"setVoltage": -0.5
	}`

	var req SetVoltageRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "QDAC1", req.Setter.ID)
	assert.Equal(t, 1, req.Setter.Channel)
	assert.Equal(t, -0.5, req.SetVoltage)

	// Convert to hub internal type
	hubReq := SetVoltageRequest{
		Setter:     InstrumentTarget{Id: req.Setter.ID, Channel: req.Setter.Channel},
		SetVoltage: req.SetVoltage,
	}
	assert.Equal(t, "QDAC1", hubReq.Setter.Id)
}

func TestFalconRequest_GetVoltage(t *testing.T) {
	reqJSON := `{
		"getter": {"id": "DMM1", "channel": 0}
	}`

	var req GetVoltageRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "DMM1", req.Getter.ID)
	assert.Equal(t, 0, req.Getter.Channel)
}

func TestFalconRequest_SetManyVoltages(t *testing.T) {
	// Typical quantum dot scenario: set all gate voltages
	reqJSON := `{
		"setters": [
			{"id": "QDAC1", "channel": 1},
			{"id": "QDAC1", "channel": 2},
			{"id": "QDAC1", "channel": 3},
			{"id": "QDAC1", "channel": 4},
			{"id": "QDAC1", "channel": 5}
		],
		"setVoltages": {
			"QDAC1:1": -0.5,
			"QDAC1:2": -0.6,
			"QDAC1:3": -1.0,
			"QDAC1:4": -1.0,
			"QDAC1:5": -1.0
		}
	}`

	var req SetManyVoltagesRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Len(t, req.Setters, 5)
	assert.Len(t, req.SetVoltages, 5)
	assert.Equal(t, -0.5, req.SetVoltages["QDAC1:1"])
	assert.Equal(t, -1.0, req.SetVoltages["QDAC1:3"])
}

func TestFalconRequest_GetManyVoltages(t *testing.T) {
	reqJSON := `{
		"getters": [
			{"id": "QDAC1", "channel": 1},
			{"id": "QDAC1", "channel": 2},
			{"id": "QDAC1", "channel": 3}
		]
	}`

	var req GetManyVoltagesRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Len(t, req.Getters, 3)
}

func TestFalconRequest_Ramp(t *testing.T) {
	// Ramp all gates to new position
	reqJSON := `{
		"setters": [
			{"id": "QDAC1", "channel": 1},
			{"id": "QDAC1", "channel": 2}
		],
		"setVoltages": {
			"QDAC1:1": -0.8,
			"QDAC1:2": -0.7
		}
	}`

	var req RampRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Len(t, req.Setters, 2)
	assert.Equal(t, -0.8, req.SetVoltages["QDAC1:1"])
}

// =============================================================================
// Test: Measurement Operations
// =============================================================================

func TestFalconRequest_MeasureGetSet(t *testing.T) {
	// Typical DC measurement: set gates, measure current
	reqJSON := `{
		"setters": [
			{"id": "QDAC1", "channel": 1},
			{"id": "QDAC1", "channel": 2}
		],
		"getters": [
			{"id": "DMM1", "channel": 0}
		],
		"setVoltages": {
			"QDAC1:1": -0.5,
			"QDAC1:2": -0.6
		},
		"sampleRate": 1000,
		"numPoints": 100
	}`

	var req MeasureGetSetRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Len(t, req.Setters, 2)
	assert.Len(t, req.Getters, 1)
	assert.Equal(t, float64(1000), req.SampleRate)
	assert.Equal(t, 100, req.NumPoints)
}

func TestFalconRequest_MeasureCurrent(t *testing.T) {
	reqJSON := `{
		"getters": [
			{"id": "DMM1", "channel": 0},
			{"id": "LOCKIN1", "channel": 0}
		],
		"sampleRate": 10000
	}`

	var req MeasureCurrentRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Len(t, req.Getters, 2)
	assert.Equal(t, float64(10000), req.SampleRate)
}

func TestFalconRequest_MeasureLeakage(t *testing.T) {
	// Measure gate leakage at elevated voltage
	reqJSON := `{
		"getter": {"id": "AMMETER1", "channel": 0},
		"voltage": 1.0
	}`

	var req MeasureLeakageRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "AMMETER1", req.Getter.ID)
	assert.Equal(t, 1.0, req.Voltage)
}

func TestFalconRequest_MeasureIllumination(t *testing.T) {
	// LED illumination to reset 2DEG
	reqJSON := `{
		"getters": [
			{"id": "DMM1", "channel": 0}
		],
		"sampleRate": 1000,
		"illuminationTime": 5.0
	}`

	var req MeasureIlluminationRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Len(t, req.Getters, 1)
	assert.Equal(t, 5.0, req.IlluminationTime)
}

// =============================================================================
// Test: Buffered 1D/2D Sweeps
// =============================================================================

func TestFalconRequest_Measure1DBuffered(t *testing.T) {
	// 1D sweep: sweep P1 while measuring current
	reqJSON := `{
		"setters": [
			{"id": "QDAC1", "channel": 2},
			{"id": "QDAC1", "channel": 3}
		],
		"bufferedSetters": [
			{"id": "QDAC1", "channel": 1}
		],
		"bufferedGetters": [
			{"id": "DMM1", "channel": 0}
		],
		"setVoltageDomains": {
			"QDAC1:1": {"min": -1.0, "max": 0.0}
		},
		"sampleRate": 10000,
		"numPoints": 100,
		"numSteps": 101
	}`

	var req Measure1DBufferedRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Len(t, req.Setters, 2)
	assert.Len(t, req.BufferedSetters, 1)
	assert.Len(t, req.BufferedGetters, 1)
	assert.Equal(t, -1.0, req.SetVoltageDomains["QDAC1:1"].Min)
	assert.Equal(t, 0.0, req.SetVoltageDomains["QDAC1:1"].Max)
	assert.Equal(t, 101, req.NumSteps)
}

func TestFalconRequest_Measure2DBuffered(t *testing.T) {
	// 2D sweep: charge stability diagram
	reqJSON := `{
		"setters": [
			{"id": "QDAC1", "channel": 3},
			{"id": "QDAC1", "channel": 4}
		],
		"bufferedXSetters": [
			{"id": "QDAC1", "channel": 1}
		],
		"bufferedYSetters": [
			{"id": "QDAC1", "channel": 2}
		],
		"bufferedGetters": [
			{"id": "DMM1", "channel": 0},
			{"id": "LOCKIN1", "channel": 0}
		],
		"setXVoltageDomains": {
			"QDAC1:1": {"min": -0.8, "max": 0.2}
		},
		"setYVoltageDomains": {
			"QDAC1:2": {"min": -0.8, "max": 0.2}
		},
		"sampleRate": 50000,
		"numPoints": 50,
		"numXSteps": 101,
		"numYSteps": 101
	}`

	var req Measure2DBufferedRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Len(t, req.Setters, 2)
	assert.Len(t, req.BufferedXSetters, 1)
	assert.Len(t, req.BufferedYSetters, 1)
	assert.Len(t, req.BufferedGetters, 2)
	assert.Equal(t, 101, req.NumXSteps)
	assert.Equal(t, 101, req.NumYSteps)
	assert.Equal(t, float64(50000), req.SampleRate)
}

// =============================================================================
// Test: ADC/DAC Configuration
// =============================================================================

func TestFalconRequest_SetSampleRate(t *testing.T) {
	reqJSON := `{
		"getter": {"id": "ADC1", "channel": 0},
		"sampleRate": 100000
	}`

	var req SetSampleRateRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "ADC1", req.Getter.ID)
	assert.Equal(t, float64(100000), req.SampleRate)
}

func TestFalconRequest_GetSampleRate(t *testing.T) {
	reqJSON := `{
		"getter": {"id": "ADC1", "channel": 0}
	}`

	var req GetSampleRateRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "ADC1", req.Getter.ID)
}

func TestFalconRequest_SetNumberOfSamples(t *testing.T) {
	reqJSON := `{
		"getter": {"id": "ADC1", "channel": 0},
		"numberOfSamples": 1024
	}`

	var req SetNumberOfSamplesRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, float64(1024), req.NumberOfSamples)
}

func TestFalconRequest_GetNumberOfSamples(t *testing.T) {
	reqJSON := `{
		"getter": {"id": "ADC1", "channel": 0}
	}`

	var req GetNumberOfSamplesRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "ADC1", req.Getter.ID)
}

func TestFalconRequest_SetSlope(t *testing.T) {
	// Set DAC ramp slope (V/sec)
	reqJSON := `{
		"setter": {"id": "QDAC1", "channel": 1},
		"slope": 0.1
	}`

	var req SetSlopeRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "QDAC1", req.Setter.ID)
	assert.Equal(t, 0.1, req.Slope)
}

func TestFalconRequest_GetSlope(t *testing.T) {
	reqJSON := `{
		"getter": {"id": "QDAC1", "channel": 1}
	}`

	var req GetSlopeRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "QDAC1", req.Getter.ID)
}

func TestFalconRequest_SetTriggerLeader(t *testing.T) {
	reqJSON := `{
		"getter": {"id": "QDAC1", "channel": 1}
	}`

	var req SetTriggerLeaderRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "QDAC1", req.Getter.ID)
}

func TestFalconRequest_GetTriggerLeader(t *testing.T) {
	reqJSON := `{
		"getter": {"id": "QDAC1", "channel": 1}
	}`

	var req GetTriggerLeaderRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	assert.Equal(t, "QDAC1", req.Getter.ID)
}

// =============================================================================
// Test: Quantum Dot Scenarios with Falcon Request Formats
// =============================================================================

func TestFalconRequest_QuantumDotPinchoff(t *testing.T) {
	// Complete pinch-off measurement as falcon would send it
	reqJSON := `{
		"setters": [
			{"id": "QDAC1", "channel": 1},
			{"id": "QDAC1", "channel": 2},
			{"id": "QDAC1", "channel": 4},
			{"id": "QDAC1", "channel": 5}
		],
		"bufferedSetters": [
			{"id": "QDAC1", "channel": 3}
		],
		"bufferedGetters": [
			{"id": "DMM1", "channel": 0}
		],
		"setVoltageDomains": {
			"QDAC1:3": {"min": 0.0, "max": -2.0}
		},
		"sampleRate": 10000,
		"numPoints": 100,
		"numSteps": 201
	}`

	var req Measure1DBufferedRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	// Should sweep barrier B3 (channel 3) from 0 to -2V
	assert.Equal(t, 0.0, req.SetVoltageDomains["QDAC1:3"].Min)
	assert.Equal(t, -2.0, req.SetVoltageDomains["QDAC1:3"].Max)
	assert.Equal(t, 201, req.NumSteps)
}

func TestFalconRequest_ChargeStabilityDiagram(t *testing.T) {
	// 2D charge stability diagram scan
	reqJSON := `{
		"setters": [
			{"id": "QDAC1", "channel": 3},
			{"id": "QDAC1", "channel": 4},
			{"id": "QDAC1", "channel": 5}
		],
		"bufferedXSetters": [
			{"id": "QDAC1", "channel": 1}
		],
		"bufferedYSetters": [
			{"id": "QDAC1", "channel": 2}
		],
		"bufferedGetters": [
			{"id": "DMM1", "channel": 0}
		],
		"setXVoltageDomains": {
			"QDAC1:1": {"min": -0.8, "max": 0.2}
		},
		"setYVoltageDomains": {
			"QDAC1:2": {"min": -0.8, "max": 0.2}
		},
		"sampleRate": 50000,
		"numPoints": 50,
		"numXSteps": 201,
		"numYSteps": 201
	}`

	var req Measure2DBufferedRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	// Total measurement points
	totalPoints := req.NumXSteps * req.NumYSteps
	assert.Equal(t, 40401, totalPoints)
}

func TestFalconRequest_CoulombDiamond(t *testing.T) {
	// Coulomb diamond measurement with bias voltage
	// This would involve sweeping both gate and bias
	reqJSON := `{
		"setters": [
			{"id": "QDAC1", "channel": 3},
			{"id": "QDAC1", "channel": 4}
		],
		"bufferedXSetters": [
			{"id": "QDAC1", "channel": 1}
		],
		"bufferedYSetters": [
			{"id": "BIAS1", "channel": 0}
		],
		"bufferedGetters": [
			{"id": "DMM1", "channel": 0}
		],
		"setXVoltageDomains": {
			"QDAC1:1": {"min": -0.6, "max": -0.4}
		},
		"setYVoltageDomains": {
			"BIAS1:0": {"min": -5e-3, "max": 5e-3}
		},
		"sampleRate": 100000,
		"numPoints": 20,
		"numXSteps": 101,
		"numYSteps": 51
	}`

	var req Measure2DBufferedRequestJSON
	err := json.Unmarshal([]byte(reqJSON), &req)
	require.NoError(t, err)

	// Bias sweep: -5mV to +5mV
	assert.Equal(t, -5e-3, req.SetYVoltageDomains["BIAS1:0"].Min)
	assert.Equal(t, 5e-3, req.SetYVoltageDomains["BIAS1:0"].Max)
}

// =============================================================================
// Test: Integration with Device Config
// =============================================================================

func TestFalconRequest_ConvertToHubFormat(t *testing.T) {
	// Load device config relative to this test file (runtime/internal/serverinterpreter/)
	configPath := filepath.Join("..", "..", "..", "test_data", "dummy_one_charge_sensor_quantum_dot_device.yaml")
	config, err := LoadQuantumDotDeviceConfig(configPath)
	require.NoError(t, err)

	setup := NewQuantumDotMeasurementSetup(config, "QDAC1", "DMM1")

	t.Run("convert measure_get_set to hub format", func(t *testing.T) {
		reqJSON := `{
			"setters": [
				{"id": "QDAC1", "channel": 1},
				{"id": "QDAC1", "channel": 2}
			],
			"getters": [
				{"id": "DMM1", "channel": 0}
			],
			"setVoltages": {
				"QDAC1:1": -0.5,
				"QDAC1:2": -0.6
			}
		}`

		var req MeasureGetSetRequestJSON
		err := json.Unmarshal([]byte(reqJSON), &req)
		require.NoError(t, err)

		// Convert to hub set voltage requests
		hubSetReqs := make([]SetVoltageRequest, len(req.Setters))
		for i, setter := range req.Setters {
			key := setter.ID
			if setter.Channel != 0 {
				key = setter.ID + ":" + string(rune('0'+setter.Channel))
			}

			hubSetReqs[i] = SetVoltageRequest{
				Setter:     InstrumentTarget{Id: setter.ID, Channel: setter.Channel},
				SetVoltage: req.SetVoltages[key],
			}
		}
		assert.Len(t, hubSetReqs, 2)
	})

	t.Run("convert 1D buffered to averaged sweep", func(t *testing.T) {
		// Falcon sends 1D buffered, hub converts to averaged sweep
		reqJSON := `{
			"bufferedSetters": [{"id": "QDAC1", "channel": 1}],
			"bufferedGetters": [{"id": "DMM1", "channel": 0}],
			"setVoltageDomains": {
				"QDAC1:1": {"min": -1.0, "max": 0.0}
			},
			"sampleRate": 10000,
			"numPoints": 100,
			"numSteps": 101
		}`

		var req Measure1DBufferedRequestJSON
		err := json.Unmarshal([]byte(reqJSON), &req)
		require.NoError(t, err)

		// Extract sweep parameters
		sweepDomain := req.SetVoltageDomains["QDAC1:1"]
		sweepSetter := req.BufferedSetters[0]

		// Build averaged sweep data (add N averages)
		avgData := AveragedSweep1DScriptData{
			MeasurementName: "converted_1d_sweep",
			MeasurementID:   "test-conversion",
			SweepGate:       "P1",
			SweepSetter:     InstrumentTarget{Id: sweepSetter.ID, Channel: sweepSetter.Channel},
			StartVoltage:    sweepDomain.Min,
			StopVoltage:     sweepDomain.Max,
			NumPoints:       req.NumSteps,
			NumAverages:     10, // Hub can add averaging
			SettlingTimeMs:  1.0,
			GetVoltageRequests: []GetVoltageRequest{
				{Getter: InstrumentTarget{Id: "DMM1", Channel: 0}},
			},
		}

		assert.Equal(t, -1.0, avgData.StartVoltage)
		assert.Equal(t, 0.0, avgData.StopVoltage)
		assert.Equal(t, 101, avgData.NumPoints)
		assert.Equal(t, 10, avgData.NumAverages)

		// Verify we can pass through the device setup
		_ = setup
	})
}

// =============================================================================
// Test: Response Format
// =============================================================================

func TestFalconResponse_MeasurementResponseJSON(t *testing.T) {
	// Hub returns results in MeasurementResponse format
	response := FalconMeasurementResponseJSON{
		Instrument: "DMM1",
		Verb:       "GET_VOLTAGE",
		Type:       "number",
		Value:      1.23e-9,
	}

	jsonData, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed FalconMeasurementResponseJSON
	err = json.Unmarshal(jsonData, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "DMM1", parsed.Instrument)
	assert.Equal(t, 1.23e-9, parsed.Value)
}

func TestFalconResponse_ArrayOfResponses(t *testing.T) {
	// Multiple measurement results
	responses := []FalconMeasurementResponseJSON{
		{Instrument: "DMM1", Verb: "GET_VOLTAGE", Type: "number", Value: 1.23e-9},
		{Instrument: "LOCKIN1", Verb: "GET_VOLTAGE", Type: "number", Value: 4.56e-9},
	}

	jsonData, err := json.Marshal(responses)
	require.NoError(t, err)

	var parsed []FalconMeasurementResponseJSON
	err = json.Unmarshal(jsonData, &parsed)
	require.NoError(t, err)

	assert.Len(t, parsed, 2)
}
