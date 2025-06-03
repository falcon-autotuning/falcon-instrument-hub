package measurements

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) (*MeasurementDB, string) {
	// Create temporary directory for test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_measurements.db")

	db, err := NewMeasurementDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	return db, dbPath
}

func TestNewMeasurementDB(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "subdir", "test.db")

	db, err := NewMeasurementDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Verify database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}

func TestAllocateUniqueID(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	timestamp := time.Now()

	// Test first allocation
	id1, err := db.AllocateUniqueID(timestamp)
	if err != nil {
		t.Fatalf("Failed to allocate first ID: %v", err)
	}
	if id1 != 1 {
		t.Errorf("Expected first ID to be 1, got %d", id1)
	}

	// Test second allocation
	id2, err := db.AllocateUniqueID(timestamp)
	if err != nil {
		t.Fatalf("Failed to allocate second ID: %v", err)
	}
	if id2 != 2 {
		t.Errorf("Expected second ID to be 2, got %d", id2)
	}

	// Verify records exist in database and are incomplete
	measurement1, err := db.GetMeasurement(id1)
	if err != nil {
		t.Fatalf("Failed to get measurement 1: %v", err)
	}
	if measurement1.Status != "incomplete" {
		t.Errorf("Expected status 'incomplete', got '%s'", measurement1.Status)
	}
	if measurement1.MeasurementTitle != "" {
		t.Errorf("Expected empty measurement title for incomplete measurement, got '%s'", measurement1.MeasurementTitle)
	}
	if measurement1.FilePath != "" {
		t.Errorf("Expected empty file path for incomplete measurement, got '%s'", measurement1.FilePath)
	}
	if measurement1.FileSize != 0 {
		t.Errorf("Expected zero file size for incomplete measurement, got %d", measurement1.FileSize)
	}
}

