package measurementdb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetGlobal() {
	_ = Close()
}

func TestStartup_Success(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	err := Startup(
		filepath.Join(root, "catalog.sqlite"),
		filepath.Join(root, "hdf5"),
		"owner",
	)

	require.NoError(t, err)

	manager, err := Instance()

	require.NoError(t, err)
	require.NotNil(t, manager)
}

func TestStartup_AlreadyStarted(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	err := Startup(
		filepath.Join(root, "catalog.sqlite"),
		filepath.Join(root, "hdf5"),
		"owner",
	)
	require.NoError(t, err)

	err = Startup(
		filepath.Join(root, "catalog2.sqlite"),
		filepath.Join(root, "hdf5"),
		"owner",
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"already started",
	)
}

func TestInstance_NotStarted(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	manager, err := Instance()

	require.Error(t, err)
	assert.Nil(t, manager)
}

func TestInstance_Started(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	require.NoError(
		t,
		Startup(
			filepath.Join(root, "catalog.sqlite"),
			filepath.Join(root, "hdf5"),
			"owner",
		),
	)

	manager, err := Instance()

	require.NoError(t, err)
	require.NotNil(t, manager)
}

func TestClose_NotStarted(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	require.NoError(
		t,
		Close(),
	)
}

func TestClose_Started(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	require.NoError(
		t,
		Startup(
			filepath.Join(root, "catalog.sqlite"),
			filepath.Join(root, "hdf5"),
			"owner",
		),
	)

	require.NoError(
		t,
		Close(),
	)

	_, err := Instance()

	require.Error(t, err)
}

func TestSearch_NotStarted(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	results, err := Search(
		SearchQuery{},
	)

	require.Error(t, err)
	assert.Nil(t, results)
}

func TestSearch_Started(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	require.NoError(
		t,
		Startup(
			filepath.Join(root, "catalog.sqlite"),
			filepath.Join(root, "hdf5"),
			"owner",
		),
	)

	manager, err := Instance()
	require.NoError(t, err)

	insertTestMeasurement(
		t,
		manager,
		"run1",
		"m1",
	)

	results, err := Search(
		SearchQuery{
			CoreEntry: CoreEntry{
				MeasurementID: "m1",
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestInsert_NotStarted(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	_, err := Insert(
		InsertEntry{},
		testDatabaseEntry(),
	)

	require.Error(t, err)
}

func TestInsert_Started(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	require.NoError(
		t,
		Startup(
			filepath.Join(root, "catalog.sqlite"),
			filepath.Join(root, "hdf5"),
			"owner",
		),
	)

	_, err := Insert(
		InsertEntry{
			CoreEntry: CoreEntry{
				RunID:           "run1",
				MeasurementID:   "m1",
				MeasurementName: "test",
			},
			Shape: []int{2},
		},
		testDatabaseEntry(),
	)

	require.NoError(t, err)

	results, err := Search(
		SearchQuery{
			CoreEntry: CoreEntry{
				MeasurementID: "m1",
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestValidate_NotStarted(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	report, err := Validate()

	require.Error(t, err)
	assert.Nil(t, report)
}

func TestValidate_Started(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	require.NoError(
		t,
		Startup(
			filepath.Join(root, "catalog.sqlite"),
			filepath.Join(root, "hdf5"),
			"owner",
		),
	)

	report, err := Validate()

	require.NoError(t, err)
	require.NotNil(t, report)

	assert.True(
		t,
		report.Healthy(),
	)
}

func TestStartup_RegistersGlobalManager(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	err := Startup(
		filepath.Join(root, "catalog.sqlite"),
		filepath.Join(root, "hdf5"),
		"owner",
	)

	require.NoError(t, err)

	manager, err := Instance()

	require.NoError(t, err)
	require.NotNil(t, manager)

	assert.Equal(
		t,
		"owner",
		manager.ownerName,
	)
}

func TestInsertAsync_NotStarted(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	err := InsertAsync(
		InsertEntry{},
		testDatabaseEntry(),
	)

	require.Error(t, err)
}

func TestInsertAsync_Started(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	require.NoError(
		t,
		Startup(
			filepath.Join(root, "catalog.sqlite"),
			filepath.Join(root, "hdf5"),
			"owner",
		),
	)

	err := InsertAsync(
		InsertEntry{
			CoreEntry: CoreEntry{
				RunID:           "run1",
				MeasurementID:   "m1",
				MeasurementName: "test",
			},
			Shape: []int{2},
		},
		testDatabaseEntry(),
	)

	require.NoError(t, err)

	require.Eventually(
		t,
		func() bool {
			results, err := Search(
				SearchQuery{
					CoreEntry: CoreEntry{
						MeasurementID: "m1",
					},
				},
			)

			return err == nil &&
				len(results) == 1
		},
		time.Second,
		10*time.Millisecond,
	)
}

func TestClose_DrainsQueue(
	t *testing.T,
) {
	resetGlobal()
	defer resetGlobal()

	root := t.TempDir()

	require.NoError(
		t,
		Startup(
			filepath.Join(root, "catalog.sqlite"),
			filepath.Join(root, "hdf5"),
			"owner",
		),
	)

	require.NoError(
		t,
		InsertAsync(
			InsertEntry{
				CoreEntry: CoreEntry{
					RunID:           "run1",
					MeasurementID:   "m1",
					MeasurementName: "test",
				},
				Shape: []int{2},
			},
			testDatabaseEntry(),
		),
	)

	require.NoError(
		t,
		Close(),
	)

	resetGlobal()

	require.NoError(
		t,
		Startup(
			filepath.Join(root, "catalog.sqlite"),
			filepath.Join(root, "hdf5"),
			"owner",
		),
	)

	results, err := Search(
		SearchQuery{
			CoreEntry: CoreEntry{
				MeasurementID: "m1",
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, results, 1)
}
