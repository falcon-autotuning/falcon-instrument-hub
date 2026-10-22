package measurementdb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/hdf5data"

	_ "github.com/mattn/go-sqlite3"
)

func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func calculateNumPoints(shape []int) int {
	if len(shape) == 0 {
		return 0
	}

	total := 1

	for _, dim := range shape {
		if dim <= 0 {
			return 0
		}
		total *= dim
	}

	return total
}

type Manager struct {
	db        *sql.DB
	hdf5Dir   string
	hdf5Store *HDF5Store
	ownerName string
}

func Open(
	catalogPath string,
	hdf5Directory string,
	ownerName string,
) (*Manager, error) {
	if err := os.MkdirAll(hdf5Directory, 0755); err != nil {
		return nil, err
	}
	absHDF5, err := filepath.Abs(hdf5Directory)
	if err != nil {
		return nil, err
	}

	absCatalog, err := filepath.Abs(catalogPath)
	if err != nil {
		return nil, err
	}

	if info, err := os.Stat(absCatalog); err == nil && info.IsDir() {
		absCatalog = filepath.Join(absCatalog, "measurement.db")
	}

	db, err := sql.Open("sqlite3", absCatalog)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Manager{
		db:        db,
		hdf5Dir:   absHDF5,
		hdf5Store: NewHDF5Store(),
		ownerName: ownerName,
	}, nil
}

func (m *Manager) Close() error {
	if m == nil || m.db == nil {
		return nil
	}

	err := m.db.Close()
	m.db = nil

	return err
}

func createSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS measurements (
			run_id TEXT NOT NULL,
			measurement_id TEXT NOT NULL,
			recorded_at DATETIME NOT NULL,
			owner TEXT NOT NULL,
			measurement_name TEXT NOT NULL,
			file_path TEXT NOT NULL UNIQUE,
			shape_json TEXT NOT NULL,
			getters_json TEXT NOT NULL,
			setters_json TEXT NOT NULL,
			num_points INTEGER NOT NULL,
			PRIMARY KEY (run_id, measurement_id)
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		PRAGMA user_version = 1;
	`)
	if err != nil {
		return err
	}

	return nil
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)

	var b strings.Builder

	for _, r := range name {
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(unicode.ToLower(r))

		case unicode.IsDigit(r):
			b.WriteRune(r)

		case r == '-':
			b.WriteRune(r)

		case r == '_':
			b.WriteRune(r)

		case unicode.IsSpace(r):
			b.WriteRune('-')
		}
	}

	if b.Len() == 0 {
		return "measurement"
	}

	return b.String()
}

func (m *Manager) buildHDF5Path(
	title string,
	recordedAt time.Time,
) (string, error) {
	year := fmt.Sprintf("%04d", recordedAt.Year())
	month := fmt.Sprintf("%02d", int(recordedAt.Month()))
	day := fmt.Sprintf("%02d", recordedAt.Day())

	dir := filepath.Join(
		m.hdf5Dir,
		year,
		month,
		day,
	)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	uniqueID := recordedAt.UnixNano()

	filename := fmt.Sprintf(
		"%s-%d.h5",
		sanitizeFilename(title),
		uniqueID,
	)

	return filepath.Join(dir, filename), nil
}

func (m *Manager) decodeCatalogEntry(
	rows *sql.Rows,
) (CatalogEntry, error) {
	var entry CatalogEntry

	var (
		shapeJSON   string
		gettersJSON string
		settersJSON string
	)

	err := rows.Scan(
		&entry.RunID,
		&entry.MeasurementID,
		&entry.RecordedAt,
		&entry.Owner,
		&entry.MeasurementName,
		&entry.FilePath,
		&shapeJSON,
		&gettersJSON,
		&settersJSON,
		&entry.NumPoints,
	)
	if err != nil {
		return CatalogEntry{}, err
	}

	err = json.Unmarshal(
		[]byte(shapeJSON),
		&entry.Shape,
	)
	if err != nil {
		return CatalogEntry{}, err
	}

	err = json.Unmarshal(
		[]byte(gettersJSON),
		&entry.Getters,
	)
	if err != nil {
		return CatalogEntry{}, err
	}

	err = json.Unmarshal(
		[]byte(settersJSON),
		&entry.Setters,
	)
	if err != nil {
		return CatalogEntry{}, err
	}

	return entry, nil
}

func (m *Manager) Insert(
	insert InsertEntry,
	entry DatabaseEntry,
) (CatalogEntry, error) {
	recordedAt := time.Now().UTC()

	filePath, err := m.buildHDF5Path(
		entry.Title,
		recordedAt,
	)
	if err != nil {
		return CatalogEntry{}, err
	}

	if err := m.hdf5Store.Write(filePath, entry); err != nil {
		return CatalogEntry{}, err
	}

	shapeJSON, err := marshalJSON(
		insert.Shape,
	)
	if err != nil {
		return CatalogEntry{}, err
	}

	gettersJSON, err := marshalJSON(
		insert.Getters,
	)
	if err != nil {
		return CatalogEntry{}, err
	}

	settersJSON, err := marshalJSON(
		insert.Setters,
	)
	if err != nil {
		return CatalogEntry{}, err
	}

	_, err = m.db.Exec(`
	INSERT INTO measurements (
		run_id,
		measurement_id,
		recorded_at,
		owner,
		measurement_name,
		file_path,
		shape_json,
		getters_json,
		setters_json,
		num_points
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		insert.RunID,
		insert.MeasurementID,
		recordedAt,
		m.ownerName,
		insert.MeasurementName,
		filePath,
		shapeJSON,
		gettersJSON,
		settersJSON,
		calculateNumPoints(insert.Shape),
	)
	if err != nil {
		_ = os.Remove(filePath)

		return CatalogEntry{}, fmt.Errorf(
			"insert measurement catalog entry: %w",
			err,
		)
	}

	return CatalogEntry{
		InsertEntry: insert,

		Owner:      m.ownerName,
		FilePath:   filePath,
		RecordedAt: recordedAt,
		NumPoints: calculateNumPoints(
			insert.Shape,
		),
	}, nil
}

