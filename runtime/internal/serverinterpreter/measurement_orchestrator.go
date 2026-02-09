// Package serverinterpreter provides measurement orchestration for falcon requests.
//
// The hub does NOT auto-generate Lua scripts. Instead, experimenters create their own
// Lua measurement scripts that run on the instrument-script-server. The hub's role is to:
//
//  1. Parse incoming falcon measurement requests
//  2. Orchestrate complex measurements by calling simpler Lua scripts multiple times
//  3. Collect and aggregate results
//  4. Return structured responses to falcon
//
// For example, a 2D voltage sweep is orchestrated by:
//   - Calling a 1D sweep script for each Y-axis value
//   - Ramping gates between sweeps
//   - Aggregating all 1D traces into a 2D result
package serverinterpreter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ScriptExecutor defines the interface for executing Lua measurement scripts.
// This abstracts the actual instrument-script-server communication.
type ScriptExecutor interface {
	// ExecuteScript runs a named Lua script with the given parameters.
	// Returns the script's result as JSON and any error.
	ExecuteScript(ctx context.Context, scriptName string, params map[string]interface{}) ([]byte, error)
}

// MeasurementOrchestrator coordinates complex measurements by calling
// simpler Lua scripts on the instrument-script-server.
type MeasurementOrchestrator struct {
	executor   ScriptExecutor
	hubConfig  *HubConfig
	mu         sync.Mutex
	inProgress map[string]*OrchestratedMeasurement
}

// OrchestratedMeasurement tracks the state of a complex measurement.
type OrchestratedMeasurement struct {
	ID            string
	Type          string // e.g., "2d_sweep", "averaged_1d_sweep"
	Status        string // "pending", "running", "completed", "failed"
	StartTime     time.Time
	EndTime       time.Time
	Progress      float64 // 0.0 to 1.0
	Error         string
	PartialResult interface{}
}

// NewMeasurementOrchestrator creates a new orchestrator.
func NewMeasurementOrchestrator(executor ScriptExecutor, hubConfig *HubConfig) *MeasurementOrchestrator {
	return &MeasurementOrchestrator{
		executor:   executor,
		hubConfig:  hubConfig,
		inProgress: make(map[string]*OrchestratedMeasurement),
	}
}

// =============================================================================
// 2D Voltage Sweep Orchestration
// =============================================================================

// Sweep2DRequest defines a 2D voltage sweep measurement request.
// This maps from falcon's measure_2D_buffered schema.
type Sweep2DRequest struct {
	MeasurementID string `json:"measurementId"`

	// X-axis (fast axis) sweep configuration
	XGate        string  `json:"xGate"`        // Gate name for X sweep (e.g., "P1")
	XInstrument  string  `json:"xInstrument"`  // Instrument ID (e.g., "QDAC1")
	XChannel     int     `json:"xChannel"`     // Channel number
	XStartV      float64 `json:"xStartV"`      // X start voltage
	XStopV       float64 `json:"xStopV"`       // X stop voltage
	XNumPoints   int     `json:"xNumPoints"`   // Number of X points per line

	// Y-axis (slow axis) sweep configuration
	YGate        string  `json:"yGate"`        // Gate name for Y sweep (e.g., "P2")
	YInstrument  string  `json:"yInstrument"`  // Instrument ID
	YChannel     int     `json:"yChannel"`     // Channel number
	YStartV      float64 `json:"yStartV"`      // Y start voltage
	YStopV       float64 `json:"yStopV"`       // Y stop voltage
	YNumPoints   int     `json:"yNumPoints"`   // Number of Y lines

	// Measurement configuration
	CurrentMeter    string  `json:"currentMeter"`    // Instrument for current measurement
	CurrentChannel  int     `json:"currentChannel"`  // Channel for current
	SettlingTimeMs  float64 `json:"settlingTimeMs"`  // Settling time after voltage change
	RampSlopeVPerS  float64 `json:"rampSlopeVPerS"`  // Ramp rate for returning to start

	// Static gate voltages (held constant during sweep)
	StaticVoltages map[string]float64 `json:"staticVoltages"` // gate -> voltage
}

