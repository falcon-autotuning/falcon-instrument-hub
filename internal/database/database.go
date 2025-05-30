package database

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // Import the PostgreSQL driver
)

//go:embed specschema.sql
var schemaSQL string

// DB struct to hold the database connection
type DB struct {
	conn *sql.DB
}

// JSONPrimitive type alias
type JSONPrimitive = interface{}

// DeviceCharacteristic struct to represent the data
type DeviceCharacteristic struct {
	Name        string                   `json:"name" db:"name"`
	HDF5File    string                   `json:"hdf5_file" db:"hdf5_file"` // Path to the HDF5 file
	Dataset     string                   `json:"dataset" db:"dataset"`     // Name of the dataset within the HDF5 file
	Indexes     []string                 `json:"indexes" db:"indexes"`
	Uncertainty float64                  `json:"uncertainty" db:"uncertainty"`
	Hash        string                   `json:"hash" db:"hash"`
	Time        time.Time                `json:"time" db:"time"`
	State       map[string]JSONPrimitive `json:"state" db:"state"` // Other relevant metadata
	UUID        string                   `json:"uuid" db:"uuid"`
}

// NewDB creates and initializes a new DB instance and sets up the database
func NewDB(host, port, user, password, dbname string) (*DB, error) {
	// Connect to the default 'postgres' database initially to check/create our target database
	defaultConnStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable", host, port, user, password)
	defaultConn, err := sql.Open("postgres", defaultConnStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to default database: %w", err)
	}
	defer defaultConn.Close()

	// Check if the target database exists
	var exists bool
	err = defaultConn.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbname).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check if database exists: %w", err)
	}

	// Create the target database if it doesn't exist
	if !exists {
		log.Printf("Database '%s' does not exist. Creating...", dbname)
		_, err = defaultConn.Exec(fmt.Sprintf("CREATE DATABASE %s", dbname))
		if err != nil {
			return nil, fmt.Errorf("failed to create database '%s': %w", dbname, err)
		}
		log.Printf("Database '%s' created successfully.", dbname)
	}

	// Now connect to the target database
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open target database '%s': %w", dbname, err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database '%s': %w", dbname, err)
	}
	log.Printf("Successfully connected to database '%s'!", dbname)

	database := &DB{conn: conn}

	// Setup the database schema from the embedded file
	if err := database.setupSchema(); err != nil {
		return nil, fmt.Errorf("failed to setup schema: %w", err)
	}

	return database, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// setupSchema executes the embedded SQL schema
func (db *DB) setupSchema() error {
	log.Println("Setting up database schema from embedded file...")

	// Choose one of your main tables to check for existence
	const mainTableName = "device_characteristics"

	exists, err := db.tableExists(mainTableName)
	if err != nil {
		return fmt.Errorf("failed to check if table '%s' exists: %w", mainTableName, err)
	}

	if exists {
		log.Printf("Table '%s' already exists. Skipping schema setup.", mainTableName)
		return nil // Schema is likely already set up, so we exit without executing schemaSQL
	}
	_, err = db.conn.Exec(schemaSQL)
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}
	log.Println("Database schema setup complete from embedded file.")
	return nil
}

// tableExists checks if a table exists
func (db *DB) tableExists(tableName string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow("SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)", tableName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("error checking table existence: %w", err)
	}
	return exists, nil
}

// ExecuteQuery executes a query
func (db *DB) ExecuteQuery(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// ExecuteNonQuery executes a non-query command
func (db *DB) ExecuteNonQuery(command string, args ...interface{}) (sql.Result, error) {
	return db.conn.Exec(command, args...)
}

// PutCharacteristic inserts a new DeviceCharacteristic into the database.
func (db *DB) PutCharacteristic(characteristic *DeviceCharacteristic) error {
	query := `
        INSERT INTO device_characteristics (name, hdf5_file, dataset, indexes, uncertainty, hash, time, state, uuid)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `
	// Convert the string slice to a PostgreSQL array string representation.
	// indexesString := "{" + strings.Join(characteristic.Indexes, ",") + "}" // Old way
	indexesJSON, err := json.Marshal(characteristic.Indexes)
	if err != nil {
		return fmt.Errorf("failed to marshal indexes to JSON: %w", err)
	}

	// Convert the state maps to JSON strings
	stateJSON, err := json.Marshal(characteristic.State)
	if err != nil {
		return fmt.Errorf("failed to marshal state to JSON: %w", err)
	}

	_, err = db.conn.Exec(query,
		characteristic.Name,
		characteristic.HDF5File,
		characteristic.Dataset,
		indexesJSON, // Use the JSON marshaled string here
		characteristic.Uncertainty,
		characteristic.Hash,
		characteristic.Time,
		stateJSON, // Use the JSON string
		characteristic.UUID,
	)
	if err != nil {
		return fmt.Errorf("failed to put characteristic: %w", err)
	}
	return nil
}

// GetCharacteristicByName retrieves a DeviceCharacteristic by its name.
func (db *DB) GetCharacteristicByName(name string) (*DeviceCharacteristic, error) {
	query := `
        SELECT name, hdf5_file, dataset, indexes, uncertainty, hash, time, state, uuid
        FROM device_characteristics
        WHERE name = $1
    `
	row := db.conn.QueryRow(query, name)

	var characteristic DeviceCharacteristic
	err := row.Scan(
		&characteristic.Name,
		&characteristic.HDF5File,
		&characteristic.Dataset,
		&characteristic.Indexes,
		&characteristic.Uncertainty,
		&characteristic.Hash,
		&characteristic.Time,
		&characteristic.State,
		&characteristic.UUID,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("characteristic with name '%s' not found", name)
		}
		return nil, fmt.Errorf("failed to get characteristic by name: %w", err)
	}
	return &characteristic, nil
}

// DeleteCharacteristicByName removes a DeviceCharacteristic by its name.
func (db *DB) DeleteCharacteristicByName(name string) error {
	query := `DELETE FROM device_characteristics WHERE name = $1`
	result, err := db.conn.Exec(query, name)
	if err != nil {
		return fmt.Errorf("failed to delete characteristic by name '%s': %w", name, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		// Error getting rows affected, but deletion might have succeeded. Log it.
		log.Printf("Could not get rows affected after deleting characteristic '%s': %v", name, err)
		return nil // Or return the original deletion error if preferred
	}

	if rowsAffected == 0 {
		return fmt.Errorf("characteristic with name '%s' not found for deletion", name)
	}

	log.Printf("Successfully deleted characteristic with name '%s'", name)
	return nil
}

// ClearCharacteristics removes all entries from the device_characteristics table.
func (db *DB) ClearCharacteristics() error {
	query := `DELETE FROM device_characteristics`
	result, err := db.conn.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to clear characteristics table: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Could not get rows affected after clearing characteristics table: %v", err)
		return nil // Or return the original deletion error
	}

	log.Printf("Successfully cleared %d characteristics from the table.", rowsAffected)
	return nil
}
