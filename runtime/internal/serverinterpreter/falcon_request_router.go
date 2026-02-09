// Package serverinterpreter provides falcon measurement request parsing and routing.
package serverinterpreter

import (
	"context"
	"encoding/json"
	"fmt"
)

// =============================================================================
// Types imported from falcon-measurement-lib
// These should eventually be imported directly from the falcon-measurement-lib module
// =============================================================================

// FalconDomain represents a voltage range (min, max).
type FalconDomain struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// FalconInstrumentTarget represents an instrument reference with optional channel.
type FalconInstrumentTarget struct {
	ID      string `json:"id"`
	Channel int    `json:"channel,omitempty"`
}

// Serialize returns the string representation "id" or "id:channel".
func (t FalconInstrumentTarget) Serialize() string {
	if t.Channel != 0 {
		return fmt.Sprintf("%s:%d", t.ID, t.Channel)
	}
	return t.ID
}

// =============================================================================
// Falcon Request Types (from falcon-measurement-lib schemas)
// =============================================================================

// FalconMeasure1DBufferedRequest matches measure_1D_buffered.json schema.
type FalconMeasure1DBufferedRequest struct {
	Setters           []FalconInstrumentTarget   `json:"setters,omitempty"`
	BufferedSetters   []FalconInstrumentTarget   `json:"bufferedSetters,omitempty"`
	BufferedGetters   []FalconInstrumentTarget   `json:"bufferedGetters"`
	SetVoltageDomains map[string]FalconDomain    `json:"setVoltageDomains,omitempty"`
	SampleRate        float64                    `json:"sampleRate"`
	NumPoints         int                        `json:"numPoints"`
	NumSteps          int                        `json:"numSteps"`
}

// FalconMeasure2DBufferedRequest matches measure_2D_buffered.json schema.
type FalconMeasure2DBufferedRequest struct {
	Setters            []FalconInstrumentTarget `json:"setters,omitempty"`
	BufferedXSetters   []FalconInstrumentTarget `json:"bufferedXSetters,omitempty"`
	BufferedYSetters   []FalconInstrumentTarget `json:"bufferedYSetters,omitempty"`
	BufferedGetters    []FalconInstrumentTarget `json:"bufferedGetters"`
	SetXVoltageDomains map[string]FalconDomain  `json:"setXVoltageDomains,omitempty"`
	SetYVoltageDomains map[string]FalconDomain  `json:"setYVoltageDomains,omitempty"`
	SampleRate         float64                  `json:"sampleRate"`
	NumPoints          int                      `json:"numPoints"`
	NumXSteps          int                      `json:"numXSteps"`
	NumYSteps          int                      `json:"numYSteps"`
}

// FalconMeasureGetSetRequest matches measure_get_set.json schema.
type FalconMeasureGetSetRequest struct {
	Setters     []FalconInstrumentTarget `json:"setters"`
	Getters     []FalconInstrumentTarget `json:"getters"`
	SetVoltages map[string]float64       `json:"setVoltages"`
	SampleRate  float64                  `json:"sampleRate,omitempty"`
	NumPoints   int                      `json:"numPoints,omitempty"`
}

// =============================================================================
// Measurement Request Envelope
// =============================================================================

// FalconMeasurementEnvelope wraps a falcon measurement request with metadata.
type FalconMeasurementEnvelope struct {
	MeasurementID   string          `json:"measurementId"`
	MeasurementType string          `json:"measurementType"` // e.g., "measure_2D_buffered"
	Request         json.RawMessage `json:"request"`         // The actual request payload
	Timestamp       int64           `json:"timestamp"`
	UnitHash        string          `json:"unitHash,omitempty"`
}

// =============================================================================
// Request Router
// =============================================================================

// MeasurementRouter routes falcon measurement requests to the appropriate
// orchestrator method based on measurement type.
type MeasurementRouter struct {
	orchestrator *MeasurementOrchestrator
}

// NewMeasurementRouter creates a new router.
func NewMeasurementRouter(orchestrator *MeasurementOrchestrator) *MeasurementRouter {
	return &MeasurementRouter{
		orchestrator: orchestrator,
	}
}

