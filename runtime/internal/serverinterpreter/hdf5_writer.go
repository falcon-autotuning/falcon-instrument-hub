// Package serverinterpreter provides HDF5 database writing for measurement results.
//
// This file implements storage of averaged measurement results to HDF5 files,
// which can later be accessed by falcon via JetStream.
package serverinterpreter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

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
	config   HDF5Config
	useJSON  bool // Fallback to JSON if HDF5 library not available
}

// NewHDF5Writer creates a new HDF5 writer.
func NewHDF5Writer(config HDF5Config) (*HDF5Writer, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(config.BasePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	writer := &HDF5Writer{
		config:  config,
		useJSON: true, // For now, use JSON fallback
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
	filepath := filepath.Join(w.config.BasePath, filename)

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	result.DatabasePath = filepath
	return filepath, nil
}

// writeHDF5 writes the result to HDF5 format.
// TODO: Implement native HDF5 writing when gonum/hdf5 is available.
func (w *HDF5Writer) writeHDF5(result *AveragedMeasurementResult) (string, error) {
	// For now, fall back to JSON
	return w.writeJSON(result)
}

// Close closes the writer.
func (w *HDF5Writer) Close() error {
	return nil
}

// MeasurementDatabase manages a collection of measurement files.
type MeasurementDatabase struct {
	basePath string
	index    map[string]MeasurementIndex
}

// MeasurementIndex tracks stored measurements.
type MeasurementIndex struct {
	MeasurementID string    `json:"measurement_id"`
	FilePath      string    `json:"file_path"`
	SweepGate     string    `json:"sweep_gate"`
	NumPoints     int       `json:"num_points"`
	NumSweeps     int       `json:"num_sweeps"`
	StoredAt      time.Time `json:"stored_at"`
}

// NewMeasurementDatabase creates a new measurement database.
func NewMeasurementDatabase(basePath string) (*MeasurementDatabase, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db := &MeasurementDatabase{
		basePath: basePath,
		index:    make(map[string]MeasurementIndex),
	}

	// Try to load existing index
	if err := db.loadIndex(); err != nil {
		// Index doesn't exist yet, that's OK
	}

	return db, nil
}

// Store stores a measurement result and updates the index.
func (db *MeasurementDatabase) Store(result *AveragedMeasurementResult) (string, error) {
	writer, err := NewHDF5Writer(HDF5Config{
		BasePath:       db.basePath,
		FilePrefix:     "sweep",
		WriteRawTraces: true,
	})
	if err != nil {
		return "", err
	}
	defer writer.Close()

	filepath, err := writer.WriteAveragedMeasurement(result)
	if err != nil {
		return "", err
	}

	// Update index
	db.index[result.MeasurementID] = MeasurementIndex{
		MeasurementID: result.MeasurementID,
		FilePath:      filepath,
		SweepGate:     result.SweepGate,
		NumPoints:     result.NumPoints,
		NumSweeps:     result.NumSweeps,
		StoredAt:      time.Now(),
	}

	if err := db.saveIndex(); err != nil {
		return filepath, fmt.Errorf("stored data but failed to update index: %w", err)
	}

	return filepath, nil
}

// Load loads a measurement result by ID.
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

// List returns all stored measurement IDs.
func (db *MeasurementDatabase) List() []MeasurementIndex {
	result := make([]MeasurementIndex, 0, len(db.index))
	for _, idx := range db.index {
		result = append(result, idx)
	}
	return result
}

// loadIndex loads the index from disk.
func (db *MeasurementDatabase) loadIndex() error {
	indexPath := filepath.Join(db.basePath, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &db.index)
}

// saveIndex saves the index to disk.
func (db *MeasurementDatabase) saveIndex() error {
	indexPath := filepath.Join(db.basePath, "index.json")
	data, err := json.MarshalIndent(db.index, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexPath, data, 0644)
}

// GetFilePath returns the file path for a measurement.
func (db *MeasurementDatabase) GetFilePath(measurementID string) (string, error) {
	idx, exists := db.index[measurementID]
	if !exists {
		return "", fmt.Errorf("measurement not found: %s", measurementID)
	}
	return idx.FilePath, nil
}