func TestAllocateUniqueID_Concurrent(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	timestamp := time.Now()
	numGoroutines := 20 // Increase to stress test more
	results := make(chan int, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Launch concurrent allocations
	for range numGoroutines {
		go func() {
			id, err := db.AllocateUniqueID(timestamp)
			if err != nil {
				errors <- err
				return
			}
			results <- id
		}()
	}

	// Collect results
	ids := make(map[int]bool)
	var allErrors []error

	for range numGoroutines {
		select {
		case id := <-results:
			if ids[id] {
				t.Errorf("Duplicate ID allocated: %d", id)
			}
			ids[id] = true
		case err := <-errors:
			allErrors = append(allErrors, err)
		case <-time.After(15 * time.Second):
			t.Fatal("Timeout waiting for allocations")
		}
	}

	// Log any errors for debugging
	if len(allErrors) > 0 {
		t.Logf("Got %d errors out of %d attempts", len(allErrors), numGoroutines)
		for i, err := range allErrors {
			t.Logf("Error %d: %v", i+1, err)
		}
	}

	// With the improved implementation, we should get all allocations successful
	successfulAllocations := len(ids)
	if successfulAllocations != numGoroutines {
		t.Errorf("Expected %d successful allocations, got %d", numGoroutines, successfulAllocations)

		// If we didn't get all allocations, at least verify no duplicates
		if successfulAllocations > 0 {
			t.Logf("Successfully allocated %d unique IDs", successfulAllocations)

			// Verify IDs are sequential starting from 1
			expectedIDs := make(map[int]bool)
			for i := 1; i <= successfulAllocations; i++ {
				expectedIDs[i] = true
			}

			// Check if we got the expected sequential IDs
			sequentialCount := 0
			for id := range ids {
				if expectedIDs[id] {
					sequentialCount++
				}
			}

			if sequentialCount != successfulAllocations {
				t.Errorf("IDs are not sequential: got %v", ids)
			}
		}
	}
}

func TestCompleteMeasurement(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	timestamp := time.Now()

	// Allocate an ID first
	id, err := db.AllocateUniqueID(timestamp)
	if err != nil {
		t.Fatalf("Failed to allocate ID: %v", err)
	}

	// Complete the measurement
	title := "Test Measurement"
	filePath := "/path/to/test.h5"
	fileSize := int64(12345)

	err = db.CompleteMeasurement(id, title, filePath, fileSize)
	if err != nil {
		t.Fatalf("Failed to complete measurement: %v", err)
	}

	// Verify the measurement was updated
	measurement, err := db.GetMeasurement(id)
	if err != nil {
		t.Fatalf("Failed to get measurement: %v", err)
	}

	if measurement.MeasurementTitle != title {
		t.Errorf("Expected title '%s', got '%s'", title, measurement.MeasurementTitle)
	}
	if measurement.FilePath != filePath {
		t.Errorf("Expected file path '%s', got '%s'", filePath, measurement.FilePath)
	}
	if measurement.FileSize != fileSize {
		t.Errorf("Expected file size %d, got %d", fileSize, measurement.FileSize)
	}
	if measurement.Status != "complete" {
		t.Errorf("Expected status 'complete', got '%s'", measurement.Status)
	}
}

func TestCompleteMeasurement_NonExistentID(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	err := db.CompleteMeasurement(999, "Test", "/path", 123)
	if err == nil {
		t.Error("Expected error when completing non-existent measurement")
	}
}

func TestCompleteMeasurement_AlreadyComplete(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	timestamp := time.Now()

	// Allocate and complete a measurement
	id, err := db.AllocateUniqueID(timestamp)
	if err != nil {
		t.Fatalf("Failed to allocate ID: %v", err)
	}

	err = db.CompleteMeasurement(id, "Test", "/path", 123)
	if err != nil {
		t.Fatalf("Failed to complete measurement: %v", err)
	}

	// Try to complete again
	err = db.CompleteMeasurement(id, "Test2", "/path2", 456)
	if err == nil {
		t.Error("Expected error when completing already complete measurement")
	}
}

func TestGetMeasurement_NotFound(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	_, err := db.GetMeasurement(999)
	if err == nil {
		t.Error("Expected error when getting non-existent measurement")
	}
}

func TestQueryMeasurements(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	// Create test data
	timestamp1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	timestamp2 := time.Date(2024, 1, 16, 10, 0, 0, 0, time.UTC)
	timestamp3 := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	// Allocate and complete measurements
	id1, _ := db.AllocateUniqueID(timestamp1)
	db.CompleteMeasurement(id1, "Test Measurement 1", "/path1.h5", 100)

	id2, _ := db.AllocateUniqueID(timestamp2)
	db.CompleteMeasurement(id2, "Test Measurement 2", "/path2.h5", 200)

	_, _ = db.AllocateUniqueID(timestamp3)
	// Leave id3 incomplete

	// Test query all
	filters := MeasurementFilters{}
	measurements, err := db.QueryMeasurements(filters)
	if err != nil {
		t.Fatalf("Failed to query measurements: %v", err)
	}
	if len(measurements) != 3 {
		t.Errorf("Expected 3 measurements, got %d", len(measurements))
	}

	// Test query by status
	filters = MeasurementFilters{Status: "complete"}
	measurements, err = db.QueryMeasurements(filters)
	if err != nil {
		t.Fatalf("Failed to query complete measurements: %v", err)
	}
	if len(measurements) != 2 {
		t.Errorf("Expected 2 complete measurements, got %d", len(measurements))
	}

	// Test query by date range
	fromDate := time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)
	toDate := time.Date(2024, 1, 16, 23, 59, 59, 0, time.UTC)
	filters = MeasurementFilters{FromDate: &fromDate, ToDate: &toDate}
	measurements, err = db.QueryMeasurements(filters)
	if err != nil {
		t.Fatalf("Failed to query measurements by date: %v", err)
	}
	if len(measurements) != 1 {
		t.Errorf("Expected 1 measurement in date range, got %d", len(measurements))
	}

	// Test query by title pattern
	filters = MeasurementFilters{TitlePattern: "Test Measurement 1"}
	measurements, err = db.QueryMeasurements(filters)
	if err != nil {
		t.Fatalf("Failed to query measurements by title: %v", err)
	}
	if len(measurements) != 1 {
		t.Errorf("Expected 1 measurement with title pattern, got %d", len(measurements))
	}

	// Test query with limit
	filters = MeasurementFilters{Limit: 2}
	measurements, err = db.QueryMeasurements(filters)
	if err != nil {
		t.Fatalf("Failed to query measurements with limit: %v", err)
	}
	if len(measurements) != 2 {
		t.Errorf("Expected 2 measurements with limit, got %d", len(measurements))
	}
}

func TestFilePathUniqueness(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()

	timestamp := time.Now()

	// Allocate and complete first measurement
	id1, _ := db.AllocateUniqueID(timestamp)
	filePath := "/unique/path/test.h5"
	err := db.CompleteMeasurement(id1, "Test 1", filePath, 100)
	if err != nil {
		t.Fatalf("Failed to complete first measurement: %v", err)
	}

	// Try to complete second measurement with same file path
	id2, _ := db.AllocateUniqueID(timestamp)
	err = db.CompleteMeasurement(id2, "Test 2", filePath, 200)
	if err == nil {
		t.Error("Expected error when using duplicate file path")
	}
}

func TestDatabaseIntegrity(t *testing.T) {
	db, dbPath := setupTestDB(t)

	// Add some test data
	timestamp := time.Now()
	id1, _ := db.AllocateUniqueID(timestamp)
	db.CompleteMeasurement(id1, "Test", "/path1.h5", 100)

	// Close and reopen database
	db.Close()

	db2, err := NewMeasurementDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen database: %v", err)
	}
	defer db2.Close()

	// Verify data persisted
	measurement, err := db2.GetMeasurement(id1)
	if err != nil {
		t.Fatalf("Failed to get measurement after reopen: %v", err)
	}
	if measurement.MeasurementTitle != "Test" {
		t.Errorf("Data not persisted correctly")
	}
}