// Sweep2DResult contains the complete 2D sweep data.
type Sweep2DResult struct {
	MeasurementID string        `json:"measurementId"`
	XGate         string        `json:"xGate"`
	YGate         string        `json:"yGate"`
	XVoltages     []float64     `json:"xVoltages"`     // X-axis voltage values
	YVoltages     []float64     `json:"yVoltages"`     // Y-axis voltage values
	CurrentData   [][]float64   `json:"currentData"`   // [y][x] array of current values
	Lines         []Sweep1DLine `json:"lines"`         // Individual 1D sweep results
	StartTime     time.Time     `json:"startTime"`
	EndTime       time.Time     `json:"endTime"`
}

// Sweep1DLine represents one horizontal line in the 2D sweep.
type Sweep1DLine struct {
	YVoltage  float64   `json:"yVoltage"`
	YIndex    int       `json:"yIndex"`
	XVoltages []float64 `json:"xVoltages"`
	Currents  []float64 `json:"currents"`
	Timestamp time.Time `json:"timestamp"`
}

// Execute2DSweep orchestrates a 2D voltage sweep by calling 1D sweep scripts.
//
// Algorithm:
//  1. Set static gate voltages
//  2. For each Y value:
//     a. Set Y gate to the target voltage
//     b. Wait for settling
//     c. Execute 1D sweep script (X sweep)
//     d. Collect current vs X voltage data
//     e. Ramp X gate back to start voltage
//  3. Aggregate all lines into 2D result
func (o *MeasurementOrchestrator) Execute2DSweep(ctx context.Context, req Sweep2DRequest) (*Sweep2DResult, error) {
	// Register measurement
	measurement := &OrchestratedMeasurement{
		ID:        req.MeasurementID,
		Type:      "2d_sweep",
		Status:    "running",
		StartTime: time.Now(),
	}
	o.mu.Lock()
	o.inProgress[req.MeasurementID] = measurement
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		measurement.EndTime = time.Now()
		if measurement.Status == "running" {
			measurement.Status = "completed"
		}
		o.mu.Unlock()
	}()

	result := &Sweep2DResult{
		MeasurementID: req.MeasurementID,
		XGate:         req.XGate,
		YGate:         req.YGate,
		XVoltages:     make([]float64, req.XNumPoints),
		YVoltages:     make([]float64, req.YNumPoints),
		CurrentData:   make([][]float64, req.YNumPoints),
		Lines:         make([]Sweep1DLine, 0, req.YNumPoints),
		StartTime:     time.Now(),
	}

	// Calculate voltage arrays
	xStep := (req.XStopV - req.XStartV) / float64(req.XNumPoints-1)
	yStep := (req.YStopV - req.YStartV) / float64(req.YNumPoints-1)

	for i := 0; i < req.XNumPoints; i++ {
		result.XVoltages[i] = req.XStartV + float64(i)*xStep
	}
	for i := 0; i < req.YNumPoints; i++ {
		result.YVoltages[i] = req.YStartV + float64(i)*yStep
	}

	// Step 1: Set static gate voltages
	for gate, voltage := range req.StaticVoltages {
		params := map[string]interface{}{
			"gate":    gate,
			"voltage": voltage,
		}
		if _, err := o.executor.ExecuteScript(ctx, "set_voltage", params); err != nil {
			measurement.Status = "failed"
			measurement.Error = fmt.Sprintf("failed to set static voltage for %s: %v", gate, err)
			return nil, fmt.Errorf("failed to set static voltage for %s: %w", gate, err)
		}
	}

	// Step 2: Execute Y sweep (slow axis)
	for yIdx := 0; yIdx < req.YNumPoints; yIdx++ {
		yVoltage := result.YVoltages[yIdx]

		// Update progress
		o.mu.Lock()
		measurement.Progress = float64(yIdx) / float64(req.YNumPoints)
		o.mu.Unlock()

		// Check for cancellation
		select {
		case <-ctx.Done():
			measurement.Status = "cancelled"
			return nil, ctx.Err()
		default:
		}

		// 2a. Set Y gate voltage
		ySetParams := map[string]interface{}{
			"instrument": req.YInstrument,
			"channel":    req.YChannel,
			"voltage":    yVoltage,
		}
		if _, err := o.executor.ExecuteScript(ctx, "set_voltage", ySetParams); err != nil {
			measurement.Status = "failed"
			measurement.Error = fmt.Sprintf("failed to set Y voltage at index %d: %v", yIdx, err)
			return nil, fmt.Errorf("failed to set Y voltage: %w", err)
		}

		// 2b. Wait for settling
		if req.SettlingTimeMs > 0 {
			time.Sleep(time.Duration(req.SettlingTimeMs) * time.Millisecond)
		}

		// 2c. Execute 1D sweep (X axis)
		sweep1DParams := map[string]interface{}{
			"sweepInstrument": req.XInstrument,
			"sweepChannel":    req.XChannel,
			"startVoltage":    req.XStartV,
			"stopVoltage":     req.XStopV,
			"numPoints":       req.XNumPoints,
			"settlingTimeMs":  req.SettlingTimeMs,
			"currentMeter":    req.CurrentMeter,
			"currentChannel":  req.CurrentChannel,
		}

		lineData, err := o.executor.ExecuteScript(ctx, "sweep_1d", sweep1DParams)
		if err != nil {
			measurement.Status = "failed"
			measurement.Error = fmt.Sprintf("failed 1D sweep at Y index %d: %v", yIdx, err)
			return nil, fmt.Errorf("failed 1D sweep at Y=%f: %w", yVoltage, err)
		}

		// 2d. Parse 1D sweep result
		currents, err := parseSweep1DResult(lineData)
		if err != nil {
			measurement.Status = "failed"
			measurement.Error = fmt.Sprintf("failed to parse 1D sweep result: %v", err)
			return nil, fmt.Errorf("failed to parse 1D sweep result: %w", err)
		}

		line := Sweep1DLine{
			YVoltage:  yVoltage,
			YIndex:    yIdx,
			XVoltages: result.XVoltages,
			Currents:  currents,
			Timestamp: time.Now(),
		}
		result.Lines = append(result.Lines, line)
		result.CurrentData[yIdx] = currents

		// 2e. Ramp X gate back to start
		rampParams := map[string]interface{}{
			"instrument":   req.XInstrument,
			"channel":      req.XChannel,
			"targetV":      req.XStartV,
			"slopeVPerSec": req.RampSlopeVPerS,
		}
		if _, err := o.executor.ExecuteScript(ctx, "ramp_voltage", rampParams); err != nil {
			// Log warning but continue - ramp failure is not fatal
			fmt.Printf("Warning: ramp back failed at Y index %d: %v\n", yIdx, err)
		}
	}

	result.EndTime = time.Now()
	measurement.Progress = 1.0
	measurement.PartialResult = result

	return result, nil
}

