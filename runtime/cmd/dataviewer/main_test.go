package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func copyDemoDataset(t *testing.T) string {
	t.Helper()
	destination := t.TempDir()
	for _, directory := range []string{"averaged", "raw"} {
		source := filepath.Join("..", "..", "..", "test_data", "demo_measurements", directory)
		files, err := os.ReadDir(source)
		require.NoError(t, err, "checked-in demo dataset must exist")
		require.NoError(t, os.Mkdir(filepath.Join(destination, directory), 0755))
		for _, file := range files {
			data, err := os.ReadFile(filepath.Join(source, file.Name()))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(destination, directory, file.Name()), data, 0600))
		}
	}
	return destination
}

func TestDemoDatasetPlotsAfterRelocation(t *testing.T) {
	dataDir := copyDemoDataset(t)
	for _, mode := range []string{"relative", "existing_absolute", "moved_absolute"} {
		t.Run(mode, func(t *testing.T) {
			srv, err := newServer(dataDir)
			require.NoError(t, err)
			legacyRoot := filepath.Join(t.TempDir(), "missing-original-dataset")
			if mode != "relative" {
				root := dataDir
				if mode == "moved_absolute" {
					root = legacyRoot
				}
				for id, idx := range srv.index {
					idx.FilePath = filepath.Join(root, idx.FilePath)
					if idx.RawDataRef != nil {
						idx.RawDataRef.RawFilePath = filepath.Join(root, idx.RawDataRef.RawFilePath)
					}
					srv.index[id] = idx
				}
			}
			list := httptest.NewRecorder()
			srv.handleListMeasurements(list, httptest.NewRequest(http.MethodGet, "/api/measurements", nil))
			require.Equal(t, http.StatusOK, list.Code)
			var summaries []measurementSummary
			require.NoError(t, json.Unmarshal(list.Body.Bytes(), &summaries))
			var ids []string
			for _, summary := range summaries {
				ids = append(ids, summary.ID)
			}
			require.Equal(t, []string{"averaged-sweep-001", "axis-sweep-001", "dc-collection-001", "sweep-2d-001"}, ids)
			for _, id := range ids {
				t.Run(id, func(t *testing.T) {
					response := httptest.NewRecorder()
					srv.handleGetPlotData(response, httptest.NewRequest(http.MethodGet, "/api/measurements/"+id+"/plot", nil))
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
					switch id {
					case "averaged-sweep-001":
						var plot plotData
						require.NoError(t, json.Unmarshal(response.Body.Bytes(), &plot))
						assert.Equal(t, id, plot.MeasurementID)
						assert.Len(t, plot.Voltages, 151)
						assert.NotEmpty(t, plot.Averaged)
						require.Len(t, plot.RawTraces, 12)
						assert.NotEmpty(t, plot.RawTraces[0].Channels)
					case "axis-sweep-001":
						var plot plotDataAxisSweep
						require.NoError(t, json.Unmarshal(response.Body.Bytes(), &plot))
						assert.Equal(t, id, plot.MeasurementID)
						assert.Equal(t, "axis_sweep", plot.MeasurementType)
						assert.NotEmpty(t, plot.Sweeps)
					case "dc-collection-001":
						var plot plotDataDC
						require.NoError(t, json.Unmarshal(response.Body.Bytes(), &plot))
						assert.Equal(t, id, plot.MeasurementID)
						assert.Equal(t, "dc_collection", plot.MeasurementType)
						assert.Len(t, plot.Points, 5)
					case "sweep-2d-001":
						var plot plotData2D
						require.NoError(t, json.Unmarshal(response.Body.Bytes(), &plot))
						assert.Equal(t, id, plot.MeasurementID)
						assert.Equal(t, "2d", plot.MeasurementType)
						assert.NotEmpty(t, plot.XVoltages)
						assert.NotEmpty(t, plot.YVoltages)
						assert.NotEmpty(t, plot.ChannelData)
					}
				})
			}
		})
	}
}

func TestMissingOptionalRawDataStillReturnsAveragedPlot(t *testing.T) {
	srv, err := newServer(copyDemoDataset(t))
	require.NoError(t, err)
	idx := srv.index["averaged-sweep-001"]
	idx.RawDataRef.RawFilePath = "raw/missing.json"
	response := httptest.NewRecorder()
	srv.handleGetPlotData(response, httptest.NewRequest(http.MethodGet, "/api/measurements/averaged-sweep-001/plot", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var plot plotData
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &plot))
	assert.NotEmpty(t, plot.Averaged)
	assert.Empty(t, plot.RawTraces)
}

func TestDatasetReadDoesNotMaskNonMissingFileErrors(t *testing.T) {
	srv, err := newServer(copyDemoDataset(t))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "sweep_averaged-sweep-001.json")
	require.NoError(t, os.Mkdir(path, 0755))
	_, err = srv.readDatasetFile(path, "averaged")
	require.Error(t, err, "an existing unreadable path must not be replaced with a different dataset file")
}

func TestViewerRejectsMissingIndexAndUnknownMeasurement(t *testing.T) {
	_, err := newServer(t.TempDir())
	require.Error(t, err)
	srv, err := newServer(copyDemoDataset(t))
	require.NoError(t, err)
	response := httptest.NewRecorder()
	srv.handleGetPlotData(response, httptest.NewRequest(http.MethodGet, "/api/measurements/missing/plot", nil))
	assert.Equal(t, http.StatusNotFound, response.Code)
}
