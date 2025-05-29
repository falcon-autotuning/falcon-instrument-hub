package database

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"instrument-server/internal/database" // Import the correct package
)

// MockDBConnector for testing purposes
type MockDBConnector struct {
	DB *sql.DB
}

// Open mocks the database connection
func (m *MockDBConnector) Open(driverName, dataSourceName string) (*sql.DB, error) {
	return m.DB, nil
}

func TestDatabaseOperations(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir, err := os.MkdirTemp("", "testdb")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up after the test

	// Construct the database file path within the temporary directory
	// dbPath := filepath.Join(tempDir, "test.db") // Remove unused variable
	connStr := fmt.Sprintf("host=localhost port=5432 user=postgres password=password dbname=test sslmode=disable")

	// Initialize the database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create a mock DBConnector
	connector := &MockDBConnector{DB: db}

	// Initialize the DB instance
	databaseInstance, err := database.NewDB(connector, "localhost", "5432", "postgres", "password", "test")
	if err != nil {
		t.Fatalf("Failed to create database instance: %v", err)
	}
	defer databaseInstance.Close()

	// Test PutCharacteristic
	characteristic := &database.DeviceCharacteristic{
		Name:        "test_characteristic",
		HDF5File:    "test.hdf5",
		Dataset:     "test_dataset",
		Indexes:     []string{"index1", "index2"},
		Uncertainty: 0.1,
		Hash:        "test_hash",
		Time:        time.Now(),
		State:       map[string]interface{}{"key": "value"},
		UUID:        uuid.New().String(),
	}

	if err := databaseInstance.PutCharacteristic(characteristic); err != nil {
		t.Fatalf("Failed to put characteristic: %v", err)
	}

	// Test GetCharacteristicByName
	retrievedCharacteristic, err := databaseInstance.GetCharacteristicByName("test_characteristic")
	if err != nil {
		t.Fatalf("Failed to get characteristic by name: %v", err)
	}

	if retrievedCharacteristic.Name != characteristic.Name {
		t.Errorf("Retrieved characteristic name does not match. Expected: %s, Got: %s", characteristic.Name, retrievedCharacteristic.Name)
	}

	// Test DeleteCharacteristicByName
	if err := databaseInstance.DeleteCharacteristicByName("test_characteristic"); err != nil {
		t.Fatalf("Failed to delete characteristic by name: %v", err)
	}

	// Verify that the characteristic is deleted
	_, err = databaseInstance.GetCharacteristicByName("test_characteristic")
	if err == nil {
		t.Error("Expected error when getting deleted characteristic, but got nil")
	}

	// Test ClearCharacteristics
	if err := databaseInstance.PutCharacteristic(characteristic); err != nil {
		t.Fatalf("Failed to put characteristic: %v", err)
	}

	if err := databaseInstance.ClearCharacteristics(); err != nil {
		t.Fatalf("Failed to clear characteristics: %v", err)
	}

	// Verify that the table is cleared
	_, err = databaseInstance.GetCharacteristicByName("test_characteristic")
	if err == nil {
		t.Error("Expected error when getting characteristic after clearing, but got nil")
	}
}
