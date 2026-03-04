// Package serverinterpreter provides a test that generates a comprehensive demo
// dataset covering three measurement types: DC get/set, averaged 1D sweep, and
// 2D sweep. Running this test writes everything to
// test-outs/data/demo_measurements/ where it can be viewed with the dataviewer.
//
//	go test ./internal/serverinterpreter -run TestGenerate_DemoMeasurements -v
//
// Then launch the dataviewer:
//
//	go build -o bin/dataviewer ./cmd/dataviewer/
//	./bin/dataviewer --data-dir ../test-outs/data/demo_measurements --port 8089
package serverinterpreter

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// =============================================================================
// Demo: All three measurement types in one dataset
// =============================================================================

const demoDir = "demo_measurements"

// TestGenerate_DemoMeasurements creates a complete demo dataset with:
//   - 1 × DC point collection (5 set-voltage → read-current operations grouped)
//   - 1 × averaged 1D sweep (Coulomb peak, 12 averages)
//   - 1 × 2D sweep (101 × 31 grid with Coulomb diamond pattern)
//   - 1 × 1D axis sweep (5 angles from 10° to 80°)
//
// All data is written to test-outs/data/demo_measurements/ and can be viewed
// with the dataviewer.
func TestGenerate_DemoMeasurements(t *testing.T) {
	outDir := filepath.Join("..", "..", "..", "..", "test-outs", "data", demoDir)
	absDir, err := filepath.Abs(outDir)
	require.NoError(t, err)

	// Clean previous run
	_ = os.RemoveAll(absDir)

	db, err := NewMeasurementDatabase(absDir)
	require.NoError(t, err)

	t.Logf("Demo dataset directory: %s\n", absDir)

	// -------------------------------------------------------------------------
	// Part A: DC point collection (5 grouped set/get operations)
	// -------------------------------------------------------------------------
	t.Run("A_DC_PointCollection", func(t *testing.T) {
		generateDCPointCollection(t, db)
	})

	// -------------------------------------------------------------------------
	// Part B: Averaged 1D sweep (Coulomb peak)
	// -------------------------------------------------------------------------
	t.Run("B_Averaged_1D_Sweep", func(t *testing.T) {
		generateAveraged1DSweep(t, db)
	})

	// -------------------------------------------------------------------------
	// Part C: 2D sweep
	// -------------------------------------------------------------------------
	t.Run("C_2D_Sweep", func(t *testing.T) {
		generate2DSweep(t, db, absDir)
	})

	// -------------------------------------------------------------------------
	// Part D: 1D axis sweep (5 angles)
	// -------------------------------------------------------------------------
	t.Run("D_Axis_Sweep", func(t *testing.T) {
		generateAxisSweep(t, db)
	})

	// Summary
	all := db.List()
	t.Logf("\n✓ Demo dataset complete: %d measurements", len(all))
	for _, idx := range all {
		t.Logf("  • %s  [%s]", idx.MeasurementID, idx.MeasurementType)
	}
	t.Logf("\nTo view:")
	t.Logf("  cd runtime")
	t.Logf("  go build -o bin/dataviewer ./cmd/dataviewer/")
	t.Logf("  ./bin/dataviewer --data-dir ../test-outs/data/%s --port 8089", demoDir)
}

// =============================================================================
// Part A: DC Point Collection — 5 grouped set/get operations
// =============================================================================

// generateDCPointCollection simulates 5 set-voltage-get-current operations
// grouped as a single measurement command from the hub. Displayed as a table
// in the dataviewer.
func generateDCPointCollection(t *testing.T, db *MeasurementDatabase) {
	t.Helper()

	voltages := []float64{-0.50, -0.25, 0.00, 0.25, 0.50}
	channel := "DMM1_0"

	// Simulate a device where current follows a Coulomb peak centred at 0 V
	peakCenter := 0.0
	peakWidth := 0.20
	peakHeight := 8e-9 // 8 nA
	baseline := 1e-10  // 100 pA leakage

	seed := uint64(99)

	points := make([]DCPoint, len(voltages))
	for i, setV := range voltages {
		dv := setV - peakCenter
		lorentzian := peakHeight / (1.0 + (dv*dv)/(peakWidth*peakWidth))
		noise := noiseFromSeed(&seed, 1e-10)
		current := baseline + lorentzian + noise

		points[i] = DCPoint{
			GateName:     "P1",
			SetVoltage:   setV,
			Measurements: map[string]float64{channel: current},
			Timestamp:    time.Date(2026, 6, 15, 9, 0, i, 0, time.UTC),
		}

		t.Logf("  DC point %d: V=%.2f → I=%.2e A", i+1, setV, current)
	}

	result := &DCPointCollectionResult{
		MeasurementID: "dc-collection-001",
		Points:        points,
		Channels:      []string{channel},
		StartTime:     time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 6, 15, 9, 0, 5, 0, time.UTC),
	}

	filePath, err := db.StoreDCCollection(result)
	require.NoError(t, err)
	t.Logf("DC point collection: 5 points stored → %s", filePath)
}

// =============================================================================
// Part B: Averaged 1D sweep (Coulomb peak with noise)
// =============================================================================

