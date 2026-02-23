// Package serverinterpreter provides integration tests for 2D sweep
// orchestration and the associated schema validation.
package serverinterpreter

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock executor for 2D sweep tests
// =============================================================================

// mockScriptExecutor records calls and returns canned responses.
type mockScriptExecutor struct {
	calls []scriptCall
	// sweepResult is returned for any "sweep_1d" invocation.
	sweepResult []byte
	failAfter   int // return error after N calls (-1 = never)
}

type scriptCall struct {
	Name   string
	Params map[string]interface{}
}

func newMockExecutor(numXPoints int) *mockScriptExecutor {
	// Build a canned 1D sweep result matching parseSweep1DResult expectations.
	type sweepPoint struct {
		Voltage float64 `json:"voltage"`
		Current float64 `json:"current"`
	}
	points := make([]sweepPoint, numXPoints)
	for i := 0; i < numXPoints; i++ {
		points[i] = sweepPoint{
			Voltage: float64(i) / float64(numXPoints-1),
			Current: float64(i) * 1e-9,
		}
	}
	data, _ := json.Marshal(map[string]interface{}{
		"sweep": points,
	})
	return &mockScriptExecutor{
		sweepResult: data,
		failAfter:   -1,
	}
}

func (m *mockScriptExecutor) ExecuteScript(_ context.Context, scriptName string, params map[string]interface{}) ([]byte, error) {
	m.calls = append(m.calls, scriptCall{Name: scriptName, Params: params})

	if m.failAfter >= 0 && len(m.calls) > m.failAfter {
		return nil, fmt.Errorf("mock failure after %d calls", m.failAfter)
	}

	switch scriptName {
	case "sweep_1d":
		return m.sweepResult, nil
	case "set_voltage", "ramp_voltage":
		return []byte(`{"ok": true}`), nil
	default:
		return []byte(`{}`), nil
	}
}

// =============================================================================
// 2D Sweep Orchestration Tests
// =============================================================================

func TestExecute2DSweep_Basic(t *testing.T) {
	xPts := 11
	yPts := 5
	mock := newMockExecutor(xPts)

	orch := NewMeasurementOrchestrator(mock, &HubConfig{})

	req := Sweep2DRequest{
		MeasurementID:  "2d-test-basic",
		XGate:          "P1",
		XInstrument:    "QDAC1",
		XChannel:       1,
		XStartV:        -1.0,
		XStopV:         0.0,
		XNumPoints:     xPts,
		YGate:          "P2",
		YInstrument:    "QDAC1",
		YChannel:       2,
		YStartV:        -0.8,
		YStopV:         0.2,
		YNumPoints:     yPts,
		CurrentMeter:   "DMM1",
		CurrentChannel: 0,
		SettlingTimeMs: 0, // no waiting in tests
		StaticVoltages: map[string]float64{"B1": -1.0},
	}

	result, err := orch.Execute2DSweep(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, "2d-test-basic", result.MeasurementID)
	assert.Equal(t, "P1", result.XGate)
	assert.Equal(t, "P2", result.YGate)
	assert.Len(t, result.XVoltages, xPts)
	assert.Len(t, result.YVoltages, yPts)

	// Verify the mock received calls:
	// 1 set_voltage for B1 + (yPts * (1 set_voltage Y + 1 sweep_1d))
	setVCalls := 0
	sweepCalls := 0
	for _, c := range mock.calls {
		switch c.Name {
		case "set_voltage":
			setVCalls++
		case "sweep_1d":
			sweepCalls++
		}
	}
	assert.Equal(t, yPts, sweepCalls, "one sweep_1d per Y step")
	assert.GreaterOrEqual(t, setVCalls, 1, "at least static + Y set_voltages")
}

func TestExecute2DSweep_CancelledContext(t *testing.T) {
	mock := newMockExecutor(10)
	orch := NewMeasurementOrchestrator(mock, &HubConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := Sweep2DRequest{
		MeasurementID: "2d-cancel",
		XNumPoints:    10,
		YNumPoints:    5,
		XStartV:       -1,
		XStopV:        0,
		YStartV:       -1,
		YStopV:        0,
	}

	_, err := orch.Execute2DSweep(ctx, req)
	assert.Error(t, err, "should fail with cancelled context")
}

// =============================================================================
// Schema Validation Tests
// =============================================================================

func TestValidate1DBuffered_Valid(t *testing.T) {
	req := FalconMeasure1DBufferedRequest{
		BufferedSetters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 1}},
		BufferedGetters: []FalconInstrumentTarget{{ID: "DMM1", Channel: 0}},
		SetVoltageDomains: map[string]FalconDomain{
			"QDAC1:1": {Min: -1.0, Max: 0.0},
		},
		SampleRate: 10000,
		NumPoints:  100,
		NumSteps:   101,
	}

	result := Validate1DBufferedRequest(req)
	assert.True(t, result.OK(), "expected no errors, got: %s", result.Error())
}

