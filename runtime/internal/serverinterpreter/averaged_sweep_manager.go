// Package serverinterpreter provides the AveragedSweepManager for handling
// N-averaged 1D sweep measurements from falcon.
//
// This manager coordinates:
// - Parsing falcon measurement requests for averaged sweeps
// - Generating Lua scripts for the instrument-script-server
// - Buffering individual sweep traces as they return
// - Computing averages when all traces are collected
// - Storing results to the HDF5 database
// - Returning averaged data to falcon via NATS/JetStream
package serverinterpreter

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// AveragedSweepRequest represents a request from falcon for an N-averaged sweep.
type AveragedSweepRequest struct {
	// MeasurementName is the human-readable name
	MeasurementName string `json:"measurement_name"`

	// SweepGate is the gate to sweep (e.g., "P1")
	SweepGate string `json:"sweep_gate"`

	// StartVoltage is the sweep start voltage
	StartVoltage float64 `json:"start_voltage"`

	// StopVoltage is the sweep end voltage
	StopVoltage float64 `json:"stop_voltage"`

	// NumPoints is the number of points per sweep
	NumPoints int `json:"num_points"`

	// NumAverages is the number of sweeps to average
	NumAverages int `json:"num_averages"`

	// SettlingTimeMs is the settling time after each voltage set
	SettlingTimeMs float64 `json:"settling_time_ms"`

	// StaticVoltages are the static gate voltages during the sweep
	StaticVoltages map[string]float64 `json:"static_voltages"`

	// MeasurementChannels specifies which channels to measure
	MeasurementChannels []string `json:"measurement_channels,omitempty"`

	// ProcessID is the falcon process ID for response routing
	ProcessID int64 `json:"process_id,omitempty"`
}

// AveragedSweepManagerConfig configures the manager.
type AveragedSweepManagerConfig struct {
	// HubConfig is the loaded hub configuration
	HubConfig *HubConfig

	// ScriptOutputDir is where generated Lua scripts are stored
	ScriptOutputDir string

	// NATSUrl is the NATS server URL
	NATSUrl string

	// Debug enables verbose logging
	Debug bool
}

// AveragedSweepManager handles N-averaged sweep measurements.
type AveragedSweepManager struct {
	config      AveragedSweepManagerConfig
	hubConfig   *HubConfig
	deviceSetup *QuantumDotMeasurementSetup
	traceBuffer *TraceBuffer
	database    *MeasurementDatabase
	client      *ScriptServerClient

	// NATS connection for trace reports
	nc *nats.Conn
	js nats.JetStreamContext

	// Active measurements
	activeMeasurements map[string]*activeMeasurement
	mu                 sync.RWMutex

	// Control
	ctx    context.Context
	cancel context.CancelFunc

	debug bool
}

// activeMeasurement tracks an in-progress averaged sweep.
type activeMeasurement struct {
	Request       *AveragedSweepRequest
	MeasurementID string
	StartTime     time.Time
	ResultChan    chan *AveragedMeasurementResult
	ErrorChan     chan error
}