// GetMeasurementStatus returns the status of an in-progress measurement.
func (o *MeasurementOrchestrator) GetMeasurementStatus(measurementID string) (*OrchestratedMeasurement, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	m, ok := o.inProgress[measurementID]
	return m, ok
}

// =============================================================================
// Averaged 1D Sweep Orchestration
// =============================================================================

// AveragedSweep1DRequest defines an N-averaged 1D sweep.
type AveragedSweep1DRequest struct {
	MeasurementID string `json:"measurementId"`

	// Sweep configuration
	SweepGate       string  `json:"sweepGate"`
	SweepInstrument string  `json:"sweepInstrument"`
	SweepChannel    int     `json:"sweepChannel"`
	StartV          float64 `json:"startV"`
	StopV           float64 `json:"stopV"`
	NumPoints       int     `json:"numPoints"`
	NumAverages     int     `json:"numAverages"`

	// Measurement
	CurrentMeter   string  `json:"currentMeter"`
	CurrentChannel int     `json:"currentChannel"`
	SettlingTimeMs float64 `json:"settlingTimeMs"`

	// Static voltages
	StaticVoltages map[string]float64 `json:"staticVoltages"`
}

// AveragedSweep1DResult contains the averaged sweep data.
type AveragedSweep1DResult struct {
	MeasurementID    string      `json:"measurementId"`
	SweepGate        string      `json:"sweepGate"`
	Voltages         []float64   `json:"voltages"`
	AveragedCurrents []float64   `json:"averagedCurrents"`
	AllTraces        [][]float64 `json:"allTraces"` // [sweep_idx][point]
	StdDev           []float64   `json:"stdDev"`    // Standard deviation at each point
	NumAverages      int         `json:"numAverages"`
}

