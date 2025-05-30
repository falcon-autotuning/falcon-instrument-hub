package database

import (
	"database/sql"
	"fmt"
	"log"
	"testing"
	"time"

	"instrument-server/internal/database" // Import the correct package

	"github.com/google/uuid"

	_ "github.com/lib/pq"
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
	// Generate a unique database name for this test run
	testDBName := fmt.Sprintf("testdb_%s", uuid.New().String())

	// Database connection parameters
	dbHost := "localhost"
	dbPort := "5432"
	dbUser := "postgres"
	dbPassword := "falcon_123"

	// Construct the connection string to the default database (postgres)
	defaultDBConnStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable", dbHost, dbPort, dbUser, dbPassword)

	// Connect to the default database to create the test database
	defaultDB, err := sql.Open("postgres", defaultDBConnStr)
	if err != nil {
		t.Fatalf("Failed to connect to default database: %v", err)
	}
	defer defaultDB.Close()

	// Create the test database
	_, err = defaultDB.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() {
		// Drop the test database after the test is complete
		_, err := defaultDB.Exec(fmt.Sprintf("DROP DATABASE %s", testDBName))
		if err != nil {
			log.Printf("Failed to drop test database: %v", err) // Log the error, but don't fail the test
		}
	}()

	// Construct the connection string to the test database
	testDBConnStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPassword, testDBName)

	// Connect to the test database
	testDB, err := sql.Open("postgres", testDBConnStr)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}
	defer testDB.Close()

	// Create a mock DBConnector
	connector := &MockDBConnector{DB: testDB}

	// Initialize the DB instance
	databaseInstance, err := database.NewDB(connector, dbHost, dbPort, dbUser, dbPassword, testDBName)
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