// NewAveragedSweepManager creates a new manager.
func NewAveragedSweepManager(config AveragedSweepManagerConfig) (*AveragedSweepManager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create database
	dbPath := config.ScriptOutputDir
	if config.HubConfig != nil && config.HubConfig.LocalDatabase != "" {
		dbPath = config.HubConfig.ResolvePath(config.HubConfig.LocalDatabase)
	}
	database, err := NewMeasurementDatabase(dbPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	mgr := &AveragedSweepManager{
		config:             config,
		hubConfig:          config.HubConfig,
		database:           database,
		activeMeasurements: make(map[string]*activeMeasurement),
		ctx:                ctx,
		cancel:             cancel,
		debug:              config.Debug,
	}

	// Create trace buffer with completion callback
	bufferConfig := DefaultTraceBufferConfig()
	bufferConfig.DatabasePath = dbPath
	bufferConfig.OnMeasurementComplete = mgr.handleMeasurementComplete
	bufferConfig.OnLog = func(msg string) {
		mgr.log(msg)
	}
	mgr.traceBuffer = NewTraceBuffer(bufferConfig)

	// Create script server client
	port := 5555
	if config.HubConfig != nil {
		port = config.HubConfig.GetInstrumentServerPort()
	}
	mgr.client = NewScriptServerClient("127.0.0.1", port)

	return mgr, nil
}

// SetDeviceSetup sets the quantum dot device setup.
func (m *AveragedSweepManager) SetDeviceSetup(setup *QuantumDotMeasurementSetup) {
	m.deviceSetup = setup
}

// LoadDeviceSetup loads the device setup from the hub config.
func (m *AveragedSweepManager) LoadDeviceSetup(dacID, dmmID string) error {
	if m.hubConfig == nil {
		return fmt.Errorf("hub config not loaded")
	}

	deviceConfig, err := m.hubConfig.GetQuantumDotDeviceConfig()
	if err != nil {
		return fmt.Errorf("failed to load device config: %w", err)
	}

	m.deviceSetup = NewQuantumDotMeasurementSetup(deviceConfig, dacID, dmmID)
	return nil
}

// Start connects to NATS and begins listening for trace reports.
func (m *AveragedSweepManager) Start() error {
	natsUrl := "nats://localhost:4222"
	if m.hubConfig != nil {
		natsUrl = m.hubConfig.GetNATSUrl()
	}

	var err error
	m.nc, err = nats.Connect(natsUrl)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Subscribe to trace reports from instrument-script-server
	_, err = m.nc.Subscribe("instrument.trace_report", m.handleTraceReport)
	if err != nil {
		return fmt.Errorf("failed to subscribe to trace reports: %w", err)
	}

	m.log("AveragedSweepManager started")
	return nil
}

// Stop shuts down the manager.
func (m *AveragedSweepManager) Stop() error {
	m.cancel()

	if m.nc != nil {
		m.nc.Drain()
	}

	return nil
}

// ExecuteAveragedSweep executes an N-averaged sweep measurement.
// This is the main entry point for falcon requests.
//
// Note: This method uses user-provided Lua scripts via the script server.
// The old auto-generation path has been removed. Use MeasurementOrchestrator
// for the recommended script-dispatch approach.
func (m *AveragedSweepManager) ExecuteAveragedSweep(req *AveragedSweepRequest) (*AveragedMeasurementResult, error) {
	if m.deviceSetup == nil {
		return nil, fmt.Errorf("device setup not configured")
	}

	// Generate unique measurement ID
	measurementID := uuid.New().String()

	m.log(fmt.Sprintf("Starting averaged sweep %s: %s, %d points, %d averages",
		measurementID, req.SweepGate, req.NumPoints, req.NumAverages))

	// Build sweep data from device setup
	sweepData, err := m.deviceSetup.Build1DSweepData(
		req.SweepGate,
		req.StartVoltage,
		req.StopVoltage,
		req.NumPoints,
		req.StaticVoltages,
		req.SettlingTimeMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build sweep data: %w", err)
	}

	// Get measurement channel names for registration
	channels := make([]string, len(sweepData.GetVoltageRequests))
	for i, gvr := range sweepData.GetVoltageRequests {
		channels[i] = fmt.Sprintf("%s_%d", gvr.Getter.Id, gvr.Getter.Channel)
	}

	// Register with trace buffer
	err = m.traceBuffer.RegisterMeasurement(
		measurementID,
		req.SweepGate,
		req.StartVoltage,
		req.StopVoltage,
		req.NumPoints,
		req.NumAverages,
		channels,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register measurement: %w", err)
	}

	// Track active measurement
	active := &activeMeasurement{
		Request:       req,
		MeasurementID: measurementID,
		StartTime:     time.Now(),
		ResultChan:    make(chan *AveragedMeasurementResult, 1),
		ErrorChan:     make(chan error, 1),
	}

	m.mu.Lock()
	m.activeMeasurements[measurementID] = active
	m.mu.Unlock()

	// Submit the user-provided sweep script to instrument-script-server
	jobID, err := m.client.SubmitMeasure("sweep_1d")
	if err != nil {
		m.mu.Lock()
		delete(m.activeMeasurements, measurementID)
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to submit script: %w", err)
	}

	m.log(fmt.Sprintf("Submitted job %s for measurement %s", jobID, measurementID))

	// Wait for completion (with timeout)
	timeout := time.Duration(req.NumAverages*req.NumPoints) * time.Second
	if timeout < time.Minute {
		timeout = time.Minute
	}

	select {
	case result := <-active.ResultChan:
		return result, nil
	case err := <-active.ErrorChan:
		return nil, err
	case <-time.After(timeout):
		m.mu.Lock()
		delete(m.activeMeasurements, measurementID)
		m.mu.Unlock()
		return nil, fmt.Errorf("measurement timed out after %v", timeout)
	case <-m.ctx.Done():
		return nil, fmt.Errorf("manager stopped")
	}
}

// handleTraceReport handles incoming trace reports from instrument-script-server.
func (m *AveragedSweepManager) handleTraceReport(msg *nats.Msg) {
	var report TraceReportMessage
	if err := json.Unmarshal(msg.Data, &report); err != nil {
		m.log(fmt.Sprintf("Failed to parse trace report: %v", err))
		return
	}

	m.log(fmt.Sprintf("Received trace %d/%d for %s",
		report.SweepIndex, report.TotalSweeps, report.MeasurementID))

	// Add to buffer
	complete, err := m.traceBuffer.AddTrace(&report)
	if err != nil {
		m.log(fmt.Sprintf("Failed to add trace: %v", err))
		return
	}

	// If all traces received, complete the measurement
	if complete {
		result, err := m.traceBuffer.Complete(report.MeasurementID)
		if err != nil {
			m.log(fmt.Sprintf("Failed to complete measurement: %v", err))
			m.notifyError(report.MeasurementID, err)
			return
		}

		// Store to database
		dbPath, err := m.database.Store(result)
		if err != nil {
			m.log(fmt.Sprintf("Failed to store result: %v", err))
		} else {
			result.DatabasePath = dbPath
			m.log(fmt.Sprintf("Stored result to: %s", dbPath))
		}

		// Notify waiting goroutine
		m.notifyResult(report.MeasurementID, result)
	}
}

// handleMeasurementComplete is called by the trace buffer when averaging is done.
func (m *AveragedSweepManager) handleMeasurementComplete(result *AveragedMeasurementResult) error {
	// Publish result to JetStream for falcon access
	if m.js != nil {
		dataJSON, err := result.ToJSON()
		if err != nil {
			return fmt.Errorf("failed to serialize result: %w", err)
		}

		subject := fmt.Sprintf("measurement.result.%s", result.MeasurementID)
		_, err = m.js.Publish(subject, []byte(dataJSON))
		if err != nil {
			return fmt.Errorf("failed to publish result: %w", err)
		}

		m.log(fmt.Sprintf("Published result to JetStream: %s", subject))
	}

	return nil
}

// notifyResult notifies the waiting goroutine of completion.
func (m *AveragedSweepManager) notifyResult(measurementID string, result *AveragedMeasurementResult) {
	m.mu.Lock()
	active, exists := m.activeMeasurements[measurementID]
	if exists {
		delete(m.activeMeasurements, measurementID)
	}
	m.mu.Unlock()

	if exists && active.ResultChan != nil {
		active.ResultChan <- result
	}
}

// notifyError notifies the waiting goroutine of an error.
func (m *AveragedSweepManager) notifyError(measurementID string, err error) {
	m.mu.Lock()
	active, exists := m.activeMeasurements[measurementID]
	if exists {
		delete(m.activeMeasurements, measurementID)
	}
	m.mu.Unlock()

	if exists && active.ErrorChan != nil {
		active.ErrorChan <- err
	}
}

// GetMeasurementStatus returns status for an active measurement.
func (m *AveragedSweepManager) GetMeasurementStatus(measurementID string) (received, expected int, exists bool) {
	return m.traceBuffer.GetStatus(measurementID)
}

// LoadMeasurement loads a completed measurement from the database.
func (m *AveragedSweepManager) LoadMeasurement(measurementID string) (*AveragedMeasurementResult, error) {
	return m.database.Load(measurementID)
}

// ListMeasurements lists all stored measurements.
func (m *AveragedSweepManager) ListMeasurements() []MeasurementIndex {
	return m.database.List()
}

func (m *AveragedSweepManager) log(msg string) {
	if m.debug {
		fmt.Printf("[AveragedSweepManager] %s\n", msg)
	}
}

// ParseAveragedSweepRequestJSON parses a request from JSON.
func ParseAveragedSweepRequestJSON(jsonStr string) (*AveragedSweepRequest, error) {
	var req AveragedSweepRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		return nil, fmt.Errorf("failed to parse averaged sweep request: %w", err)
	}

	// Validate
	if req.SweepGate == "" {
		return nil, fmt.Errorf("sweep_gate is required")
	}
	if req.NumPoints <= 0 {
		return nil, fmt.Errorf("num_points must be positive")
	}
	if req.NumAverages <= 0 {
		req.NumAverages = 1 // Default to single sweep
	}

	return &req, nil
}
