// Command dataviewer starts a local HTTP server for plotting raw and averaged
// measurement data from the FALCon two-database JSON storage.
//
// Usage:
//
//	go run ./cmd/dataviewer --data-dir ../../test-outs/data/dummy_measurement
//	# then open http://localhost:8089 in a browser
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Embedded frontend
// ---------------------------------------------------------------------------

//go:embed frontend
var frontendFS embed.FS

// ---------------------------------------------------------------------------
// On-disk JSON shapes (mirrors serverinterpreter types, kept minimal for the
// viewer so we avoid importing the internal package from a separate main).
// ---------------------------------------------------------------------------

type tracePoint struct {
	Voltage      float64            `json:"voltage"`
	Measurements map[string]float64 `json:"measurements"`
}

type trace struct {
	SweepIndex int          `json:"sweep_index"`
	Points     []tracePoint `json:"points"`
	Timestamp  time.Time    `json:"timestamp"`
}

type averagedTrace struct {
	Points []tracePoint `json:"points"`
}

type rawDataRef struct {
	MeasurementID string `json:"measurement_id"`
	RawFilePath   string `json:"raw_file_path"`
	NumTraces     int    `json:"num_traces"`
	NumPoints     int    `json:"num_points_per_trace"`
}

type averagedRecord struct {
	MeasurementID string        `json:"measurement_id"`
	SweepGate     string        `json:"sweep_gate"`
	StartVoltage  float64       `json:"start_voltage"`
	StopVoltage   float64       `json:"stop_voltage"`
	NumPoints     int           `json:"num_points"`
	NumSweeps     int           `json:"num_sweeps"`
	AveragedTrace averagedTrace `json:"averaged_trace"`
	RawRef        *rawDataRef   `json:"raw_data_ref,omitempty"`
}

type rawRecord struct {
	MeasurementID string   `json:"measurement_id"`
	SweepGate     string   `json:"sweep_gate"`
	StartVoltage  float64  `json:"start_voltage"`
	StopVoltage   float64  `json:"stop_voltage"`
	NumTraces     int      `json:"num_traces"`
	NumPoints     int      `json:"num_points"`
	Traces        []trace  `json:"traces"`
	Channels      []string `json:"channels"`
}

type measurementIndex struct {
	MeasurementID string      `json:"measurement_id"`
	FilePath      string      `json:"file_path"`
	SweepGate     string      `json:"sweep_gate"`
	NumPoints     int         `json:"num_points"`
	NumSweeps     int         `json:"num_sweeps"`
	StoredAt      time.Time   `json:"stored_at"`
	RawDataRef    *rawDataRef `json:"raw_data_ref,omitempty"`
}

// ---------------------------------------------------------------------------
// API response types
// ---------------------------------------------------------------------------

type measurementSummary struct {
	ID        string `json:"id"`
	SweepGate string `json:"sweep_gate"`
	NumPoints int    `json:"num_points"`
	NumSweeps int    `json:"num_sweeps"`
	HasRaw    bool   `json:"has_raw"`
}

type plotData struct {
	Voltages     []float64          `json:"voltages"`
	Averaged     map[string][]float64 `json:"averaged"`      // channel -> values
	RawTraces    []rawTraceData     `json:"raw_traces"`
	SweepGate    string             `json:"sweep_gate"`
	NumSweeps    int                `json:"num_sweeps"`
	MeasurementID string           `json:"measurement_id"`
}

