package measurementdb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

func testInsertEntry(
	runID string,
	measurementID string,
) InsertEntry {
	return InsertEntry{
		CoreEntry: CoreEntry{
			RunID:           runID,
			MeasurementID:   measurementID,
			MeasurementName: measurementID,
		},
		Shape: []int{2},
	}
}

func insertTestMeasurement(
	t *testing.T,
	manager *Manager,
	runID string,
	measurementID string,
) {
	t.Helper()

	_, err := manager.Insert(
		testInsertEntry(
			runID,
			measurementID,
		),
		testDatabaseEntry(),
	)

	require.NoError(t, err)
}

func newTestManager(
	t *testing.T,
) *Manager {
	t.Helper()

	root := t.TempDir()

	manager, err := Open(
		filepath.Join(root, "catalog.sqlite"),
		filepath.Join(root, "hdf5"),
		"test-owner",
	)
	require.NoError(t, err)

	return manager
}

func TestSearch_RunID(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	insertTestMeasurement(
		t,
		manager,
		"run1",
		"m1",
	)

	insertTestMeasurement(
		t,
		manager,
		"run2",
		"m2",
	)

	results, err := manager.Search(
		SearchQuery{
			CoreEntry: CoreEntry{
				RunID: "run1",
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(
		t,
		"run1",
		results[0].RunID,
	)
}

func TestSearch_MeasurementName(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	_, err := manager.Insert(
		InsertEntry{
			CoreEntry: CoreEntry{
				RunID:           "run1",
				MeasurementID:   "m1",
				MeasurementName: "Charge Scan",
			},
			Shape: []int{2},
		},
		testDatabaseEntry(),
	)
	require.NoError(t, err)

	_, err = manager.Insert(
		InsertEntry{
			CoreEntry: CoreEntry{
				RunID:           "run1",
				MeasurementID:   "m2",
				MeasurementName: "Pinch Off Scan",
			},
			Shape: []int{2},
		},
		testDatabaseEntry(),
	)
	require.NoError(t, err)

	results, err := manager.Search(
		SearchQuery{
			CoreEntry: CoreEntry{
				MeasurementName: "Charge",
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Contains(
		t,
		results[0].MeasurementName,
		"Charge",
	)
}

func TestSearch_Limit(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	insertTestMeasurement(t, manager, "run1", "m1")
	insertTestMeasurement(t, manager, "run1", "m2")
	insertTestMeasurement(t, manager, "run1", "m3")

	results, err := manager.Search(
		SearchQuery{
			Limit: 2,
		},
	)

	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestInsert_DuplicateMeasurementID(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	insertTestMeasurement(
		t,
		manager,
		"run1",
		"duplicate",
	)

	_, err := manager.Insert(
		InsertEntry{
			CoreEntry: CoreEntry{
				RunID:           "run1",
				MeasurementID:   "duplicate",
				MeasurementName: "duplicate",
			},
			Shape: []int{2},
		},
		testDatabaseEntry(),
	)
	require.Error(t, err)
}

func TestValidate_Healthy(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	insertTestMeasurement(
		t,
		manager,
		"run1",
		"m1",
	)

	report, err := manager.Validate()

	require.NoError(t, err)

	assert.Empty(t, report.MissingFiles)
	assert.Empty(t, report.OrphanFiles)
	assert.Empty(t, report.BadFiles)

	assert.True(t, report.Healthy())
	assert.False(t, report.Corrupted())
}

func TestValidate_MissingFile(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	insertTestMeasurement(
		t,
		manager,
		"run1",
		"m1",
	)

	results, err := manager.Search(
		SearchQuery{
			CoreEntry: CoreEntry{
				MeasurementID: "m1",
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 1)

	require.NoError(
		t,
		os.Remove(results[0].FilePath),
	)

	report, err := manager.Validate()

	require.NoError(t, err)

	require.Len(t, report.MissingFiles, 1)
	assert.True(t, report.Corrupted())
}

func TestValidate_OrphanFile(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	nowPath := filepath.Join(
		manager.hdf5Dir,
		"2026",
		"10",
		"12",
	)

	require.NoError(
		t,
		os.MkdirAll(nowPath, 0755),
	)

	orphan := filepath.Join(
		nowPath,
		"orphan.h5",
	)

	require.NoError(
		t,
		os.WriteFile(
			orphan,
			[]byte("orphan"),
			0644,
		),
	)

	report, err := manager.Validate()

	require.NoError(t, err)

	require.Len(t, report.OrphanFiles, 1)
	assert.Equal(t, orphan, report.OrphanFiles[0])
}

func TestValidate_BadFile(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	badFile := filepath.Join(
		manager.hdf5Dir,
		"bad.h5",
	)

	require.NoError(
		t,
		os.WriteFile(
			badFile,
			[]byte("not hdf5"),
			0644,
		),
	)

	shapeJSON, err := marshalJSON(
		[]int{1},
	)
	require.NoError(t, err)

	gettersJSON, err := marshalJSON(
		[]GetterInfo{},
	)
	require.NoError(t, err)

	settersJSON, err := marshalJSON(
		[]SweepInfo{},
	)
	require.NoError(t, err)

	_, err = manager.db.Exec(`
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
	) VALUES (?, ?, datetime('now'), ?, ?, ?, ?, ?, ?, ?)
`,
		"run1",
		"bad",
		"owner",
		"bad file",
		badFile,
		shapeJSON,
		gettersJSON,
		settersJSON,
		1,
	)

	require.NoError(t, err)

	report, err := manager.Validate()

	require.NoError(t, err)

	require.Len(t, report.BadFiles, 1)
	assert.Equal(t, badFile, report.BadFiles[0])
}

func TestInsert_ReturnsCatalogEntry(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	inserted, err := manager.Insert(
		InsertEntry{
			CoreEntry: CoreEntry{
				RunID:           "run1",
				MeasurementID:   "m1",
				MeasurementName: "Charge Scan",
			},
			Shape: []int{2},
		},
		testDatabaseEntry(),
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		"run1",
		inserted.RunID,
	)

	assert.Equal(
		t,
		"m1",
		inserted.MeasurementID,
	)

	assert.Equal(
		t,
		"test-owner",
		inserted.Owner,
	)

	assert.NotEmpty(
		t,
		inserted.FilePath,
	)

	assert.False(
		t,
		inserted.RecordedAt.IsZero(),
	)
}

func TestDecodeCatalogEntry_JSONRoundTrip(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	expected := InsertEntry{
		CoreEntry: CoreEntry{
			RunID:           "run1",
			MeasurementID:   "m1",
			MeasurementName: "test",
		},
		Shape: []int{10, 20},
		Getters: []GetterInfo{
			{
				Unit: falconcore.Volt,
			},
		},
	}

	inserted, err := manager.Insert(
		expected,
		testDatabaseEntry(),
	)

	require.NoError(t, err)

	results, err := manager.Search(
		SearchQuery{
			CoreEntry: CoreEntry{
				MeasurementID: "m1",
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(
		t,
		inserted.Shape,
		results[0].Shape,
	)

	assert.Equal(
		t,
		inserted.Getters,
		results[0].Getters,
	)

	assert.Equal(
		t,
		inserted.NumPoints,
		results[0].NumPoints,
	)
}

func TestInsert_ComputesNumPoints(
	t *testing.T,
) {
	manager := newTestManager(t)
	defer manager.Close()

	inserted, err := manager.Insert(
		InsertEntry{
			CoreEntry: CoreEntry{
				RunID:           "run1",
				MeasurementID:   "m1",
				MeasurementName: "test",
			},
			Shape: []int{10, 20},
		},
		testDatabaseEntry(),
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		200,
		inserted.NumPoints,
	)
}