// generateAveraged1DSweep creates a 1D sweep with 12 individual traces and
// their average. The simulated data shows a Coulomb peak with realistic noise.
func generateAveraged1DSweep(t *testing.T, db *MeasurementDatabase) {
	t.Helper()

	measurementID := "averaged-sweep-001"
	sweepGate := "P1"
	startV := -1.0
	stopV := 0.5
	numPoints := 151
	numSweeps := 12
	channel := "DMM1_0"

	// Peak parameters
	peakCenter := -0.25
	peakWidth := 0.06  // HWHM
	peakHeight := 6e-9 // 6 nA
	baseline := 5e-11  // 50 pA
	noiseAmp := 3e-10  // noise σ

	step := (stopV - startV) / float64(numPoints-1)
	seed := uint64(2025)

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
		TotalDuration: 15 * time.Second,
	}

	// Generate individual traces
	for s := 0; s < numSweeps; s++ {
		result.AllTraces[s] = Trace{
			SweepIndex: s + 1,
			Points:     make([]TracePoint, numPoints),
			Timestamp:  time.Date(2026, 6, 15, 10, 0, s*2, 0, time.UTC),
		}
		result.AveragedTrace.Timestamps[s] = result.AllTraces[s].Timestamp

		for i := 0; i < numPoints; i++ {
			voltage := startV + float64(i)*step
			dv := voltage - peakCenter
			lorentzian := peakHeight / (1.0 + (dv*dv)/(peakWidth*peakWidth))
			noise := noiseFromSeed(&seed, noiseAmp)
			current := baseline + lorentzian + noise

			result.AllTraces[s].Points[i] = TracePoint{
				Voltage:      voltage,
				Measurements: map[string]float64{channel: current},
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
		result.AveragedTrace.Points[i] = TracePoint{
			Voltage:      voltage,
			Measurements: map[string]float64{channel: sum / float64(numSweeps)},
		}
	}

	avgPath, err := db.Store(result)
	require.NoError(t, err)
	require.NotNil(t, result.RawRef)

	t.Logf("Averaged 1D sweep: %s", avgPath)
	t.Logf("  Gate: %s  Range: [%.1f, %.1f] V  Points: %d  Sweeps: %d",
		sweepGate, startV, stopV, numPoints, numSweeps)

	// Verify reload
	loaded, err := db.LoadWithRawTraces(measurementID)
	require.NoError(t, err)
	require.Len(t, loaded.AllTraces, numSweeps)
}

// =============================================================================
// Coulomb diamond physics (shared by 2D sweep and axis sweep)
// =============================================================================

// diamondCurrent evaluates the Coulomb-diamond current at gate voltages (xV, yV).
// This is the same model used for sweep-2d-001 so that the axis sweep data is
// physically consistent when viewed in P1 × P2 gate space.
func diamondCurrent(xV, yV float64, seed *uint64) float64 {
	diamondCenter := [2]float64{0.0, 0.0}
	diamondScale := 0.25 // controls diamond size
	peakHeight := 4e-9   // 4 nA at diamond edge
	baseline := 2e-11    // 20 pA background
	edgeWidth := 0.04

	dx := xV - diamondCenter[0]
	dy := yV - diamondCenter[1]

	edge1 := math.Abs(dx+dy) - diamondScale
	edge2 := math.Abs(dx-dy) - diamondScale

	lor1 := peakHeight / (1.0 + (edge1*edge1)/(edgeWidth*edgeWidth))
	lor2 := peakHeight / (1.0 + (edge2*edge2)/(edgeWidth*edgeWidth))

	noise := noiseFromSeed(seed, 5e-11)
	return baseline + lor1 + lor2 + noise
}

// =============================================================================
// Part C: 2D sweep — stored as a single 2D measurement with channel_data matrix
// =============================================================================

// generate2DSweep creates a 2D voltage sweep (P1 × P2) where:
//   - P1 is the fast axis (101 points per line)
//   - P2 is the slow axis (31 lines)
//
// The result is stored as a single Sweep2DMeasurementResult viewable in the
// dataviewer as a heatmap with colorbar.
func generate2DSweep(t *testing.T, db *MeasurementDatabase, absDir string) {
	t.Helper()

	channel := "DMM1_0"

	// X-axis (fast): P1
	xGate := "P1"
	xStartV := -0.5
	xStopV := 0.5
	xNumPoints := 101

	// Y-axis (slow): P2
	yGate := "P2"
	yStartV := -0.3
	yStopV := 0.3
	yNumPoints := 31

	xStep := (xStopV - xStartV) / float64(xNumPoints-1)
	yStep := (yStopV - yStartV) / float64(yNumPoints-1)

	seed := uint64(7777)

	// Build X and Y voltage arrays
	xVoltages := make([]float64, xNumPoints)
	for i := 0; i < xNumPoints; i++ {
		xVoltages[i] = xStartV + float64(i)*xStep
	}
	yVoltages := make([]float64, yNumPoints)
	for j := 0; j < yNumPoints; j++ {
		yVoltages[j] = yStartV + float64(j)*yStep
	}

	// Build the current matrix [y][x] and line-cuts
	currentMatrix := make([][]float64, yNumPoints)
	lines := make([]Sweep2DLine, yNumPoints)

	for j := 0; j < yNumPoints; j++ {
		yV := yVoltages[j]
		currentMatrix[j] = make([]float64, xNumPoints)
		lineCurrents := make([]float64, xNumPoints)

		for i := 0; i < xNumPoints; i++ {
			xV := xVoltages[i]
			current := diamondCurrent(xV, yV, &seed)

			currentMatrix[j][i] = current
			lineCurrents[i] = current
		}

		lines[j] = Sweep2DLine{
			YVoltage:  yV,
			YIndex:    j,
			Currents:  map[string][]float64{channel: lineCurrents},
			Timestamp: time.Date(2026, 6, 15, 12, j, 0, 0, time.UTC),
		}
	}

	result := &Sweep2DMeasurementResult{
		MeasurementID: "sweep-2d-001",
		XGate:         xGate,
		YGate:         yGate,
		XVoltages:     xVoltages,
		YVoltages:     yVoltages,
		Channels:      []string{channel},
		ChannelData:   map[string][][]float64{channel: currentMatrix},
		Lines:         lines,
		StartTime:     time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 6, 15, 12, 30, 0, 0, time.UTC),
	}

	filePath, err := db.Store2D(result)
	require.NoError(t, err)

	t.Logf("2D sweep: %d × %d = %d points", xNumPoints, yNumPoints, xNumPoints*yNumPoints)
	t.Logf("  X: %s [%.2f, %.2f] V  Y: %s [%.2f, %.2f] V",
		xGate, xStartV, xStopV, yGate, yStartV, yStopV)
	t.Logf("  Stored as single 2D measurement: %s", filePath)
}