// ExecuteAveraged1DSweep performs N 1D sweeps and averages the results.
func (o *MeasurementOrchestrator) ExecuteAveraged1DSweep(ctx context.Context, req AveragedSweep1DRequest) (*AveragedSweep1DResult, error) {
	result := &AveragedSweep1DResult{
		MeasurementID: req.MeasurementID,
		SweepGate:     req.SweepGate,
		Voltages:      make([]float64, req.NumPoints),
		AllTraces:     make([][]float64, 0, req.NumAverages),
		NumAverages:   req.NumAverages,
	}

	// Calculate voltage array
	step := (req.StopV - req.StartV) / float64(req.NumPoints-1)
	for i := 0; i < req.NumPoints; i++ {
		result.Voltages[i] = req.StartV + float64(i)*step
	}

	// Set static voltages
	for gate, voltage := range req.StaticVoltages {
		params := map[string]interface{}{
			"gate":    gate,
			"voltage": voltage,
		}
		if _, err := o.executor.ExecuteScript(ctx, "set_voltage", params); err != nil {
			return nil, fmt.Errorf("failed to set static voltage for %s: %w", gate, err)
		}
	}

	// Perform N sweeps
	for sweepIdx := 0; sweepIdx < req.NumAverages; sweepIdx++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sweep1DParams := map[string]interface{}{
			"sweepInstrument": req.SweepInstrument,
			"sweepChannel":    req.SweepChannel,
			"startVoltage":    req.StartV,
			"stopVoltage":     req.StopV,
			"numPoints":       req.NumPoints,
			"settlingTimeMs":  req.SettlingTimeMs,
			"currentMeter":    req.CurrentMeter,
			"currentChannel":  req.CurrentChannel,
		}

		traceData, err := o.executor.ExecuteScript(ctx, "sweep_1d", sweep1DParams)
		if err != nil {
			return nil, fmt.Errorf("sweep %d failed: %w", sweepIdx+1, err)
		}

		currents, err := parseSweep1DResult(traceData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse sweep %d: %w", sweepIdx+1, err)
		}

		result.AllTraces = append(result.AllTraces, currents)
	}

	// Compute averages and standard deviation
	result.AveragedCurrents = make([]float64, req.NumPoints)
	result.StdDev = make([]float64, req.NumPoints)

	for i := 0; i < req.NumPoints; i++ {
		sum := 0.0
		for _, trace := range result.AllTraces {
			sum += trace[i]
		}
		mean := sum / float64(req.NumAverages)
		result.AveragedCurrents[i] = mean

		// Standard deviation
		sumSqDiff := 0.0
		for _, trace := range result.AllTraces {
			diff := trace[i] - mean
			sumSqDiff += diff * diff
		}
		result.StdDev[i] = sqrt(sumSqDiff / float64(req.NumAverages))
	}

	return result, nil
}

// sqrt computes square root (inline to avoid math import for this simple case)
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// =============================================================================
// Helper Functions
// =============================================================================

// parseSweep1DResult extracts current values from a 1D sweep script result.
func parseSweep1DResult(data []byte) ([]float64, error) {
	// Parse the JSON result from sweep_1d script
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sweep result: %w", err)
	}

	// Extract the sweep array
	sweepData, ok := result["sweep"]
	if !ok {
		// Fallback: check for "results" key
		sweepData, ok = result["results"]
		if !ok {
			return nil, fmt.Errorf("sweep result missing 'sweep' or 'results' field")
		}
	}

	sweepArray, ok := sweepData.([]interface{})
	if !ok {
		return nil, fmt.Errorf("sweep data is not an array")
	}

	currents := make([]float64, len(sweepArray))
	for i, point := range sweepArray {
		pointMap, ok := point.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("sweep point %d is not a map", i)
		}

		// Try to get current value
		if current, ok := pointMap["current"].(float64); ok {
			currents[i] = current
		} else if value, ok := pointMap["value"].(float64); ok {
			currents[i] = value
		} else {
			// Default to zero if not found
			currents[i] = 0
		}
	}

	return currents, nil
}
