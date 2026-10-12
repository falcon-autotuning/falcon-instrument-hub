package measurementdb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeFilename_Empty(t *testing.T) {
	assert.Equal(
		t,
		"measurement",
		sanitizeFilename(""),
	)
}

func TestSanitizeFilename_Whitespace(t *testing.T) {
	assert.Equal(
		t,
		"measurement",
		sanitizeFilename("   "),
	)
}

func TestSanitizeFilename_Spaces(t *testing.T) {
	assert.Equal(
		t,
		"charge-scan",
		sanitizeFilename("Charge Scan"),
	)
}

func TestSanitizeFilename_InvalidCharacters(t *testing.T) {
	assert.Equal(
		t,
		"abc123",
		sanitizeFilename("abc!@#$123"),
	)
}

func TestSanitizeFilename_PreservesDashUnderscore(t *testing.T) {
	assert.Equal(
		t,
		"charge_scan-1",
		sanitizeFilename("charge_scan-1"),
	)
}

func TestValidationReport_Healthy(t *testing.T) {
	r := &ValidationReport{}

	assert.True(t, r.Healthy())
	assert.True(t, r.Empty())
	assert.False(t, r.Corrupted())
}

func TestValidationReport_Corrupted(t *testing.T) {
	r := &ValidationReport{
		BadFiles: []string{"bad.h5"},
	}

	assert.False(t, r.Healthy())
	assert.False(t, r.Empty())
	assert.True(t, r.Corrupted())
}

func TestOpen_Close(t *testing.T) {
	dir := t.TempDir()

	mgr, err := Open(
		filepath.Join(dir, "catalog.sqlite"),
		filepath.Join(dir, "hdf5"),
		"owner",
	)

	var version int

	err = mgr.db.QueryRow(
		"PRAGMA user_version",
	).Scan(&version)

	require.NoError(t, err)

	assert.Equal(
		t,
		1,
		version,
	)

	require.NoError(t, err)
	require.NotNil(t, mgr)

	assert.NoError(t, mgr.Close())
	assert.Nil(t, mgr.db)
}

func TestManager_Close_Nil(t *testing.T) {
	var manager *Manager

	assert.NoError(t, manager.Close())
}

func TestManager_Close_NilDB(t *testing.T) {
	manager := &Manager{}

	assert.NoError(t, manager.Close())
}

func TestBuildHDF5Path(t *testing.T) {
	root := t.TempDir()

	manager := &Manager{
		hdf5Dir: root,
	}

	ts := time.Date(
		2026,
		time.October,
		12,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	path, err := manager.buildHDF5Path(
		"Charge Scan",
		ts,
	)

	require.NoError(t, err)

	assert.Contains(
		t,
		path,
		filepath.Join(
			"2026",
			"10",
			"12",
		),
	)

	assert.Contains(
		t,
		filepath.Base(path),
		"charge-scan",
	)
}

func TestCalculateNumPoints_Empty(
	t *testing.T,
) {
	assert.Equal(
		t,
		0,
		calculateNumPoints(nil),
	)
}

func TestCalculateNumPoints_OneDimensional(
	t *testing.T,
) {
	assert.Equal(
		t,
		100,
		calculateNumPoints(
			[]int{100},
		),
	)
}

func TestCalculateNumPoints_TwoDimensional(
	t *testing.T,
) {
	assert.Equal(
		t,
		20000,
		calculateNumPoints(
			[]int{100, 200},
		),
	)
}

func TestCalculateNumPoints_InvalidDimension(
	t *testing.T,
) {
	assert.Equal(
		t,
		0,
		calculateNumPoints(
			[]int{100, 0},
		),
	)
}

func TestMarshalJSON(
	t *testing.T,
) {
	out, err := marshalJSON(
		[]int{1, 2, 3},
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		"[1,2,3]",
		out,
	)
}