// RouteResult contains the result of routing a measurement request.
type RouteResult struct {
	Success       bool            `json:"success"`
	MeasurementID string          `json:"measurementId"`
	ResultType    string          `json:"resultType"`
	Result        json.RawMessage `json:"result,omitempty"`
	Error         string          `json:"error,omitempty"`
}

// Route parses a falcon measurement envelope and executes the appropriate measurement.
func (r *MeasurementRouter) Route(ctx context.Context, envelope FalconMeasurementEnvelope) (*RouteResult, error) {
	result := &RouteResult{
		MeasurementID: envelope.MeasurementID,
	}

	switch envelope.MeasurementType {
	case "measure_2D_buffered":
		return r.route2DBuffered(ctx, envelope)

	case "measure_1D_buffered":
		return r.route1DBuffered(ctx, envelope)

	case "measure_get_set":
		return r.routeGetSet(ctx, envelope)

	default:
		result.Success = false
		result.Error = fmt.Sprintf("unknown measurement type: %s", envelope.MeasurementType)
		return result, fmt.Errorf("unknown measurement type: %s", envelope.MeasurementType)
	}
}

// route2DBuffered handles 2D buffered sweep requests.
func (r *MeasurementRouter) route2DBuffered(ctx context.Context, envelope FalconMeasurementEnvelope) (*RouteResult, error) {
	result := &RouteResult{
		MeasurementID: envelope.MeasurementID,
		ResultType:    "Sweep2DResult",
	}

	// Parse the falcon request
	var falconReq FalconMeasure2DBufferedRequest
	if err := json.Unmarshal(envelope.Request, &falconReq); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to parse 2D buffered request: %v", err)
		return result, err
	}

	// Convert to orchestrator request
	orchReq, err := convert2DBufferedToSweep2D(envelope.MeasurementID, falconReq)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to convert request: %v", err)
		return result, err
	}

	// Execute the 2D sweep
	sweepResult, err := r.orchestrator.Execute2DSweep(ctx, orchReq)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result, err
	}

	// Serialize result
	resultJSON, err := json.Marshal(sweepResult)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to serialize result: %v", err)
		return result, err
	}

	result.Success = true
	result.Result = resultJSON
	return result, nil
}

// route1DBuffered handles 1D buffered sweep requests.
func (r *MeasurementRouter) route1DBuffered(ctx context.Context, envelope FalconMeasurementEnvelope) (*RouteResult, error) {
	result := &RouteResult{
		MeasurementID: envelope.MeasurementID,
		ResultType:    "AveragedSweep1DResult",
	}

	var falconReq FalconMeasure1DBufferedRequest
	if err := json.Unmarshal(envelope.Request, &falconReq); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to parse 1D buffered request: %v", err)
		return result, err
	}

	// Convert to orchestrator request (default to 1 average for simple 1D)
	orchReq, err := convert1DBufferedToAveraged1D(envelope.MeasurementID, falconReq, 1)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to convert request: %v", err)
		return result, err
	}

	sweepResult, err := r.orchestrator.ExecuteAveraged1DSweep(ctx, orchReq)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result, err
	}

	resultJSON, err := json.Marshal(sweepResult)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to serialize result: %v", err)
		return result, err
	}

	result.Success = true
	result.Result = resultJSON
	return result, nil
}

// routeGetSet handles DC get/set measurement requests.
// For simple get/set, we dispatch directly to the dc_get_set script.
func (r *MeasurementRouter) routeGetSet(ctx context.Context, envelope FalconMeasurementEnvelope) (*RouteResult, error) {
	result := &RouteResult{
		MeasurementID: envelope.MeasurementID,
		ResultType:    "MeasureGetSetResult",
	}

	var falconReq FalconMeasureGetSetRequest
	if err := json.Unmarshal(envelope.Request, &falconReq); err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to parse get/set request: %v", err)
		return result, err
	}

	// For simple DC get/set, call the script directly
	params := map[string]interface{}{
		"setters":     falconReq.Setters,
		"getters":     falconReq.Getters,
		"setVoltages": falconReq.SetVoltages,
		"sampleRate":  falconReq.SampleRate,
	}

	scriptResult, err := r.orchestrator.executor.ExecuteScript(ctx, "dc_get_set", params)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		return result, err
	}

	result.Success = true
	result.Result = scriptResult
	return result, nil
}

