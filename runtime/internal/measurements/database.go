package measurements

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// MeasurementDB manages the SQLite database for measurement metadata
type MeasurementDB struct {
	db *sql.DB
}

// MeasurementMetadata represents the metadata stored in the database
type MeasurementMetadata struct {
	UniqueID         int       `json:"unique_id"` // Integer primary key
	Timestamp        time.Time `json:"timestamp"`
	MeasurementTitle string    `json:"measurement_title"`
	FilePath         string    `json:"file_path"`
	FileSize         int64     `json:"file_size"`
	Status           string    `json:"status"` // "incomplete", "complete"
	Created          time.Time `json:"created"`
}

// NewMeasurementDB creates a new measurement database connection
func NewMeasurementDB(dbPath string) (*MeasurementDB, error) {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open with better concurrency settings and immediate mode
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL&_sync=NORMAL&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings for better concurrency
	db.SetMaxOpenConns(1) // Use single connection for SQLite to avoid lock contention
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	mdb := &MeasurementDB{db: db}
	if err := mdb.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	return mdb, nil
}

// initSchema creates the necessary tables
func (mdb *MeasurementDB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS measurements (
		unique_id INTEGER PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		measurement_title TEXT,
		file_path TEXT UNIQUE,
		file_size INTEGER,
		status TEXT NOT NULL DEFAULT 'incomplete',
		created DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS id_counter (
		id INTEGER PRIMARY KEY DEFAULT 1,
		next_id INTEGER NOT NULL DEFAULT 1
	);

	CREATE INDEX IF NOT EXISTS idx_measurements_timestamp ON measurements(timestamp);
	CREATE INDEX IF NOT EXISTS idx_measurements_title ON measurements(measurement_title);
	CREATE INDEX IF NOT EXISTS idx_measurements_date ON measurements(date(timestamp));
	CREATE INDEX IF NOT EXISTS idx_measurements_status ON measurements(status);
	`

	_, err := mdb.db.Exec(schema)
	return err
}

// Close closes the database connection
func (mdb *MeasurementDB) Close() error {
	if mdb.db != nil {
		return mdb.db.Close()
	}
	return nil
}

// AllocateUniqueID reserves the next available unique ID and marks it as incomplete
// AllocateUniqueID reserves the next available unique ID and marks it as incomplete
func (mdb *MeasurementDB) AllocateUniqueID(timestamp time.Time) (int, error) {
	// Retry logic for handling database locks with exponential backoff
	maxRetries := 10
	for attempt := range maxRetries {
		id, err := mdb.attemptAllocateID(timestamp)
		if err == nil {
			return id, nil
		}

		// Check if it's a locking error and we should retry
		errorStr := err.Error()
		if attempt < maxRetries-1 && (strings.Contains(errorStr, "database is locked") ||
			strings.Contains(errorStr, "database locked") ||
			strings.Contains(errorStr, "SQLITE_BUSY")) {
			// Exponential backoff with jitter
			backoff := min(time.Duration(1<<uint(attempt))*5*time.Millisecond, 100*time.Millisecond)
			time.Sleep(backoff)
			continue
		}

		return 0, fmt.Errorf("failed to reserve unique ID: %w", err)
	}

	return 0, fmt.Errorf("failed to allocate ID after %d attempts", maxRetries)
}

// attemptAllocateID performs a single attempt at allocating an ID
func (mdb *MeasurementDB) attemptAllocateID(timestamp time.Time) (int, error) {
	// Use a simpler, more atomic approach with a single query
	// This avoids multiple round trips and reduces lock contention
	var nextID int

	// Try to use a single atomic operation to get and increment the counter
	err := mdb.db.QueryRow(`
		INSERT INTO id_counter (id, next_id) 
		VALUES (1, 2) 
		ON CONFLICT(id) DO UPDATE SET next_id = next_id + 1 
		RETURNING next_id - 1
	`).Scan(&nextID)
	if err != nil {
		return 0, fmt.Errorf("failed to get next ID: %w", err)
	}

	// Insert the measurement record in a separate transaction
	// This reduces the time we hold locks
	_, err = mdb.db.Exec(`
		INSERT INTO measurements (unique_id, timestamp, status) 
		VALUES (?, ?, 'incomplete')
	`, nextID, timestamp)
	if err != nil {
		return 0, fmt.Errorf("failed to insert measurement: %w", err)
	}

	return nextID, nil
}

// CompleteMeasurement updates an incomplete measurement record with full details
func (mdb *MeasurementDB) CompleteMeasurement(uniqueID int, measurementTitle, filePath string, fileSize int64) error {
	query := `
	UPDATE measurements 
	SET measurement_title = ?, file_path = ?, file_size = ?, status = 'complete'
	WHERE unique_id = ? AND status = 'incomplete'
	`

	result, err := mdb.db.Exec(query, measurementTitle, filePath, fileSize, uniqueID)
	if err != nil {
		return fmt.Errorf("failed to complete measurement: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no incomplete measurement found with ID %d", uniqueID)
	}

	return nil
}

// GetMeasurement retrieves a measurement by unique ID
func (mdb *MeasurementDB) GetMeasurement(uniqueID int) (*MeasurementMetadata, error) {
	query := `
	SELECT unique_id, timestamp, measurement_title, file_path, file_size, status, created
	FROM measurements WHERE unique_id = ?
	`

	var m MeasurementMetadata
	var measurementTitle, filePath sql.NullString
	var fileSize sql.NullInt64

	err := mdb.db.QueryRow(query, uniqueID).Scan(
		&m.UniqueID, &m.Timestamp, &measurementTitle,
		&filePath, &fileSize, &m.Status, &m.Created,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("measurement not found: %d", uniqueID)
		}
		return nil, fmt.Errorf("failed to get measurement: %w", err)
	}

	// Handle nullable fields
	if measurementTitle.Valid {
		m.MeasurementTitle = measurementTitle.String
	}
	if filePath.Valid {
		m.FilePath = filePath.String
	}
	if fileSize.Valid {
		m.FileSize = fileSize.Int64
	}

	return &m, nil
}

// QueryMeasurements retrieves measurements based on filters
func (mdb *MeasurementDB) QueryMeasurements(filters MeasurementFilters) ([]*MeasurementMetadata, error) {
	query := "SELECT unique_id, timestamp, measurement_title, file_path, file_size, status, created FROM measurements WHERE 1=1"
	args := []any{}

	if filters.FromDate != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filters.FromDate)
	}
	if filters.ToDate != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filters.ToDate)
	}
	if filters.TitlePattern != "" {
		query += " AND measurement_title LIKE ?"
		args = append(args, "%"+filters.TitlePattern+"%")
	}
	if filters.DateString != "" {
		query += " AND date(timestamp) = ?"
		args = append(args, filters.DateString)
	}
	if filters.Status != "" {
		query += " AND status = ?"
		args = append(args, filters.Status)
	}

	query += " ORDER BY timestamp DESC"
	if filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
	}

	rows, err := mdb.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query measurements: %w", err)
	}
	defer rows.Close()

	var measurements []*MeasurementMetadata
	for rows.Next() {
		var m MeasurementMetadata
		var measurementTitle, filePath sql.NullString
		var fileSize sql.NullInt64

		err := rows.Scan(&m.UniqueID, &m.Timestamp, &measurementTitle,
			&filePath, &fileSize, &m.Status, &m.Created)
		if err != nil {
			return nil, fmt.Errorf("failed to scan measurement row: %w", err)
		}

		// Handle nullable fields
		if measurementTitle.Valid {
			m.MeasurementTitle = measurementTitle.String
		}
		if filePath.Valid {
			m.FilePath = filePath.String
		}
		if fileSize.Valid {
			m.FileSize = fileSize.Int64
		}

		measurements = append(measurements, &m)
	}

	return measurements, nil
}

// MeasurementFilters defines query filters for measurements
type MeasurementFilters struct {
	FromDate     *time.Time `json:"from_date,omitempty"`
	ToDate       *time.Time `json:"to_date,omitempty"`
	DateString   string     `json:"date_string,omitempty"` // Format: "2024-01-15"
	TitlePattern string     `json:"title_pattern,omitempty"`
	Status       string     `json:"status,omitempty"` // "incomplete", "complete"
	Limit        int        `json:"limit,omitempty"`
}
