// Package serverinterpreter provides a test that generates a realistic dummy
// dataset for GUI development. Running this test writes a complete raw + averaged
// measurement to test-outs/data/dummy_measurement/.
//
//	go test ./internal/serverinterpreter -run TestGenerate_DummyDataset -v
package serverinterpreter

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGenerate_DummyDataset creates a realistic measurement dataset with
// 10 raw current traces and an averaged result. The data simulates a
// Coulomb-peak-like feature: a Lorentzian in the current vs gate voltage.
//
// Output layout:
//
//	test-outs/data/dummy_measurement/
//	  averaged/
//	    index.json
//	    sweep_coulomb-peak-001.json
//	  raw/
//	    raw_index.json
//	    raw_coulomb-peak-001.json
func TestGenerate_DummyDataset(t *testing.T) {
	// Write to test-outs/data/dummy_measurement relative to the repo root
	outDir := filepath.Join("..", "..", "..", "..", "test-outs", "data", "dummy_measurement")

	// Resolve to absolute path for clarity in logs
	absDir, err := filepath.Abs(outDir)
	require.NoError(t, err)

	// Clean previous run
	_ = os.RemoveAll(absDir)

	db, err := NewMeasurementDatabase(absDir)
	require.NoError(t, err)

	// ---------------------------------------------------------------------------
	// Measurement parameters
	// ---------------------------------------------------------------------------
	measurementID := "coulomb-peak-001"
	sweepGate := "P1"
	startV := -1.5 // Volts
	stopV := 0.5   // Volts
	numPoints := 201
	numSweeps := 10
	channel := "DMM1_0"

	// Coulomb peak parameters
	peakCenter := -0.5       // Volts
	peakWidth := 0.08        // Lorentzian HWHM in Volts
	peakHeight := 5e-9       // Amps
	baselineCurrent := 1e-10 // Amps (leakage floor)
	noiseAmplitude := 2e-10  // Amps (Gaussian noise σ)

	// Deterministic "noise" using a simple LCG so the fixture is reproducible
	seed := uint64(42)
	lcg := func() float64 {
		seed = seed*6364136223846793005 + 1
		// Map to roughly Gaussian via Box-Muller-ish cheap approximation:
		// sum of 6 uniform deviates ≈ N(0,1)
		sum := 0.0
		for k := 0; k < 6; k++ {
			seed = seed*6364136223846793005 + 1
			sum += float64(seed>>33) / float64(1<<31)
		}
		return (sum - 3.0) / 3.0 // rough standard normal
	}

	step := (stopV - startV) / float64(numPoints-1)

	// ---------------------------------------------------------------------------
	// Build traces
	// ---------------------------------------------------------------------------
	result := &AveragedMeasurementResult{
		MeasurementID: measurementID,
		SweepGate:     sweepGate,
		StartVoltage:  startV,
		StopVoltage:   stopV,
		NumPoints:     numPoints,
		NumSweeps:     numSweeps,
		AllTraces:     make([]Trace, numSweeps),
		AveragedTrace: AveragedTrace{
			Points:     make([]TracePoint, numPoints),
			NumSweeps:  numSweeps,
			SweepGate:  sweepGate,
			StartV:     startV,
			StopV:      stopV,
			Timestamps: make([]time.Time, numSweeps),
		},
		TotalDuration: 12 * time.Second,
	}

	// Generate individual raw traces
	for s := 0; s < numSweeps; s++ {
		result.AllTraces[s] = Trace{
			SweepIndex: s + 1,
			Points:     make([]TracePoint, numPoints),
			Timestamp:  time.Date(2026, 2, 13, 10, 0, s, 0, time.UTC),
		}
		result.AveragedTrace.Timestamps[s] = result.AllTraces[s].Timestamp

		for i := 0; i < numPoints; i++ {
			voltage := startV + float64(i)*step

			// Lorentzian peak + baseline + noise
			dv := voltage - peakCenter
			lorentzian := peakHeight / (1.0 + (dv*dv)/(peakWidth*peakWidth))
			noise := noiseAmplitude * lcg()
			current := baselineCurrent + lorentzian + noise

			result.AllTraces[s].Points[i] = TracePoint{
				Voltage: voltage,
				Measurements: map[string]float64{
					channel: current,
				},
			}
		}
	}

	// Compute averaged trace
	for i := 0; i < numPoints; i++ {
		voltage := startV + float64(i)*step
		sum := 0.0
		for s := 0; s < numSweeps; s++ {
			sum += result.AllTraces[s].Points[i].Measurements[channel]
		}
		mean := sum / float64(numSweeps)

		result.AveragedTrace.Points[i] = TracePoint{
			Voltage: voltage,
			Measurements: map[string]float64{
				channel: mean,
			},
		}
	}

	// ---------------------------------------------------------------------------
	// Store to two-database
	// ---------------------------------------------------------------------------
	avgPath, err := db.Store(result)
	require.NoError(t, err)
	require.NotNil(t, result.RawRef)

	t.Logf("Dummy dataset written to: %s", absDir)
	t.Logf("  Averaged: %s", avgPath)
	t.Logf("  Raw:      %s", result.RawRef.RawFilePath)
	t.Logf("")
	t.Logf("Parameters:")
	t.Logf("  Sweep gate:      %s", sweepGate)
	t.Logf("  Voltage range:   [%.2f, %.2f] V", startV, stopV)
	t.Logf("  Num points:      %d", numPoints)
	t.Logf("  Num sweeps:      %d", numSweeps)
	t.Logf("  Peak center:     %.2f V", peakCenter)
	t.Logf("  Peak width:      %.3f V (HWHM)", peakWidth)
	t.Logf("  Peak height:     %.1e A", peakHeight)
	t.Logf("  Noise σ:         %.1e A", noiseAmplitude)

	// Quick sanity: averaged peak should be near peakHeight at the center
	centerIdx := 0
	minDist := math.Abs(result.AveragedTrace.Points[0].Voltage - peakCenter)
	for i := 1; i < numPoints; i++ {
		d := math.Abs(result.AveragedTrace.Points[i].Voltage - peakCenter)
		if d < minDist {
			minDist = d
			centerIdx = i
		}
	}
	avgPeakCurrent := result.AveragedTrace.Points[centerIdx].Measurements[channel]
	t.Logf("  Averaged peak at V=%.3f: I=%.3e A",
		result.AveragedTrace.Points[centerIdx].Voltage, avgPeakCurrent)

	// Verify files exist
	require.FileExists(t, avgPath)
	require.FileExists(t, result.RawRef.RawFilePath)

	// Verify we can reload
	loaded, err := db.LoadWithRawTraces(measurementID)
	require.NoError(t, err)
	require.Len(t, loaded.AllTraces, numSweeps)

	fmt.Printf("\n✓ Dummy dataset ready at: %s\n", absDir)
}
