// Package serverinterpreter provides database writing for measurement results.
//
// Two-Database Architecture:
//
//	RawTraceDB   – stores every individual sweep trace (hub-local only)
//	AveragedDB   – stores the averaged result + a RawDataRef link (shared with falcon)
//
// Only the averaged database is exposed to falcon via JetStream.
// Raw traces are kept locally for post-hoc analysis, noise diagnostics,
// or re-averaging with different filters.
package serverinterpreter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// =============================================================================
// Raw / Averaged Linking
// =============================================================================

// RawDataRef links an averaged measurement back to its raw trace data.
// This is stored inside the averaged record so falcon can reference raw data
// without receiving it.
type RawDataRef struct {
	MeasurementID string `json:"measurement_id"`
	RawFilePath   string `json:"raw_file_path"`
	NumTraces     int    `json:"num_traces"`
	NumPoints     int    `json:"num_points_per_trace"`
}

// RawTraceRecord stores all individual sweep traces for a single measurement.
// This is the on-disk format in the raw database.
type RawTraceRecord struct {
	MeasurementID string        `json:"measurement_id"`
	SweepGate     string        `json:"sweep_gate"`
	StartVoltage  float64       `json:"start_voltage"`
	StopVoltage   float64       `json:"stop_voltage"`
	NumTraces     int           `json:"num_traces"`
	NumPoints     int           `json:"num_points"`
	Traces        []Trace       `json:"traces"`
	Channels      []string      `json:"channels"`
	RecordedAt    time.Time     `json:"recorded_at"`
	TotalDuration time.Duration `json:"total_duration"`
}

// =============================================================================
// Raw Trace Database (hub-local, NOT shared with falcon)
// =============================================================================

// RawTraceDatabase stores individual sweep traces.
// This data stays on the hub and is never shared via JetStream.
type RawTraceDatabase struct {
	basePath string
	index    map[string]RawTraceIndex
}

// RawTraceIndex tracks raw trace files.
type RawTraceIndex struct {
	MeasurementID string    `json:"measurement_id"`
	FilePath      string    `json:"file_path"`
	NumTraces     int       `json:"num_traces"`
	NumPoints     int       `json:"num_points"`
	StoredAt      time.Time `json:"stored_at"`
}

// NewRawTraceDatabase creates a new raw trace database.
func NewRawTraceDatabase(basePath string) (*RawTraceDatabase, error) {
	rawPath := filepath.Join(basePath, "raw")
	if err := os.MkdirAll(rawPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create raw database directory: %w", err)
	}

	db := &RawTraceDatabase{
		basePath: rawPath,
		index:    make(map[string]RawTraceIndex),
	}

	if err := db.loadIndex(); err != nil {
		// Index doesn't exist yet, that's OK
	}

	return db, nil
}

// Store stores raw traces and returns a RawDataRef for linking.
func (db *RawTraceDatabase) Store(record *RawTraceRecord) (*RawDataRef, error) {
	filename := fmt.Sprintf("raw_%s.json", record.MeasurementID)
	fp := filepath.Join(db.basePath, filename)

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal raw traces: %w", err)
	}

	if err := os.WriteFile(fp, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write raw trace file: %w", err)
	}

	// Update index
	db.index[record.MeasurementID] = RawTraceIndex{
		MeasurementID: record.MeasurementID,
		FilePath:      fp,
		NumTraces:     record.NumTraces,
		NumPoints:     record.NumPoints,
		StoredAt:      time.Now(),
	}

	if err := db.saveIndex(); err != nil {
		return nil, fmt.Errorf("stored raw data but failed to update index: %w", err)
	}

	return &RawDataRef{
		MeasurementID: record.MeasurementID,
		RawFilePath:   fp,
		NumTraces:     record.NumTraces,
		NumPoints:     record.NumPoints,
	}, nil
}