func TestValidate1DBuffered_MissingGetters(t *testing.T) {
	req := FalconMeasure1DBufferedRequest{
		BufferedSetters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 1}},
		SetVoltageDomains: map[string]FalconDomain{
			"QDAC1:1": {Min: -1.0, Max: 0.0},
		},
		SampleRate: 10000,
		NumPoints:  100,
		NumSteps:   101,
	}

	result := Validate1DBufferedRequest(req)
	assert.False(t, result.OK())
	assert.Contains(t, result.Error(), "bufferedGetters")
}

func TestValidate1DBuffered_DomainMinGeMax(t *testing.T) {
	req := FalconMeasure1DBufferedRequest{
		BufferedSetters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 1}},
		BufferedGetters: []FalconInstrumentTarget{{ID: "DMM1", Channel: 0}},
		SetVoltageDomains: map[string]FalconDomain{
			"QDAC1:1": {Min: 0.0, Max: -1.0}, // inverted!
		},
		SampleRate: 10000,
		NumPoints:  100,
		NumSteps:   101,
	}

	result := Validate1DBufferedRequest(req)
	assert.False(t, result.OK())
	assert.Contains(t, result.Error(), "domain min")
}

func TestValidate1DBuffered_MissingSampleRate(t *testing.T) {
	req := FalconMeasure1DBufferedRequest{
		BufferedSetters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 1}},
		BufferedGetters: []FalconInstrumentTarget{{ID: "DMM1", Channel: 0}},
		SetVoltageDomains: map[string]FalconDomain{
			"QDAC1:1": {Min: -1.0, Max: 0.0},
		},
		SampleRate: 0,
		NumPoints:  100,
		NumSteps:   101,
	}

	result := Validate1DBufferedRequest(req)
	assert.False(t, result.OK())
	assert.Contains(t, result.Error(), "sampleRate")
}

func TestValidate2DBuffered_Valid(t *testing.T) {
	req := FalconMeasure2DBufferedRequest{
		BufferedXSetters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 1}},
		BufferedYSetters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 2}},
		BufferedGetters:  []FalconInstrumentTarget{{ID: "DMM1", Channel: 0}},
		SetXVoltageDomains: map[string]FalconDomain{
			"QDAC1:1": {Min: -0.8, Max: 0.2},
		},
		SetYVoltageDomains: map[string]FalconDomain{
			"QDAC1:2": {Min: -0.8, Max: 0.2},
		},
		SampleRate: 50000,
		NumPoints:  50,
		NumXSteps:  101,
		NumYSteps:  101,
	}

	result := Validate2DBufferedRequest(req)
	assert.True(t, result.OK(), "expected no errors, got: %s", result.Error())
}

func TestValidate2DBuffered_MissingYDomain(t *testing.T) {
	req := FalconMeasure2DBufferedRequest{
		BufferedXSetters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 1}},
		BufferedYSetters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 2}},
		BufferedGetters:  []FalconInstrumentTarget{{ID: "DMM1", Channel: 0}},
		SetXVoltageDomains: map[string]FalconDomain{
			"QDAC1:1": {Min: -0.8, Max: 0.2},
		},
		// Missing Y domain
		SampleRate: 50000,
		NumPoints:  50,
		NumXSteps:  101,
		NumYSteps:  101,
	}

	result := Validate2DBufferedRequest(req)
	assert.False(t, result.OK())
	assert.Contains(t, result.Error(), "setYVoltageDomains")
}

func TestValidateGetSet_Valid(t *testing.T) {
	req := FalconMeasureGetSetRequest{
		Setters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 1}},
		Getters: []FalconInstrumentTarget{{ID: "DMM1", Channel: 0}},
		SetVoltages: map[string]float64{
			"QDAC1:1": -0.5,
		},
	}

	result := ValidateGetSetRequest(req)
	assert.True(t, result.OK(), "expected no errors, got: %s", result.Error())
}

func TestValidateGetSet_MissingVoltage(t *testing.T) {
	req := FalconMeasureGetSetRequest{
		Setters: []FalconInstrumentTarget{{ID: "QDAC1", Channel: 1}},
		Getters: []FalconInstrumentTarget{{ID: "DMM1", Channel: 0}},
		// Missing setVoltages
	}

	result := ValidateGetSetRequest(req)
	assert.False(t, result.OK())
	assert.Contains(t, result.Error(), "setVoltages")
}

func TestValidateEnvelope_Valid(t *testing.T) {
	env := FalconMeasurementEnvelope{
		MeasurementID:   "test-1",
		MeasurementType: "measure_1D_buffered",
		Request: json.RawMessage(`{
			"bufferedSetters": [{"id": "QDAC1", "channel": 1}],
			"bufferedGetters": [{"id": "DMM1", "channel": 0}],
			"setVoltageDomains": {"QDAC1:1": {"min": -1.0, "max": 0.0}},
			"sampleRate": 10000,
			"numPoints": 100,
			"numSteps": 101
		}`),
	}

	result := ValidateEnvelope(env)
	assert.True(t, result.OK(), "expected no errors, got: %s", result.Error())
}

