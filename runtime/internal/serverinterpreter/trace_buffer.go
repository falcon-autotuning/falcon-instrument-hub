// Package serverinterpreter provides trace buffering and averaging for measurements.
//
// This file implements the TraceBuffer which accumulates measurement traces
// from multiple sweeps, computes averages, and stores results to HDF5.
package serverinterpreter

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// TracePoint represents a single measurement point in a trace.
type TracePoint struct {
	Voltage      float64            `json:"voltage"`
	Measurements map[string]float64 `json:"measurements"`
}

// Trace represents a complete voltage sweep trace.
type Trace struct {
	SweepIndex int          `json:"sweep_index"`
	Points     []TracePoint `json:"points"`
	Timestamp  time.Time    `json:"timestamp"`
}

// AveragedTrace represents the result of averaging multiple traces.
type AveragedTrace struct {
	Points     []TracePoint `json:"points"`
	NumSweeps  int          `json:"num_sweeps"`
	SweepGate  string       `json:"sweep_gate"`
	StartV     float64      `json:"start_v"`
	StopV      float64      `json:"stop_v"`
	Timestamps []time.Time  `json:"timestamps"`
}

// TraceReportMessage is the format for trace reports from Lua scripts.
// This matches the ctx:report_trace() call in the averaged sweep template.
type TraceReportMessage struct {
	MeasurementID string                   `json:"measurement_id"`
	SweepIndex    int                      `json:"sweep_index"`
	TotalSweeps   int                      `json:"total_sweeps"`
	Trace         []map[string]interface{} `json:"trace"`
}

// PendingAveragedMeasurement tracks an in-progress averaged measurement.
type PendingAveragedMeasurement struct {
	MeasurementID string
	SweepGate     string
	StartVoltage  float64
	StopVoltage   float64
	NumPoints     int
	ExpectedCount int
	ReceivedCount int
	Traces        []Trace
	StartTime     time.Time
	Channels      []string // Measurement channel names
	mu            sync.Mutex
}

// TraceBufferConfig configures the trace buffer.
type TraceBufferConfig struct {
	// MaxPendingMeasurements limits memory usage
	MaxPendingMeasurements int

	// TraceTimeout is how long to wait for all traces
	TraceTimeout time.Duration

	// DatabasePath is where to store HDF5 files
	DatabasePath string

	// OnMeasurementComplete is called when averaging is done
	OnMeasurementComplete func(*AveragedMeasurementResult) error

	// OnLog is called for logging
	OnLog func(string)
}

// DefaultTraceBufferConfig returns reasonable defaults.
func DefaultTraceBufferConfig() TraceBufferConfig {
	return TraceBufferConfig{
		MaxPendingMeasurements: 100,
		TraceTimeout:           5 * time.Minute,
		DatabasePath:           "/tmp/falcon-data",
	}
}

// TraceBuffer accumulates traces for averaged measurements.
type TraceBuffer struct {
	pending map[string]*PendingAveragedMeasurement
	mu      sync.RWMutex
	config  TraceBufferConfig
}

// NewTraceBuffer creates a new trace buffer.
func NewTraceBuffer(config TraceBufferConfig) *TraceBuffer {
	return &TraceBuffer{
		pending: make(map[string]*PendingAveragedMeasurement),
		config:  config,
	}
}

// RegisterMeasurement registers a new averaged measurement to collect traces for.
func (tb *TraceBuffer) RegisterMeasurement(
	measurementID string,
	sweepGate string,
	startV, stopV float64,
	numPoints int,
	expectedSweeps int,
	channels []string,
) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if _, exists := tb.pending[measurementID]; exists {
		return fmt.Errorf("measurement %s already registered", measurementID)
	}

	if len(tb.pending) >= tb.config.MaxPendingMeasurements {
		return fmt.Errorf("max pending measurements reached: %d", tb.config.MaxPendingMeasurements)
	}

	tb.pending[measurementID] = &PendingAveragedMeasurement{
		MeasurementID: measurementID,
		SweepGate:     sweepGate,
		StartVoltage:  startV,
		StopVoltage:   stopV,
		NumPoints:     numPoints,
		ExpectedCount: expectedSweeps,
		ReceivedCount: 0,
		Traces:        make([]Trace, 0, expectedSweeps),
		StartTime:     time.Now(),
		Channels:      channels,
	}

	tb.log(fmt.Sprintf("Registered measurement %s: expecting %d sweeps", measurementID, expectedSweeps))
	return nil
}