// Load loads raw traces by measurement ID.
func (db *RawTraceDatabase) Load(measurementID string) (*RawTraceRecord, error) {
	idx, exists := db.index[measurementID]
	if !exists {
		return nil, fmt.Errorf("raw traces not found: %s", measurementID)
	}

	data, err := os.ReadFile(idx.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read raw trace file: %w", err)
	}

	var record RawTraceRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to parse raw trace file: %w", err)
	}

	return &record, nil
}

// List returns all stored raw trace IDs.
func (db *RawTraceDatabase) List() []RawTraceIndex {
	result := make([]RawTraceIndex, 0, len(db.index))
	for _, idx := range db.index {
		result = append(result, idx)
	}
	return result
}

// GetFilePath returns the file path for a raw trace record.
func (db *RawTraceDatabase) GetFilePath(measurementID string) (string, error) {
	idx, exists := db.index[measurementID]
	if !exists {
		return "", fmt.Errorf("raw traces not found: %s", measurementID)
	}
	return idx.FilePath, nil
}

func (db *RawTraceDatabase) loadIndex() error {
	indexPath := filepath.Join(db.basePath, "raw_index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &db.index)
}

func (db *RawTraceDatabase) saveIndex() error {
	indexPath := filepath.Join(db.basePath, "raw_index.json")
	data, err := json.MarshalIndent(db.index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0644)
}

// =============================================================================
// Averaged Database (shared with falcon via JetStream)
// =============================================================================

// DatabaseWriter is the interface for storing measurement results.
type DatabaseWriter interface {
	// WriteAveragedMeasurement stores an averaged measurement result.
	WriteAveragedMeasurement(result *AveragedMeasurementResult) (string, error)

	// Close closes the database writer.
	Close() error
}

// HDF5Config configures the HDF5 database writer.
type HDF5Config struct {
	// BasePath is the base directory for HDF5 files
	BasePath string

	// FilePrefix is the prefix for generated filenames
	FilePrefix string

	// WriteRawTraces determines if individual traces are stored
	WriteRawTraces bool

	// Compression level (0-9, 0 = none)
	Compression int
}

// DefaultHDF5Config returns reasonable defaults.
func DefaultHDF5Config() HDF5Config {
	return HDF5Config{
		BasePath:       "/tmp/falcon-data",
		FilePrefix:     "measurement",
		WriteRawTraces: true,
		Compression:    4,
	}
}

// HDF5Writer writes measurement data to HDF5 format.
// This is a wrapper that can use either native HDF5 or JSON fallback.
type HDF5Writer struct {
	config  HDF5Config
	useJSON bool // Fallback to JSON if HDF5 library not available
}

// NewHDF5Writer creates a new HDF5 writer.
// If the native HDF5 library is available (built with -tags hdf5), HDF5 is
// preferred.  Otherwise the writer automatically falls back to JSON.
func NewHDF5Writer(config HDF5Config) (*HDF5Writer, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(config.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// hdf5Available is set to true by hdf5_impl.go (build tag: hdf5).
	// The stub (hdf5_stub.go) leaves it false.
	writer := &HDF5Writer{
		config:  config,
		useJSON: !hdf5Available,
	}

	return writer, nil
}

// WriteAveragedMeasurement stores the measurement result.
func (w *HDF5Writer) WriteAveragedMeasurement(result *AveragedMeasurementResult) (string, error) {
	if w.useJSON {
		return w.writeJSON(result)
	}
	return w.writeHDF5(result)
}

// writeJSON writes the result as a JSON file (fallback).
func (w *HDF5Writer) writeJSON(result *AveragedMeasurementResult) (string, error) {
	filename := fmt.Sprintf("%s_%s.json",
		w.config.FilePrefix,
		result.MeasurementID)
	fp := filepath.Join(w.config.BasePath, filename)

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := os.WriteFile(fp, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	result.DatabasePath = fp
	return fp, nil
}

// writeHDF5 writes the result to HDF5 format using gonum/hdf5.
//
// Dataset layout inside the HDF5 file:
//
//	/<measurementID>/
//	    averaged_trace/          float64 [numPoints x numChannels]
//	    voltages/                float64 [numPoints]
//	    metadata (attrs):
//	        measurement_id       string
//	        sweep_gate           string
//	        start_voltage        float64
//	        stop_voltage         float64
//	        num_points           int
//	        num_sweeps           int
//	        total_duration_ns    int64
//
// If the HDF5 library (libhdf5) is not installed on the host, the writer
// automatically falls back to JSON via writeJSON.
func (w *HDF5Writer) writeHDF5(result *AveragedMeasurementResult) (string, error) {
	filename := fmt.Sprintf("%s_%s.h5",
		w.config.FilePrefix,
		result.MeasurementID)
	fp := filepath.Join(w.config.BasePath, filename)

	// Try native HDF5 first; fall back to JSON if the C library is missing.
	if err := writeHDF5Native(fp, result); err != nil {
		// HDF5 library might not be available — fall back to JSON and log.
		fmt.Printf("HDF5 write failed (%v), falling back to JSON\n", err)
		return w.writeJSON(result)
	}

	result.DatabasePath = fp
	return fp, nil
}

// writeHDF5Native performs the actual HDF5 I/O.
// It is extracted so that the fallback path is always available even when
// the hdf5 C library is not linked.
func writeHDF5Native(fp string, result *AveragedMeasurementResult) error {
	// ---------------------------------------------------------------
	// HDF5 C bindings are optional.  When they are not available the
	// build uses hdf5_stub.go which makes this function return an error
	// immediately, causing the caller to fall back to JSON.
	// ---------------------------------------------------------------
	return writeHDF5Impl(fp, result)
}

// Close closes the writer.
func (w *HDF5Writer) Close() error {
	return nil
}

// MeasurementDatabase manages both raw and averaged measurement databases.
//
// Layout on disk:
//
//	<basePath>/
//	  averaged/         <- averaged results (shared with falcon)
//	    index.json
//	    sweep_<id>.json
//	  raw/              <- individual traces (hub-local only)
//	    raw_index.json
//	    raw_<id>.json
type MeasurementDatabase struct {
	basePath string
	rawDB    *RawTraceDatabase
	index    map[string]MeasurementIndex
}

// MeasurementIndex tracks stored measurements.
type MeasurementIndex struct {
	MeasurementID   string      `json:"measurement_id"`
	FilePath        string      `json:"file_path"`
	SweepGate       string      `json:"sweep_gate"`
	NumPoints       int         `json:"num_points"`
	NumSweeps       int         `json:"num_sweeps"`
	StoredAt        time.Time   `json:"stored_at"`
	RawDataRef      *RawDataRef `json:"raw_data_ref,omitempty"`
	MeasurementType string      `json:"measurement_type,omitempty"` // "" or "1d" = legacy 1D, "2d" = 2D sweep
}

// NewMeasurementDatabase creates a new measurement database with raw/averaged split.
func NewMeasurementDatabase(basePath string) (*MeasurementDatabase, error) {
	avgPath := filepath.Join(basePath, "averaged")
	if err := os.MkdirAll(avgPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create averaged database directory: %w", err)
	}

	rawDB, err := NewRawTraceDatabase(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw database: %w", err)
	}

	db := &MeasurementDatabase{
		basePath: basePath,
		rawDB:    rawDB,
		index:    make(map[string]MeasurementIndex),
	}

	// Try to load existing index
	if err := db.loadIndex(); err != nil {
		// Index doesn't exist yet, that's OK
	}

	return db, nil
}

// Store stores a measurement result with split raw/averaged storage.
// Raw traces go to the raw database, averaged results to the averaged database.
// Returns the averaged file path (for JetStream notification).
func (db *MeasurementDatabase) Store(result *AveragedMeasurementResult) (string, error) {
	var rawRef *RawDataRef

	// 1. Store raw traces if present
	if len(result.AllTraces) > 0 {
		rawRecord := &RawTraceRecord{
			MeasurementID: result.MeasurementID,
			SweepGate:     result.SweepGate,
			StartVoltage:  result.StartVoltage,
			StopVoltage:   result.StopVoltage,
			NumTraces:     len(result.AllTraces),
			NumPoints:     result.NumPoints,
			Traces:        result.AllTraces,
			Channels:      extractChannels(result),
			RecordedAt:    time.Now(),
			TotalDuration: result.TotalDuration,
		}

		var err error
		rawRef, err = db.rawDB.Store(rawRecord)
		if err != nil {
			return "", fmt.Errorf("failed to store raw traces: %w", err)
		}
	}

	// 2. Build averaged-only result (without raw traces)
	avgResult := &AveragedMeasurementResult{
		MeasurementID: result.MeasurementID,
		SweepGate:     result.SweepGate,
		StartVoltage:  result.StartVoltage,
		StopVoltage:   result.StopVoltage,
		NumPoints:     result.NumPoints,
		NumSweeps:     result.NumSweeps,
		AllTraces:     nil, // Raw traces are NOT included in averaged DB
		AveragedTrace: result.AveragedTrace,
		TotalDuration: result.TotalDuration,
		RawRef:        rawRef,
	}

	// 3. Write averaged result to averaged/ subdirectory
	avgPath := filepath.Join(db.basePath, "averaged")
	writer, err := NewHDF5Writer(HDF5Config{
		BasePath:       avgPath,
		FilePrefix:     "sweep",
		WriteRawTraces: false,
	})
	if err != nil {
		return "", err
	}
	defer writer.Close()

	avgFilePath, err := writer.WriteAveragedMeasurement(avgResult)
	if err != nil {
		return "", err
	}

	// Also set the path on the original result so callers can access it
	result.DatabasePath = avgFilePath
	result.RawRef = rawRef

	// 4. Update index with links to both
	db.index[result.MeasurementID] = MeasurementIndex{
		MeasurementID: result.MeasurementID,
		FilePath:      avgFilePath,
		SweepGate:     result.SweepGate,
		NumPoints:     result.NumPoints,
		NumSweeps:     result.NumSweeps,
		StoredAt:      time.Now(),
		RawDataRef:    rawRef,
	}

	if err := db.saveIndex(); err != nil {
		return avgFilePath, fmt.Errorf("stored data but failed to update index: %w", err)
	}

	return avgFilePath, nil
}

// Load loads an averaged measurement result by ID (does NOT include raw traces).
func (db *MeasurementDatabase) Load(measurementID string) (*AveragedMeasurementResult, error) {
	idx, exists := db.index[measurementID]
	if !exists {
		return nil, fmt.Errorf("measurement not found: %s", measurementID)
	}

	data, err := os.ReadFile(idx.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var result AveragedMeasurementResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}

	return &result, nil
}

// LoadRawTraces loads the raw trace data for a measurement.
// This is hub-local and should NOT be shared with falcon.
func (db *MeasurementDatabase) LoadRawTraces(measurementID string) (*RawTraceRecord, error) {
	return db.rawDB.Load(measurementID)
}

// LoadWithRawTraces loads both averaged and raw data, populating AllTraces.
func (db *MeasurementDatabase) LoadWithRawTraces(measurementID string) (*AveragedMeasurementResult, error) {
	avgResult, err := db.Load(measurementID)
	if err != nil {
		return nil, err
	}

	rawRecord, err := db.rawDB.Load(measurementID)
	if err != nil {
		// Raw data might not exist (e.g., old format)
		return avgResult, nil
	}

	avgResult.AllTraces = rawRecord.Traces
	return avgResult, nil
}

// List returns all stored measurement IDs.
func (db *MeasurementDatabase) List() []MeasurementIndex {
	result := make([]MeasurementIndex, 0, len(db.index))
	for _, idx := range db.index {
		result = append(result, idx)
	}
	return result
}

// RawDB returns the underlying raw trace database for direct access.
func (db *MeasurementDatabase) RawDB() *RawTraceDatabase {
	return db.rawDB
}

// loadIndex loads the index from disk.
func (db *MeasurementDatabase) loadIndex() error {
	indexPath := filepath.Join(db.basePath, "averaged", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &db.index)
}

// saveIndex saves the index to disk.
func (db *MeasurementDatabase) saveIndex() error {
	indexPath := filepath.Join(db.basePath, "averaged", "index.json")
	data, err := json.MarshalIndent(db.index, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexPath, data, 0644)
}

// GetFilePath returns the averaged file path for a measurement.
func (db *MeasurementDatabase) GetFilePath(measurementID string) (string, error) {
	idx, exists := db.index[measurementID]
	if !exists {
		return "", fmt.Errorf("measurement not found: %s", measurementID)
	}
	return idx.FilePath, nil
}

// extractChannels gets channel names from the first trace point.
func extractChannels(result *AveragedMeasurementResult) []string {
	if len(result.AveragedTrace.Points) == 0 {
		return nil
	}
	channels := make([]string, 0, len(result.AveragedTrace.Points[0].Measurements))
	for ch := range result.AveragedTrace.Points[0].Measurements {
		channels = append(channels, ch)
	}
	return channels
}

// ===========================================================================
// DC Point Collection Storage
// ===========================================================================

// DCPointCollectionResult holds a group of set-voltage / get-current operations
// that were issued as a single measurement command from the hub.
type DCPointCollectionResult struct {
	MeasurementID string    `json:"measurement_id"`
	Points        []DCPoint `json:"points"`
	Channels      []string  `json:"channels"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
}

// DCPoint is a single set-voltage → read-current sample.
type DCPoint struct {
	GateName    string             `json:"gate_name"`
	SetVoltage  float64            `json:"set_voltage"`
	Measurements map[string]float64 `json:"measurements"` // channel -> measured current
	Timestamp   time.Time          `json:"timestamp"`
}

// StoreDCCollection stores grouped DC get/set points as a single measurement.
// Written to averaged/dc_collection_<id>.json with measurement_type "dc_collection".
func (db *MeasurementDatabase) StoreDCCollection(result *DCPointCollectionResult) (string, error) {
	avgPath := filepath.Join(db.basePath, "averaged")
	filePath := filepath.Join(avgPath, fmt.Sprintf("dc_collection_%s.json", result.MeasurementID))

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal DC collection: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write DC collection: %w", err)
	}

	// Build sweep gate label from unique gate names
	gateSet := make(map[string]struct{})
	for _, pt := range result.Points {
		gateSet[pt.GateName] = struct{}{}
	}
	gates := make([]string, 0, len(gateSet))
	for g := range gateSet {
		gates = append(gates, g)
	}
	gateLabel := strings.Join(gates, ", ")

	db.index[result.MeasurementID] = MeasurementIndex{
		MeasurementID:   result.MeasurementID,
		FilePath:        filePath,
		SweepGate:       gateLabel,
		NumPoints:       len(result.Points),
		NumSweeps:       1,
		StoredAt:        time.Now(),
		MeasurementType: "dc_collection",
	}

	if err := db.saveIndex(); err != nil {
		return filePath, fmt.Errorf("stored DC collection but failed to update index: %w", err)
	}

	return filePath, nil
}

// ===========================================================================
// 1D Axis Sweep Storage
// ===========================================================================

// AxisSweep1DResult holds multiple 1D sweeps along a parameterised axis.
// Each sweep is performed at a specific angle θ where the voltage axis is
// defined as V = V_gate1 * cos(θ) + V_gate2 * sin(θ).
type AxisSweep1DResult struct {
	MeasurementID string            `json:"measurement_id"`
	Gate1         string            `json:"gate1"`           // e.g. "P1"
	Gate2         string            `json:"gate2"`           // e.g. "P2"
	Channels      []string          `json:"channels"`
	Sweeps        []AxisSweep1DLine `json:"sweeps"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
}

// AxisSweep1DLine is one 1D sweep at a particular angle.
type AxisSweep1DLine struct {
	AngleDeg    float64                `json:"angle_deg"`     // θ in degrees
	Voltages    []float64              `json:"voltages"`      // axis voltage values
	ChannelData map[string][]float64   `json:"channel_data"`  // channel -> current array
	Gate1V      []float64              `json:"gate1_voltages"` // V_gate1 = V * cos(θ)
	Gate2V      []float64              `json:"gate2_voltages"` // V_gate2 = V * sin(θ)
	Timestamp   time.Time              `json:"timestamp"`
}

// StoreAxisSweep stores a multi-angle 1D axis sweep measurement.
// Written to averaged/axis_sweep_<id>.json with measurement_type "axis_sweep".
func (db *MeasurementDatabase) StoreAxisSweep(result *AxisSweep1DResult) (string, error) {
	avgPath := filepath.Join(db.basePath, "averaged")
	filePath := filepath.Join(avgPath, fmt.Sprintf("axis_sweep_%s.json", result.MeasurementID))

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal axis sweep: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write axis sweep: %w", err)
	}

	numPts := 0
	for _, s := range result.Sweeps {
		numPts += len(s.Voltages)
	}

	db.index[result.MeasurementID] = MeasurementIndex{
		MeasurementID:   result.MeasurementID,
		FilePath:        filePath,
		SweepGate:       result.Gate1 + " · " + result.Gate2 + " axis",
		NumPoints:       numPts,
		NumSweeps:       len(result.Sweeps),
		StoredAt:        time.Now(),
		MeasurementType: "axis_sweep",
	}

	if err := db.saveIndex(); err != nil {
		return filePath, fmt.Errorf("stored axis sweep but failed to update index: %w", err)
	}

	return filePath, nil
}

// ===========================================================================
// 2D Sweep Storage
// ===========================================================================

// Sweep2DMeasurementResult holds a complete 2D voltage sweep.
// Each row in Lines corresponds to one Y-axis voltage value and contains
// the 1D trace data for the fast (X) axis sweep at that Y setting.
type Sweep2DMeasurementResult struct {
	MeasurementID string          `json:"measurement_id"`
	XGate         string          `json:"x_gate"`         // Fast axis gate name
	YGate         string          `json:"y_gate"`         // Slow axis gate name
	XVoltages     []float64       `json:"x_voltages"`     // Voltage values along X
	YVoltages     []float64       `json:"y_voltages"`     // Voltage values along Y
	Channels      []string        `json:"channels"`       // Measurement channel names
	ChannelData   map[string][][]float64 `json:"channel_data"` // channel -> [y][x] current matrix
	Lines         []Sweep2DLine   `json:"lines"`          // Individual 1D line-cuts
	StartTime     time.Time       `json:"start_time"`
	EndTime       time.Time       `json:"end_time"`
}

// Sweep2DLine is one horizontal line-cut (fixed Y) in a 2D sweep.
type Sweep2DLine struct {
	YVoltage  float64            `json:"y_voltage"`
	YIndex    int                `json:"y_index"`
	Currents  map[string][]float64 `json:"currents"` // channel -> X-axis current array
	Timestamp time.Time          `json:"timestamp"`
}

// Store2D stores a 2D sweep measurement result.
// It writes the full 2D dataset to averaged/sweep_2d_<id>.json and
// registers it in the index with measurement_type = "2d".
func (db *MeasurementDatabase) Store2D(result *Sweep2DMeasurementResult) (string, error) {
	avgPath := filepath.Join(db.basePath, "averaged")
	filePath := filepath.Join(avgPath, fmt.Sprintf("sweep_2d_%s.json", result.MeasurementID))

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal 2D result: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write 2D result: %w", err)
	}

	db.index[result.MeasurementID] = MeasurementIndex{
		MeasurementID:   result.MeasurementID,
		FilePath:        filePath,
		SweepGate:       result.XGate + " × " + result.YGate,
		NumPoints:       len(result.XVoltages) * len(result.YVoltages),
		NumSweeps:       len(result.YVoltages),
		StoredAt:        time.Now(),
		MeasurementType: "2d",
	}

	if err := db.saveIndex(); err != nil {
		return filePath, fmt.Errorf("stored 2D data but failed to update index: %w", err)
	}

	return filePath, nil
}