func TestValidateEnvelope_UnknownType(t *testing.T) {
	env := FalconMeasurementEnvelope{
		MeasurementID:   "test-bad",
		MeasurementType: "measure_magic",
		Request:         json.RawMessage(`{}`),
	}

	result := ValidateEnvelope(env)
	assert.False(t, result.OK())
	assert.Contains(t, result.Error(), "unknown type")
}

func TestValidateRequest_FullPipeline_1D(t *testing.T) {
	env := FalconMeasurementEnvelope{
		MeasurementID:   "pipeline-1d",
		MeasurementType: "measure_1D_buffered",
		Request: json.RawMessage(`{
			"bufferedSetters": [{"id": "QDAC1", "channel": 1}],
			"bufferedGetters": [{"id": "DMM1", "channel": 0}],
			"setVoltageDomains": {"QDAC1:1": {"min": -1.0, "max": 0.0}},
			"sampleRate": 10000,
			"numPoints": 100,
			"numSteps": 101
		}`),
	}

	result := ValidateRequest(env)
	assert.True(t, result.OK(), "expected no errors, got: %s", result.Error())
}

func TestValidateRequest_FullPipeline_2D(t *testing.T) {
	env := FalconMeasurementEnvelope{
		MeasurementID:   "pipeline-2d",
		MeasurementType: "measure_2D_buffered",
		Request: json.RawMessage(`{
			"bufferedXSetters": [{"id": "QDAC1", "channel": 1}],
			"bufferedYSetters": [{"id": "QDAC1", "channel": 2}],
			"bufferedGetters": [{"id": "DMM1", "channel": 0}],
			"setXVoltageDomains": {"QDAC1:1": {"min": -0.8, "max": 0.2}},
			"setYVoltageDomains": {"QDAC1:2": {"min": -0.8, "max": 0.2}},
			"sampleRate": 50000,
			"numPoints": 50,
			"numXSteps": 101,
			"numYSteps": 101
		}`),
	}

	result := ValidateRequest(env)
	assert.True(t, result.OK(), "expected no errors, got: %s", result.Error())
}

func TestValidateRequest_FullPipeline_GetSet(t *testing.T) {
	env := FalconMeasurementEnvelope{
		MeasurementID:   "pipeline-gs",
		MeasurementType: "measure_get_set",
		Request: json.RawMessage(`{
			"setters": [{"id": "QDAC1", "channel": 1}],
			"getters": [{"id": "DMM1", "channel": 0}],
			"setVoltages": {"QDAC1:1": -0.5}
		}`),
	}

	result := ValidateRequest(env)
	assert.True(t, result.OK(), "expected no errors, got: %s", result.Error())
}

func TestValidateRequest_RejectsInvalidPayload(t *testing.T) {
	env := FalconMeasurementEnvelope{
		MeasurementID:   "pipeline-bad",
		MeasurementType: "measure_1D_buffered",
		Request: json.RawMessage(`{
			"bufferedGetters": [],
			"sampleRate": -5,
			"numPoints": 0,
			"numSteps": 1
		}`),
	}

	result := ValidateRequest(env)
	assert.False(t, result.OK())
	// Should catch multiple issues
	assert.GreaterOrEqual(t, len(result.Errors), 3)
}

// =============================================================================
// Database Writer (HDF5 fallback → JSON) Tests
// =============================================================================

func TestHDF5Writer_FallsBackToJSON(t *testing.T) {
	// Without the hdf5 build tag, the writer should fall back to JSON.
	// If the hdf5 tag is present, the writer should use native HDF5.
	tempDir := t.TempDir()

	writer, err := NewHDF5Writer(HDF5Config{
		BasePath:   tempDir,
		FilePrefix: "test",
	})
	require.NoError(t, err)

	result := &AveragedMeasurementResult{
		MeasurementID: "hdf5-test",
		SweepGate:     "P1",
		StartVoltage:  -1.0,
		StopVoltage:   0.0,
		NumPoints:     3,
		NumSweeps:     2,
		AveragedTrace: AveragedTrace{
			Points: []TracePoint{
				{Voltage: -1.0, Measurements: map[string]float64{"DMM1_0": 1e-9}},
				{Voltage: -0.5, Measurements: map[string]float64{"DMM1_0": 2e-9}},
				{Voltage: 0.0, Measurements: map[string]float64{"DMM1_0": 3e-9}},
			},
		},
	}

	path, err := writer.WriteAveragedMeasurement(result)
	require.NoError(t, err)
	assert.NotEmpty(t, path)
	assert.FileExists(t, path)
}