// AddTrace adds a trace to a pending measurement.
// Returns (isComplete, error).
func (tb *TraceBuffer) AddTrace(report *TraceReportMessage) (bool, error) {
	tb.mu.RLock()
	pm, exists := tb.pending[report.MeasurementID]
	tb.mu.RUnlock()

	if !exists {
		return false, fmt.Errorf("unknown measurement: %s", report.MeasurementID)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Parse trace data
	trace := Trace{
		SweepIndex: report.SweepIndex,
		Points:     make([]TracePoint, len(report.Trace)),
		Timestamp:  time.Now(),
	}

	for i, pt := range report.Trace {
		trace.Points[i] = TracePoint{
			Measurements: make(map[string]float64),
		}

		if v, ok := pt["voltage"].(float64); ok {
			trace.Points[i].Voltage = v
		}

		if measurements, ok := pt["measurements"].(map[string]interface{}); ok {
			for ch, val := range measurements {
				if f, ok := val.(float64); ok {
					trace.Points[i].Measurements[ch] = f
				}
			}
		}
	}

	pm.Traces = append(pm.Traces, trace)
	pm.ReceivedCount++

	tb.log(fmt.Sprintf("Received trace %d/%d for measurement %s",
		pm.ReceivedCount, pm.ExpectedCount, report.MeasurementID))

	return pm.ReceivedCount >= pm.ExpectedCount, nil
}

// AveragedMeasurementResult contains the completed measurement data.
type AveragedMeasurementResult struct {
	MeasurementID  string         `json:"measurement_id"`
	SweepGate      string         `json:"sweep_gate"`
	StartVoltage   float64        `json:"start_voltage"`
	StopVoltage    float64        `json:"stop_voltage"`
	NumPoints      int            `json:"num_points"`
	NumSweeps      int            `json:"num_sweeps"`
	AllTraces      []Trace        `json:"all_traces"`
	AveragedTrace  AveragedTrace  `json:"averaged_trace"`
	TotalDuration  time.Duration  `json:"total_duration"`
	DatabasePath   string         `json:"database_path,omitempty"`
}

// Complete computes the average and returns the result.
func (tb *TraceBuffer) Complete(measurementID string) (*AveragedMeasurementResult, error) {
	tb.mu.Lock()
	pm, exists := tb.pending[measurementID]
	if exists {
		delete(tb.pending, measurementID)
	}
	tb.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("unknown measurement: %s", measurementID)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.ReceivedCount < pm.ExpectedCount {
		return nil, fmt.Errorf("incomplete measurement: received %d of %d traces",
			pm.ReceivedCount, pm.ExpectedCount)
	}

	// Compute averaged trace
	averaged := tb.computeAverage(pm)

	result := &AveragedMeasurementResult{
		MeasurementID:  pm.MeasurementID,
		SweepGate:      pm.SweepGate,
		StartVoltage:   pm.StartVoltage,
		StopVoltage:    pm.StopVoltage,
		NumPoints:      pm.NumPoints,
		NumSweeps:      len(pm.Traces),
		AllTraces:      pm.Traces,
		AveragedTrace:  averaged,
		TotalDuration:  time.Since(pm.StartTime),
	}

	tb.log(fmt.Sprintf("Completed measurement %s: %d sweeps averaged in %v",
		measurementID, result.NumSweeps, result.TotalDuration))

	// Call completion callback if set
	if tb.config.OnMeasurementComplete != nil {
		if err := tb.config.OnMeasurementComplete(result); err != nil {
			tb.log(fmt.Sprintf("Warning: completion callback failed: %v", err))
		}
	}

	return result, nil
}

// computeAverage computes the point-by-point average across all traces.
func (tb *TraceBuffer) computeAverage(pm *PendingAveragedMeasurement) AveragedTrace {
	if len(pm.Traces) == 0 {
		return AveragedTrace{}
	}

	numPoints := len(pm.Traces[0].Points)
	numSweeps := len(pm.Traces)

	averaged := AveragedTrace{
		Points:     make([]TracePoint, numPoints),
		NumSweeps:  numSweeps,
		SweepGate:  pm.SweepGate,
		StartV:     pm.StartVoltage,
		StopV:      pm.StopVoltage,
		Timestamps: make([]time.Time, numSweeps),
	}

	// Collect timestamps
	for i, tr := range pm.Traces {
		averaged.Timestamps[i] = tr.Timestamp
	}

	// Compute averages for each point
	for i := 0; i < numPoints; i++ {
		averaged.Points[i] = TracePoint{
			Voltage:      pm.Traces[0].Points[i].Voltage,
			Measurements: make(map[string]float64),
		}

		// Sum and average each measurement channel
		channelSums := make(map[string]float64)
		for _, trace := range pm.Traces {
			if i < len(trace.Points) {
				for ch, val := range trace.Points[i].Measurements {
					channelSums[ch] += val
				}
			}
		}

		for ch, sum := range channelSums {
			averaged.Points[i].Measurements[ch] = sum / float64(numSweeps)
		}
	}

	return averaged
}

// GetPending returns the number of pending measurements.
func (tb *TraceBuffer) GetPending() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return len(tb.pending)
}

// GetStatus returns status for a pending measurement.
func (tb *TraceBuffer) GetStatus(measurementID string) (received, expected int, exists bool) {
	tb.mu.RLock()
	pm, exists := tb.pending[measurementID]
	tb.mu.RUnlock()

	if !exists {
		return 0, 0, false
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.ReceivedCount, pm.ExpectedCount, true
}

// CleanupExpired removes measurements that have timed out.
func (tb *TraceBuffer) CleanupExpired() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	cleaned := 0
	for id, pm := range tb.pending {
		if time.Since(pm.StartTime) > tb.config.TraceTimeout {
			delete(tb.pending, id)
			cleaned++
			tb.log(fmt.Sprintf("Cleaned up expired measurement: %s", id))
		}
	}
	return cleaned
}

func (tb *TraceBuffer) log(msg string) {
	if tb.config.OnLog != nil {
		tb.config.OnLog(msg)
	}
}

// ToJSON serializes the result to JSON.
func (r *AveragedMeasurementResult) ToJSON() (string, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ExtractCurrentTrace extracts the current values from the averaged trace.
// Returns voltage array and current array for the specified channel.
func (r *AveragedMeasurementResult) ExtractCurrentTrace(channel string) ([]float64, []float64, error) {
	voltages := make([]float64, len(r.AveragedTrace.Points))
	currents := make([]float64, len(r.AveragedTrace.Points))

	for i, pt := range r.AveragedTrace.Points {
		voltages[i] = pt.Voltage
		if val, ok := pt.Measurements[channel]; ok {
			currents[i] = val
		} else {
			return nil, nil, fmt.Errorf("channel %s not found at point %d", channel, i)
		}
	}

	return voltages, currents, nil
}
