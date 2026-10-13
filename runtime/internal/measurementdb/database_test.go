//go:build cgo

package measurementdb_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/autotuner-interfaces/contexts/acquisitioncontext"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/hdf5data"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdb"
)

func TestDatabaseInitializesAndIndexesHDF5Measurement(t *testing.T) {
	temporaryDirectory := t.TempDir()
	catalogPath := filepath.Join(temporaryDirectory, "measurements.sqlite")
	const runID = "test-run"

	if err := measurementdb.Initialize(catalogPath, runID); err != nil {
		t.Fatalf("Initialize() error: %v", err)
	}
	t.Cleanup(func() {
		if err := measurementdb.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	})

	if _, err := os.Stat(catalogPath); err != nil {
		t.Fatalf("SQLite catalog was not created: %v", err)
	}

	getterConnection, err := connection.NewOhmic("O1")
	if err != nil {
		t.Fatalf("create getter connection: %v", err)
	}
	defer getterConnection.Close()
	volts, err := symbolunit.NewVolt()
	if err != nil {
		t.Fatalf("create voltage unit: %v", err)
	}
	defer volts.Close()
	context, err := acquisitioncontext.New(
		getterConnection,
		instrument.Voltmeter,
		volts,
	)
	if err != nil {
		t.Fatalf("create acquisition context: %v", err)
	}
	defer context.Close()

	record := measurementdb.Record{
		MeasurementID:   "measurement-1",
		MeasurementName: "measure_set_get",
		Getter:          "Meter1.analog.1.voltage",
		Raw:             []float64{1.0, 2.0, 3.0},
		Unit:            "V",
	}
	if err := measurementdb.Save(record, context); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	hdf5Directory := filepath.Join(temporaryDirectory, "hdf5")
	files, err := os.ReadDir(hdf5Directory)
	if err != nil {
		t.Fatalf("read HDF5 directory: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("HDF5 file count = %d, want 1", len(files))
	}
	hdf5Path := filepath.Join(hdf5Directory, files[0].Name())
	storedData, err := hdf5data.NewFromFile(hdf5Path)
	if err != nil {
		t.Fatalf("open written HDF5 file: %v", err)
	}
	defer storedData.Close()

	if err := measurementdb.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	database, err := sql.Open("sqlite3", catalogPath)
	if err != nil {
		t.Fatalf("open SQLite catalog: %v", err)
	}
	defer database.Close()

	var (
		storedRunID           string
		storedMeasurementID   string
		storedRecordedAt      string
		storedMeasurementName string
		storedFilePath        string
		storedGetter          string
		storedUnit            string
		storedSampleCount     int
	)
	err = database.QueryRow(`SELECT
		run_id, measurement_id, recorded_at, measurement_name, file_path,
		getter, unit, sample_count
		FROM measurements`).Scan(
		&storedRunID,
		&storedMeasurementID,
		&storedRecordedAt,
		&storedMeasurementName,
		&storedFilePath,
		&storedGetter,
		&storedUnit,
		&storedSampleCount,
	)
	if err != nil {
		t.Fatalf("query measurement index: %v", err)
	}

	if storedRunID != runID ||
		storedMeasurementID != record.MeasurementID ||
		storedMeasurementName != record.MeasurementName ||
		storedFilePath != hdf5Path ||
		storedGetter != record.Getter ||
		storedUnit != record.Unit ||
		storedSampleCount != len(record.Raw) {
		t.Fatalf(
			"stored index = {%q %q %q %q %q %q %d}, want {%q %q %q %q %q %q %d}",
			storedRunID,
			storedMeasurementID,
			storedMeasurementName,
			storedFilePath,
			storedGetter,
			storedUnit,
			storedSampleCount,
			runID,
			record.MeasurementID,
			record.MeasurementName,
			hdf5Path,
			record.Getter,
			record.Unit,
			len(record.Raw),
		)
	}
	if _, err := time.Parse(time.RFC3339Nano, storedRecordedAt); err != nil {
		t.Fatalf("stored timestamp %q is invalid: %v", storedRecordedAt, err)
	}
}