type rawTraceData struct {
	SweepIndex int                  `json:"sweep_index"`
	Channels   map[string][]float64 `json:"channels"` // channel -> values
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

type server struct {
	dataDir string
	index   map[string]measurementIndex
}

func newServer(dataDir string) (*server, error) {
	s := &server{
		dataDir: dataDir,
		index:   make(map[string]measurementIndex),
	}
	if err := s.loadIndex(); err != nil {
		return nil, fmt.Errorf("failed to load index from %s: %w", dataDir, err)
	}
	return s, nil
}

func (s *server) loadIndex() error {
	indexPath := filepath.Join(s.dataDir, "averaged", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.index)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *server) handleListMeasurements(w http.ResponseWriter, r *http.Request) {
	var summaries []measurementSummary
	for _, idx := range s.index {
		summaries = append(summaries, measurementSummary{
			ID:        idx.MeasurementID,
			SweepGate: idx.SweepGate,
			NumPoints: idx.NumPoints,
			NumSweeps: idx.NumSweeps,
			HasRaw:    idx.RawDataRef != nil,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ID < summaries[j].ID
	})
	writeJSON(w, summaries)
}

func (s *server) handleGetPlotData(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/measurements/")
	id = strings.TrimSuffix(id, "/plot")
	if id == "" {
		http.Error(w, "measurement ID required", http.StatusBadRequest)
		return
	}

	idx, ok := s.index[id]
	if !ok {
		http.Error(w, fmt.Sprintf("measurement %q not found", id), http.StatusNotFound)
		return
	}

	// Load averaged data
	avgData, err := os.ReadFile(idx.FilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read averaged file: %v", err), http.StatusInternalServerError)
		return
	}
	var avg averagedRecord
	if err := json.Unmarshal(avgData, &avg); err != nil {
		http.Error(w, fmt.Sprintf("failed to parse averaged data: %v", err), http.StatusInternalServerError)
		return
	}

	pd := plotData{
		Voltages:      make([]float64, len(avg.AveragedTrace.Points)),
		Averaged:      make(map[string][]float64),
		SweepGate:     avg.SweepGate,
		NumSweeps:     avg.NumSweeps,
		MeasurementID: avg.MeasurementID,
	}

	// Extract averaged trace channels
	for i, pt := range avg.AveragedTrace.Points {
		pd.Voltages[i] = pt.Voltage
		for ch, val := range pt.Measurements {
			if pd.Averaged[ch] == nil {
				pd.Averaged[ch] = make([]float64, len(avg.AveragedTrace.Points))
			}
			pd.Averaged[ch][i] = val
		}
	}

	// Load raw traces if available
	if idx.RawDataRef != nil {
		rawPath := idx.RawDataRef.RawFilePath

		// Try both the absolute path and a relative path from our data dir
		rawData, err := os.ReadFile(rawPath)
		if err != nil {
			// Try constructing from data dir
			rawPath = filepath.Join(s.dataDir, "raw", fmt.Sprintf("raw_%s.json", id))
			rawData, err = os.ReadFile(rawPath)
		}
		if err == nil {
			var raw rawRecord
			if err := json.Unmarshal(rawData, &raw); err == nil {
				pd.RawTraces = make([]rawTraceData, len(raw.Traces))
				for t, tr := range raw.Traces {
					rtd := rawTraceData{
						SweepIndex: tr.SweepIndex,
						Channels:   make(map[string][]float64),
					}
					for _, pt := range tr.Points {
						for ch, val := range pt.Measurements {
							rtd.Channels[ch] = append(rtd.Channels[ch], val)
						}
					}
					pd.RawTraces[t] = rtd
				}
			}
		}
	}

	writeJSON(w, pd)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("failed to encode JSON response: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	dataDir := flag.String("data-dir", "", "path to the measurement data directory (required)")
	port := flag.Int("port", 8089, "HTTP port to listen on")
	flag.Parse()

	if *dataDir == "" {
		fmt.Fprintln(os.Stderr, "error: --data-dir is required")
		flag.Usage()
		os.Exit(1)
	}

	absDataDir, err := filepath.Abs(*dataDir)
	if err != nil {
		log.Fatalf("failed to resolve data dir: %v", err)
	}

	srv, err := newServer(absDataDir)
	if err != nil {
		log.Fatalf("failed to initialise server: %v", err)
	}

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/measurements", srv.handleListMeasurements)
	mux.HandleFunc("/api/measurements/", srv.handleGetPlotData)

	// Embedded frontend
	frontendSub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("failed to create sub-FS: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(frontendSub)))

	addr := fmt.Sprintf(":%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}

	url := fmt.Sprintf("http://localhost:%d", *port)
	log.Printf("FALCon Data Viewer")
	log.Printf("  data dir : %s", absDataDir)
	log.Printf("  measurements: %d", len(srv.index))
	log.Printf("  open: %s", url)

	if err := http.Serve(ln, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
