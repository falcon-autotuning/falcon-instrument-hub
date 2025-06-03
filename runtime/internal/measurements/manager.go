package measurements

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Manager handles the storage and organization of measurement files
type Manager struct {
	baseDataDir string
	db          *MeasurementDB
}

// NewManager creates a new measurement manager
func NewManager(baseDataDir string, dbPath string) (*Manager, error) {
	// Ensure base data directory exists
	if err := os.MkdirAll(baseDataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base data directory: %w", err)
	}

	// Initialize database
	db, err := NewMeasurementDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	manager := &Manager{
		baseDataDir: baseDataDir,
		db:          db,
	}

	// Load existing indexes on startup
	if err := manager.loadExistingIndexes(); err != nil {
		log.Printf("Warning: failed to load existing indexes: %v", err)
	}

	return manager, nil
}

// Close closes the manager and its resources
func (m *Manager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

// loadExistingIndexes scans the database to rebuild in-memory state on startup
func (m *Manager) loadExistingIndexes() error {
	log.Println("Loading existing measurement indexes from database...")
	
	// Query all measurements to understand current state
	filters := MeasurementFilters{Limit: 0} // No limit, get all
	measurements, err := m.db.QueryMeasurements(filters)
	if err != nil {
		return fmt.Errorf("failed to query existing measurements: %w", err)
	}

	completeCount := 0
	incompleteCount := 0
	for _, measurement := range measurements {
		if measurement.Status == "complete" {
			completeCount++
		} else {
			incompleteCount++
		}
	}

	log.Printf("Loaded %d measurements (%d complete, %d incomplete)", 
		len(measurements), completeCount, incompleteCount)
	
	// Clean up any incomplete measurements older than 1 hour (optional)
	// This handles cases where the process crashed before completing a measurement
	cutoffTime := time.Now().Add(-1 * time.Hour)
	if err := m.cleanupOldIncomplete(cutoffTime); err != nil {
		log.Printf("Warning: failed to cleanup old incomplete measurements: %v", err)
	}

	return nil
}

// cleanupOldIncomplete removes incomplete measurements older than the cutoff time
func (m *Manager) cleanupOldIncomplete(cutoffTime time.Time) error {
	query := `DELETE FROM measurements WHERE status = 'incomplete' AND created < ?`
	result, err := m.db.db.Exec(query, cutoffTime)
	if err != nil {
		return err
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("Cleaned up %d old incomplete measurements", rowsAffected)
	}
	
	return nil
}

// AllocateMeasurementID generates a unique integer ID and returns the expected file path
// This should be called BEFORE creating the HDF5 file
func (m *Manager) AllocateMeasurementID(timestamp time.Time) (uniqueID int, expectedPath string, err error) {
	// Allocate unique ID in database (reserves it as incomplete)
	uniqueID, err = m.db.AllocateUniqueID(timestamp)
	if err != nil {
		return 0, "", fmt.Errorf("failed to allocate unique ID: %w", err)
	}

	// Extract date components for directory structure
	year, month, day := timestamp.Date()
	
	// Create directory structure: baseDataDir/YYYY/MM/DD/
	dateDir := filepath.Join(m.baseDataDir, 
		fmt.Sprintf("%04d", year),
		fmt.Sprintf("%02d", int(month)),
		fmt.Sprintf("%02d", day))
	
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return 0, "", fmt.Errorf("failed to create date directory: %w", err)
	}

	// Expected file path using integer ID
	expectedPath = filepath.Join(dateDir, fmt.Sprintf("%d.h5", uniqueID))

	log.Printf("Allocated measurement ID %d, expected path: %s", uniqueID, expectedPath)
	return uniqueID, expectedPath, nil
}

// CompleteMeasurement marks a measurement as complete and stores final metadata
// Call this after the HDF5 file has been successfully created
func (m *Manager) CompleteMeasurement(uniqueID int, measurementTitle string, filePath string) (*MeasurementMetadata, error) {
	// Verify the file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("measurement file does not exist: %w", err)
	}

	// Update the database record
	err = m.db.CompleteMeasurement(uniqueID, measurementTitle, filePath, fileInfo.Size())
	if err != nil {
		return nil, fmt.Errorf("failed to complete measurement: %w", err)
	}

	log.Printf("Completed measurement ID %d: %s (%d bytes)", uniqueID, measurementTitle, fileInfo.Size())

	// Return the completed metadata
	return m.db.GetMeasurement(uniqueID)
}

// GetMeasurement retrieves measurement metadata by unique ID
func (m *Manager) GetMeasurement(uniqueID int) (*MeasurementMetadata, error) {
	return m.db.GetMeasurement(uniqueID)
}

// QueryMeasurements searches for measurements based on filters
func (m *Manager) QueryMeasurements(filters MeasurementFilters) ([]*MeasurementMetadata, error) {
	return m.db.QueryMeasurements(filters)
}

// GetMeasurementFilePath returns the full path to a measurement file
func (m *Manager) GetMeasurementFilePath(uniqueID int) (string, error) {
	metadata, err := m.db.GetMeasurement(uniqueID)
	if err != nil {
		return "", err
	}
	return metadata.FilePath, nil
}
