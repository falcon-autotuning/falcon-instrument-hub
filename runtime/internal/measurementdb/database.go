//go:build cgo

// Package measurementdb stores raw measurements in individual HDF5 files and
// indexes them in a SQLite catalog. This prototype has one store per hub process.
package measurementdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/autotuner-interfaces/contexts/acquisitioncontext"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/hdf5data"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listlabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringstring"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescontrolarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescoupledlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesint"
)

var (
	mu        sync.Mutex
	catalog   *sql.DB
	directory string
	runID     string
)

// Raw contains the samples in the original ISS unit. Averages are not persisted.
type Record struct {
	MeasurementID   string
	MeasurementName string
	Getter          string
	Raw             []float64
	Unit            string
}

// Initialize opens a persistent catalog and prepares the HDF5 directory.
func Initialize(path, run string) error {
	mu.Lock()
	defer mu.Unlock()
	if catalog != nil {
		return fmt.Errorf("measurement database is already initialized")
	}
	if run == "" {
		return fmt.Errorf("measurement run ID is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	files := filepath.Join(filepath.Dir(absolute), "hdf5")
	if err := os.MkdirAll(files, 0755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite3", absolute)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS measurements (
		id INTEGER PRIMARY KEY,
		run_id TEXT NOT NULL,
		measurement_id TEXT NOT NULL,
		recorded_at TEXT NOT NULL,
		measurement_name TEXT NOT NULL,
		file_path TEXT NOT NULL UNIQUE,
		getter TEXT NOT NULL,
		unit TEXT NOT NULL,
		sample_count INTEGER NOT NULL,
		UNIQUE (run_id, measurement_id)
	)`)
	if err != nil {
		db.Close()
		return fmt.Errorf("initialize measurement catalog: %w", err)
	}
	catalog, directory, runID = db, files, run
	return nil
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if catalog == nil {
		return nil
	}
	err := catalog.Close()
	catalog = nil
	directory, runID = "", ""
	return err
}

// Save writes one raw numeric HDF5 array, then inserts its catalog entry.
// The supplied context labels the raw samples, including their original unit.
func Save(record Record, context *acquisitioncontext.Handle) error {
	mu.Lock()
	defer mu.Unlock()
	if catalog == nil {
		return fmt.Errorf("measurement database is not initialized")
	}
	if len(record.Raw) == 0 || context == nil {
		return fmt.Errorf("measurement requires raw samples and an acquisition context")
	}
	if record.MeasurementID == "" || record.MeasurementName == "" {
		return fmt.Errorf("measurement ID and name are required")
	}
	recordedAt := time.Now().UTC()
	data, err := rawMeasurement(record, context, recordedAt)
	if err != nil {
		return err
	}
	defer data.Close()

	// Reserve a unique path so Falcon's truncating writer cannot overwrite an
	// earlier measurement. Only our newly created, incomplete file is removed.
	file, err := os.CreateTemp(directory, "measurement-*.h5")
	if err != nil {
		return err
	}
	name := file.Name()
	complete := false
	defer func() {
		if !complete {
			os.Remove(name)
		}
	}()
	if err := file.Close(); err != nil {
		return err
	}
	if err := data.ToFile(name); err != nil {
		return err
	}
	// ToFile wraps a void C API, so verify the file before cataloging it.
	check, err := hdf5data.NewFromFile(name)
	if err != nil {
		return fmt.Errorf("read saved HDF5 measurement: %w", err)
	}
	defer check.Close()
	equal, err := check.Equal(data)
	if err != nil {
		return err
	}
	if !equal {
		return fmt.Errorf("saved HDF5 file does not match the raw measurement")
	}
	complete = true

	_, err = catalog.Exec(`INSERT INTO measurements
		(run_id, measurement_id, recorded_at, measurement_name, file_path,
		 getter, unit, sample_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, record.MeasurementID, recordedAt.Format(time.RFC3339Nano),
		record.MeasurementName, name, record.Getter, record.Unit, len(record.Raw))
	if err != nil {
		// Keep successfully written raw data if the separate SQL write fails.
		return fmt.Errorf("catalog insert failed; raw data retained at %q: %w", name, err)
	}
	return nil
}

func rawMeasurement(
	record Record,
	context *acquisitioncontext.Handle,
	recordedAt time.Time,
) (*hdf5data.Handle, error) {
	array, err := farraydouble.FromData(record.Raw, []uint64{uint64(len(record.Raw))})
	if err != nil {
		return nil, err
	}
	defer array.Close()
	labelled, err := labelledmeasuredarray.FromFArray(array, context)
	if err != nil {
		return nil, err
	}
	defer labelled.Close()
	list, err := listlabelledmeasuredarray.New([]*labelledmeasuredarray.Handle{labelled})
	if err != nil {
		return nil, err
	}
	defer list.Close()
	ranges, err := labelledarrayslabelledmeasuredarray.NewFromList(list)
	if err != nil {
		return nil, err
	}
	defer ranges.Close()
	// No sweep axes or sample timestamps are inferred for this set/get record.
	// The raw array retains its own [N] shape in /ranges/range0/data.
	shape, err := axesint.NewEmpty()
	if err != nil {
		return nil, err
	}
	defer shape.Close()
	domains, err := axescontrolarray.NewEmpty()
	if err != nil {
		return nil, err
	}
	defer domains.Close()
	labels, err := axescoupledlabelleddomain.NewEmpty()
	if err != nil {
		return nil, err
	}
	defer labels.Close()
	metadata, err := mapstringstring.NewEmpty()
	if err != nil {
		return nil, err
	}
	defer metadata.Close()
	for key, value := range map[string]string{
		"run_id": runID, "measurement_id": record.MeasurementID,
		"measurement_name": record.MeasurementName,
		"recorded_at":      recordedAt.Format(time.RFC3339Nano),
		"getter":           record.Getter, "unit": record.Unit,
		"sample_count": strconv.Itoa(len(record.Raw)),
	} {
		if err := metadata.Insert(key, value); err != nil {
			return nil, err
		}
	}
	return hdf5data.New(shape, domains, labels, ranges, metadata,
		record.MeasurementName, 0, int32(recordedAt.Unix()))
}