// =============================================================================
// Part D: 1D Axis Sweep — 5 angles along V_P1·cos(θ) + V_P2·sin(θ)
// =============================================================================

// generateAxisSweep creates 1D sweeps along a parameterised voltage axis
// V = V_P1·cos(θ) + V_P2·sin(θ) at 5 different angles between 10° and 80°.
// The current at each point is evaluated using the same Coulomb diamond model
// as generate2DSweep (via diamondCurrent), so viewing the axis sweep in
// P1 × P2 gate space should match the 2D sweep data.
func generateAxisSweep(t *testing.T, db *MeasurementDatabase) {
	t.Helper()

	channel := "DMM1_0"
	gate1 := "P1"
	gate2 := "P2"

	angles := []float64{10, 27.5, 45, 62.5, 80} // degrees
	numPoints := 101
	vStart := 0.0  // rays fan outward from the origin
	vStop := 0.6
	vStep := (vStop - vStart) / float64(numPoints-1)

	seed := uint64(31415)

	sweeps := make([]AxisSweep1DLine, len(angles))

	for a, angleDeg := range angles {
		theta := angleDeg * math.Pi / 180.0
		cosT := math.Cos(theta)
		sinT := math.Sin(theta)

		voltages := make([]float64, numPoints)
		gate1V := make([]float64, numPoints)
		gate2V := make([]float64, numPoints)
		currents := make([]float64, numPoints)

		for i := 0; i < numPoints; i++ {
			v := vStart + float64(i)*vStep
			voltages[i] = v
			gate1V[i] = v * cosT
			gate2V[i] = v * sinT

			// Sample the same diamond physics used in the 2D sweep
			currents[i] = diamondCurrent(gate1V[i], gate2V[i], &seed)
		}

		sweeps[a] = AxisSweep1DLine{
			AngleDeg:    angleDeg,
			Voltages:    voltages,
			ChannelData: map[string][]float64{channel: currents},
			Gate1V:      gate1V,
			Gate2V:      gate2V,
			Timestamp:   time.Date(2026, 6, 15, 14, a*5, 0, 0, time.UTC),
		}

		t.Logf("  Angle %.1f°: %d points, P1 [%.3f, %.3f], P2 [%.3f, %.3f]",
			angleDeg, numPoints, gate1V[0], gate1V[numPoints-1], gate2V[0], gate2V[numPoints-1])
	}

	result := &AxisSweep1DResult{
		MeasurementID: "axis-sweep-001",
		Gate1:         gate1,
		Gate2:         gate2,
		Channels:      []string{channel},
		Sweeps:        sweeps,
		StartTime:     time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 6, 15, 14, 25, 0, 0, time.UTC),
	}

	filePath, err := db.StoreAxisSweep(result)
	require.NoError(t, err)
	t.Logf("Axis sweep: %d angles, %d pts each → %s", len(angles), numPoints, filePath)
}

// =============================================================================
// Utility
// =============================================================================

// noiseFromSeed generates a pseudo-Gaussian noise sample using a simple LCG.
// The seed is mutated in place for deterministic reproducibility.
func noiseFromSeed(seed *uint64, amplitude float64) float64 {
	sum := 0.0
	for k := 0; k < 6; k++ {
		*seed = *seed*6364136223846793005 + 1
		sum += float64(*seed>>33) / float64(1<<31)
	}
	return amplitude * (sum - 3.0) / 3.0
}