func (m *Manager) Search(
	query SearchQuery,
) ([]CatalogEntry, error) {
	sqlQuery := `
	SELECT
		run_id,
		measurement_id,
		recorded_at,
		owner,
		measurement_name,
		file_path,
		shape_json,
		getters_json,
		setters_json,
		num_points
	FROM measurements
	WHERE 1 = 1
`

	var args []any

	if query.RunID != "" {
		sqlQuery += " AND run_id = ?"
		args = append(args, query.RunID)
	}

	if query.MeasurementID != "" {
		sqlQuery += " AND measurement_id = ?"
		args = append(args, query.MeasurementID)
	}

	if query.Owner != "" {
		sqlQuery += " AND owner LIKE ?"
		args = append(
			args,
			"%"+query.Owner+"%",
		)
	}

	if query.MeasurementName != "" {
		sqlQuery += " AND measurement_name LIKE ?"
		args = append(
			args,
			"%"+query.MeasurementName+"%",
		)
	}

	if query.StartTime != nil {
		sqlQuery += " AND recorded_at >= ?"
		args = append(args, *query.StartTime)
	}

	if query.EndTime != nil {
		sqlQuery += " AND recorded_at <= ?"
		args = append(args, *query.EndTime)
	}

	sqlQuery += " ORDER BY recorded_at DESC"

	if query.Limit > 0 {
		sqlQuery += fmt.Sprintf(" LIMIT %d", query.Limit)
	}

	rows, err := m.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var output []CatalogEntry

	for rows.Next() {
		entry, err := m.decodeCatalogEntry(rows)
		if err != nil {
			return nil, err
		}

		output = append(output, entry)
	}

	return output, rows.Err()
}

func (r *ValidationReport) Healthy() bool {
	return len(r.MissingFiles) == 0 &&
		len(r.OrphanFiles) == 0 &&
		len(r.BadFiles) == 0
}

func (r *ValidationReport) Empty() bool {
	return r.Healthy()
}

func (r *ValidationReport) Corrupted() bool {
	return !r.Healthy()
}

func (m *Manager) Validate() (*ValidationReport, error) {
	report := &ValidationReport{}

	rows, err := m.db.Query(`
	SELECT
		run_id,
		measurement_id,
		recorded_at,
		owner,
		measurement_name,
		file_path,
		shape_json,
		getters_json,
		setters_json,
		num_points
	FROM measurements
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	catalogFiles := make(map[string]struct{})

	for rows.Next() {
		entry, err := m.decodeCatalogEntry(rows)
		if err != nil {
			return nil, err
		}

		catalogFiles[entry.FilePath] = struct{}{}

		if _, err := os.Stat(entry.FilePath); err != nil {
			if os.IsNotExist(err) {
				report.MissingFiles = append(
					report.MissingFiles,
					entry,
				)
				continue
			}

			return nil, err
		}

		hdf5, err := hdf5data.NewFromFile(
			entry.FilePath,
		)
		if err != nil {
			report.BadFiles = append(
				report.BadFiles,
				entry.FilePath,
			)
			continue
		}

		hdf5.Close()
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	err = filepath.WalkDir(
		m.hdf5Dir,
		func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}

			if filepath.Ext(path) != ".h5" {
				return nil
			}

			if _, ok := catalogFiles[path]; ok {
				return nil
			}

			report.OrphanFiles = append(
				report.OrphanFiles,
				path,
			)

			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return report, nil
}