// =============================================================================
// Request Conversion Functions
// =============================================================================

// convert2DBufferedToSweep2D converts a falcon 2D buffered request to our
// orchestrator's Sweep2DRequest format.
func convert2DBufferedToSweep2D(measurementID string, req FalconMeasure2DBufferedRequest) (Sweep2DRequest, error) {
	result := Sweep2DRequest{
		MeasurementID: measurementID,
		XNumPoints:    req.NumXSteps,
		YNumPoints:    req.NumYSteps,
	}

	// Extract X-axis configuration from bufferedXSetters
	if len(req.BufferedXSetters) == 0 {
		return result, fmt.Errorf("bufferedXSetters must have at least one entry")
	}
	xSetter := req.BufferedXSetters[0]
	result.XInstrument = xSetter.ID
	result.XChannel = xSetter.Channel
	result.XGate = xSetter.Serialize()

	// Get X voltage domain
	xDomainKey := xSetter.Serialize()
	if xDomain, ok := req.SetXVoltageDomains[xDomainKey]; ok {
		result.XStartV = xDomain.Min
		result.XStopV = xDomain.Max
	} else {
		return result, fmt.Errorf("no X voltage domain found for %s", xDomainKey)
	}

	// Extract Y-axis configuration from bufferedYSetters
	if len(req.BufferedYSetters) == 0 {
		return result, fmt.Errorf("bufferedYSetters must have at least one entry")
	}
	ySetter := req.BufferedYSetters[0]
	result.YInstrument = ySetter.ID
	result.YChannel = ySetter.Channel
	result.YGate = ySetter.Serialize()

	// Get Y voltage domain
	yDomainKey := ySetter.Serialize()
	if yDomain, ok := req.SetYVoltageDomains[yDomainKey]; ok {
		result.YStartV = yDomain.Min
		result.YStopV = yDomain.Max
	} else {
		return result, fmt.Errorf("no Y voltage domain found for %s", yDomainKey)
	}

	// Extract current meter from bufferedGetters
	if len(req.BufferedGetters) == 0 {
		return result, fmt.Errorf("bufferedGetters must have at least one entry")
	}
	getter := req.BufferedGetters[0]
	result.CurrentMeter = getter.ID
	result.CurrentChannel = getter.Channel

	// Default settling time (can be made configurable)
	result.SettlingTimeMs = 1.0
	result.RampSlopeVPerS = 0.1

	// Static voltages from setters (not yet implemented - would need setVoltages map)
	result.StaticVoltages = make(map[string]float64)

	return result, nil
}

// convert1DBufferedToAveraged1D converts a falcon 1D buffered request to
// our AveragedSweep1DRequest format.
func convert1DBufferedToAveraged1D(measurementID string, req FalconMeasure1DBufferedRequest, numAverages int) (AveragedSweep1DRequest, error) {
	result := AveragedSweep1DRequest{
		MeasurementID: measurementID,
		NumPoints:     req.NumSteps,
		NumAverages:   numAverages,
	}

	// Extract sweep configuration from bufferedSetters
	if len(req.BufferedSetters) == 0 {
		return result, fmt.Errorf("bufferedSetters must have at least one entry")
	}
	sweepSetter := req.BufferedSetters[0]
	result.SweepInstrument = sweepSetter.ID
	result.SweepChannel = sweepSetter.Channel
	result.SweepGate = sweepSetter.Serialize()

	// Get voltage domain
	domainKey := sweepSetter.Serialize()
	if domain, ok := req.SetVoltageDomains[domainKey]; ok {
		result.StartV = domain.Min
		result.StopV = domain.Max
	} else {
		return result, fmt.Errorf("no voltage domain found for %s", domainKey)
	}

	// Extract current meter from bufferedGetters
	if len(req.BufferedGetters) == 0 {
		return result, fmt.Errorf("bufferedGetters must have at least one entry")
	}
	getter := req.BufferedGetters[0]
	result.CurrentMeter = getter.ID
	result.CurrentChannel = getter.Channel

	// Default settling time
	result.SettlingTimeMs = 1.0

	// Static voltages (not yet implemented)
	result.StaticVoltages = make(map[string]float64)

	return result, nil
}
