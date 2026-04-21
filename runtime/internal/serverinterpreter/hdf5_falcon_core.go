// Package serverinterpreter – falcon-core HDF5 storage using the hdf5data C API.
// Requires falcon-core-c-api pkg-config configuration.
package serverinterpreter

import (
	"fmt"
	"hash/fnv"
	"sort"
	"time"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/autotuner-interfaces/contexts/acquisitioncontext"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/hdf5data"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringstring"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/controlarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescontrolarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescoupledlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesint"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
)

// =============================================================================
// Raw 1D trace storage (single sweeps)
// =============================================================================

// storeRawTraceRecordAsHDF5 stores all individual sweeps from a RawTraceRecord
// as a single 2D HDF5 file using the falcon-core HDF5Data type.
//
// Storage layout:
//
//	shape:       [numTraces, numPoints]
//	unit_domain: [ControlArray(traceIndices), ControlArray(voltages)]
//	ranges:      one LabelledMeasuredArray per channel, flattened row-major
//	             [trace0_pt0, trace0_pt1, …, trace1_pt0, …]
func storeRawTraceRecordAsHDF5(fp string, record *RawTraceRecord) error {
	numTraces := len(record.Traces)
	if numTraces == 0 {
		return fmt.Errorf("empty trace record")
	}
	numPoints := record.NumPoints
	if numPoints == 0 && len(record.Traces[0].Points) > 0 {
		numPoints = len(record.Traces[0].Points)
	}
	if numPoints == 0 {
		return fmt.Errorf("zero points in raw trace record")
	}

	// ── shape ──────────────────────────────────────────────────────────────
	shapeHandle, err := axesint.New([]int32{int32(numTraces), int32(numPoints)})
	if err != nil {
		return fmt.Errorf("build shape: %w", err)
	}
	defer shapeHandle.Close()

	// ── unit_domain ─────────────────────────────────────────────────────────
	// Axis 0: trace indices [0, 1, …, numTraces-1]
	traceIdx := make([]float64, numTraces)
	for i := range traceIdx {
		traceIdx[i] = float64(i)
	}
	traceCA, err := controlarray.FromData(traceIdx, []uint64{uint64(numTraces)})
	if err != nil {
		return fmt.Errorf("build trace-index control array: %w", err)
	}
	defer traceCA.Close()

	// Axis 1: voltage values from the first trace
	voltages := make([]float64, numPoints)
	for i, pt := range record.Traces[0].Points {
		if i >= numPoints {
			break
		}
		voltages[i] = pt.Voltage
	}
	voltCA, err := controlarray.FromData(voltages, []uint64{uint64(numPoints)})
	if err != nil {
		return fmt.Errorf("build voltage control array: %w", err)
	}
	defer voltCA.Close()

	unitDomain, err := axescontrolarray.New([]*controlarray.Handle{traceCA, voltCA})
	if err != nil {
		return fmt.Errorf("build unit domain: %w", err)
	}
	defer unitDomain.Close()

	// ── domain_labels ───────────────────────────────────────────────────────
	domainLabels, err := axescoupledlabelleddomain.NewEmpty()
	if err != nil {
		return fmt.Errorf("build domain labels: %w", err)
	}
	defer domainLabels.Close()

	// ── ranges ──────────────────────────────────────────────────────────────
	// Gather channel names from all traces
	channelNames := gatherChannelNamesFromTraces(record.Traces)

	lmaHandles := make([]*labelledmeasuredarray.Handle, 0, len(channelNames))
	defer func() {
		for _, lma := range lmaHandles {
			lma.Close()
		}
	}()

	total := numTraces * numPoints
	for _, ch := range channelNames {
		flat := make([]float64, total)
		for ti, trace := range record.Traces {
			for pi, pt := range trace.Points {
				if pi >= numPoints {
					break
				}
				flat[ti*numPoints+pi] = pt.Measurements[ch]
			}
		}
		lma, err := buildLabelledMeasuredArray(flat, ch, total)
		if err != nil {
			return fmt.Errorf("build LMA for %s: %w", ch, err)
		}
		lmaHandles = append(lmaHandles, lma)
	}

	ranges, err := labelledarrayslabelledmeasuredarray.New(lmaHandles)
	if err != nil {
		return fmt.Errorf("build ranges: %w", err)
	}
	defer ranges.Close()

	// ── metadata ────────────────────────────────────────────────────────────
	meta, err := buildMetadataMap(map[string]string{
		"sweep_gate":     record.SweepGate,
		"start_voltage":  fmt.Sprintf("%.6f", record.StartVoltage),
		"stop_voltage":   fmt.Sprintf("%.6f", record.StopVoltage),
		"num_traces":     fmt.Sprintf("%d", numTraces),
		"num_points":     fmt.Sprintf("%d", numPoints),
		"measurement_id": record.MeasurementID,
		"data_type":      "raw_traces",
	})
	if err != nil {
		return fmt.Errorf("build metadata: %w", err)
	}
	defer meta.Close()

	uniqueID := measurementIDToInt32(record.MeasurementID)
	ts := int32(record.RecordedAt.Unix())

	hd, err := hdf5data.New(shapeHandle, unitDomain, domainLabels, ranges, meta,
		record.MeasurementID+"_raw", uniqueID, ts)
	if err != nil {
		return fmt.Errorf("create HDF5Data: %w", err)
	}
	defer hd.Close()

	return hd.ToFile(fp)
}

// loadRawTracesFromHDF5 reconstructs a RawTraceRecord from a 2D HDF5 file.
func loadRawTracesFromHDF5(fp string) (*RawTraceRecord, error) {
	hd, err := hdf5data.NewFromFile(fp)
	if err != nil {
		return nil, fmt.Errorf("load HDF5 file %s: %w", fp, err)
	}
	defer hd.Close()

	meta, err := hd.Metadata()
	if err != nil {
		return nil, fmt.Errorf("get metadata: %w", err)
	}
	defer meta.Close()

	measurementID, _ := meta.At("measurement_id")
	sweepGate, _ := meta.At("sweep_gate")
	startVStr, _ := meta.At("start_voltage")
	stopVStr, _ := meta.At("stop_voltage")
	numTracesStr, _ := meta.At("num_traces")
	numPointsStr, _ := meta.At("num_points")

	var startV, stopV float64
	fmt.Sscanf(startVStr, "%f", &startV)
	fmt.Sscanf(stopVStr, "%f", &stopV)
	var numTraces, numPoints int
	fmt.Sscanf(numTracesStr, "%d", &numTraces)
	fmt.Sscanf(numPointsStr, "%d", &numPoints)

	// Extract voltages from unit_domain axis 1
	unitDomain, err := hd.UnitDomain()
	if err != nil {
		return nil, fmt.Errorf("get unit domain: %w", err)
	}
	defer unitDomain.Close()

	var voltages []float64
	udSize, _ := unitDomain.Size()
	if udSize > 1 {
		voltCA, err := unitDomain.At(1)
		if err != nil {
			return nil, fmt.Errorf("get voltage control array: %w", err)
		}
		voltages, err = voltCA.Data()
		voltCA.Close()
		if err != nil {
			return nil, fmt.Errorf("get voltage data: %w", err)
		}
	}
	if numPoints == 0 {
		numPoints = len(voltages)
	}

	// Extract channel data from ranges
	ranges, err := hd.Ranges()
	if err != nil {
		return nil, fmt.Errorf("get ranges: %w", err)
	}
	defer ranges.Close()

	channelData, channelNames, err := extractChannelDataFromRanges(ranges)
	if err != nil {
		return nil, err
	}

	// Reconstruct individual traces
	ts, _ := hd.Timestamp()
	traces := make([]Trace, numTraces)
	for ti := range traces {
		traces[ti] = Trace{
			SweepIndex: ti,
			Points:     make([]TracePoint, numPoints),
			Timestamp:  time.Unix(int64(ts), 0),
		}
		for pi := range traces[ti].Points {
			meas := make(map[string]float64, len(channelNames))
			for _, ch := range channelNames {
				idx := ti*numPoints + pi
				if idx < len(channelData[ch]) {
					meas[ch] = channelData[ch][idx]
				}
			}
			v := 0.0
			if pi < len(voltages) {
				v = voltages[pi]
			}
			traces[ti].Points[pi] = TracePoint{Voltage: v, Measurements: meas}
		}
	}

	return &RawTraceRecord{
		MeasurementID: measurementID,
		SweepGate:     sweepGate,
		StartVoltage:  startV,
		StopVoltage:   stopV,
		NumTraces:     numTraces,
		NumPoints:     numPoints,
		Traces:        traces,
		Channels:      channelNames,
		RecordedAt:    time.Unix(int64(ts), 0),
	}, nil
}

// =============================================================================
// Averaged result storage
// =============================================================================

// storeAveragedAsHDF5 writes an averaged sweep result to a 1D HDF5 file using
// the falcon-core HDF5Data type.
//
// Storage layout:
//
//	shape:       [numPoints]
//	unit_domain: [ControlArray(voltages)]
//	ranges:      one LabelledMeasuredArray per channel
//	metadata:    sweep_gate, start_voltage, stop_voltage, num_sweeps, …
func storeAveragedAsHDF5(fp string, result *AveragedMeasurementResult) error {
	points := result.AveragedTrace.Points
	n := len(points)
	if n == 0 {
		return fmt.Errorf("empty averaged trace")
	}

	// ── shape ──────────────────────────────────────────────────────────────
	shapeHandle, err := axesint.New([]int32{int32(n)})
	if err != nil {
		return fmt.Errorf("build shape: %w", err)
	}
	defer shapeHandle.Close()

	// ── unit_domain ─────────────────────────────────────────────────────────
	voltages := make([]float64, n)
	for i, pt := range points {
		voltages[i] = pt.Voltage
	}
	voltCA, err := controlarray.FromData(voltages, []uint64{uint64(n)})
	if err != nil {
		return fmt.Errorf("build voltage control array: %w", err)
	}
	defer voltCA.Close()

	unitDomain, err := axescontrolarray.New([]*controlarray.Handle{voltCA})
	if err != nil {
		return fmt.Errorf("build unit domain: %w", err)
	}
	defer unitDomain.Close()

	// ── domain_labels ───────────────────────────────────────────────────────
	domainLabels, err := axescoupledlabelleddomain.NewEmpty()
	if err != nil {
		return fmt.Errorf("build domain labels: %w", err)
	}
	defer domainLabels.Close()

	// ── ranges ──────────────────────────────────────────────────────────────
	channelNames := extractChannelNamesFromPoints(points)
	lmaHandles := make([]*labelledmeasuredarray.Handle, 0, len(channelNames))
	defer func() {
		for _, lma := range lmaHandles {
			lma.Close()
		}
	}()

	for _, ch := range channelNames {
		chData := make([]float64, n)
		for i, pt := range points {
			chData[i] = pt.Measurements[ch]
		}
		lma, err := buildLabelledMeasuredArray(chData, ch, n)
		if err != nil {
			return fmt.Errorf("build LMA for %s: %w", ch, err)
		}
		lmaHandles = append(lmaHandles, lma)
	}

	ranges, err := labelledarrayslabelledmeasuredarray.New(lmaHandles)
	if err != nil {
		return fmt.Errorf("build ranges: %w", err)
	}
	defer ranges.Close()

	// ── metadata ────────────────────────────────────────────────────────────
	meta, err := buildMetadataMap(map[string]string{
		"sweep_gate":     result.SweepGate,
		"start_voltage":  fmt.Sprintf("%.6f", result.StartVoltage),
		"stop_voltage":   fmt.Sprintf("%.6f", result.StopVoltage),
		"num_sweeps":     fmt.Sprintf("%d", result.NumSweeps),
		"num_points":     fmt.Sprintf("%d", n),
		"measurement_id": result.MeasurementID,
		"data_type":      "averaged",
	})
	if err != nil {
		return fmt.Errorf("build metadata: %w", err)
	}
	defer meta.Close()

	uniqueID := measurementIDToInt32(result.MeasurementID)
	ts := int32(time.Now().Unix())

	hd, err := hdf5data.New(shapeHandle, unitDomain, domainLabels, ranges, meta,
		result.MeasurementID, uniqueID, ts)
	if err != nil {
		return fmt.Errorf("create HDF5Data: %w", err)
	}
	defer hd.Close()

	return hd.ToFile(fp)
}

// loadAveragedFromHDF5 reconstructs an AveragedMeasurementResult from a 1D HDF5 file.
func loadAveragedFromHDF5(fp, measurementID string) (*AveragedMeasurementResult, error) {
	hd, err := hdf5data.NewFromFile(fp)
	if err != nil {
		return nil, fmt.Errorf("load HDF5 file %s: %w", fp, err)
	}
	defer hd.Close()

	// Metadata
	meta, err := hd.Metadata()
	if err != nil {
		return nil, fmt.Errorf("get metadata: %w", err)
	}
	defer meta.Close()

	sweepGate, _ := meta.At("sweep_gate")
	startVStr, _ := meta.At("start_voltage")
	stopVStr, _ := meta.At("stop_voltage")
	numSweepsStr, _ := meta.At("num_sweeps")

	var startV, stopV float64
	fmt.Sscanf(startVStr, "%f", &startV)
	fmt.Sscanf(stopVStr, "%f", &stopV)
	var numSweeps int
	fmt.Sscanf(numSweepsStr, "%d", &numSweeps)

	// Voltages from unit_domain axis 0
	unitDomain, err := hd.UnitDomain()
	if err != nil {
		return nil, fmt.Errorf("get unit domain: %w", err)
	}
	defer unitDomain.Close()

	var voltages []float64
	udSize, _ := unitDomain.Size()
	if udSize > 0 {
		voltCA, err := unitDomain.At(0)
		if err != nil {
			return nil, fmt.Errorf("get voltage control array: %w", err)
		}
		voltages, err = voltCA.Data()
		voltCA.Close()
		if err != nil {
			return nil, fmt.Errorf("get voltage data: %w", err)
		}
	}

	numPoints := len(voltages)

	// Channel data from ranges
	ranges, err := hd.Ranges()
	if err != nil {
		return nil, fmt.Errorf("get ranges: %w", err)
	}
	defer ranges.Close()

	channelData, _, err := extractChannelDataFromRanges(ranges)
	if err != nil {
		return nil, err
	}

	// Reconstruct AveragedTrace points
	avgPoints := make([]TracePoint, numPoints)
	for i := range avgPoints {
		v := 0.0
		if i < len(voltages) {
			v = voltages[i]
		}
		meas := make(map[string]float64)
		for ch, data := range channelData {
			if i < len(data) {
				meas[ch] = data[i]
			}
		}
		avgPoints[i] = TracePoint{Voltage: v, Measurements: meas}
	}

	ts, _ := hd.Timestamp()

	return &AveragedMeasurementResult{
		MeasurementID: measurementID,
		SweepGate:     sweepGate,
		StartVoltage:  startV,
		StopVoltage:   stopV,
		NumPoints:     numPoints,
		NumSweeps:     numSweeps,
		AveragedTrace: AveragedTrace{
			Points:    avgPoints,
			NumSweeps: numSweeps,
			SweepGate: sweepGate,
			StartV:    startV,
			StopV:     stopV,
		},
		TotalDuration: time.Duration(ts) * time.Second,
	}, nil
}

// =============================================================================
// Helper functions
// =============================================================================

// buildLabelledMeasuredArray creates a LabelledMeasuredArray from measurement
// channel data. The channel name becomes the ohmic connection name (SI Ampere units).
func buildLabelledMeasuredArray(data []float64, channelName string, n int) (*labelledmeasuredarray.Handle, error) {
	fa, err := farraydouble.FromData(data, []uint64{uint64(n)})
	if err != nil {
		return nil, fmt.Errorf("build FArrayDouble: %w", err)
	}
	defer fa.Close()

	conn, err := connection.NewOhmic(channelName)
	if err != nil {
		return nil, fmt.Errorf("build connection: %w", err)
	}
	defer conn.Close()

	units, err := symbolunit.NewAmpere()
	if err != nil {
		return nil, fmt.Errorf("build symbol unit: %w", err)
	}
	defer units.Close()

	acq, err := acquisitioncontext.New(conn, "current_meter", units)
	if err != nil {
		return nil, fmt.Errorf("build acquisition context: %w", err)
	}
	defer acq.Close()

	return labelledmeasuredarray.FromFArray(fa, acq)
}

// buildMetadataMap constructs a MapStringString handle from a Go map.
func buildMetadataMap(entries map[string]string) (*mapstringstring.Handle, error) {
	meta, err := mapstringstring.NewEmpty()
	if err != nil {
		return nil, err
	}
	for k, v := range entries {
		if err := meta.InsertOrAssign(k, v); err != nil {
			meta.Close()
			return nil, fmt.Errorf("set %s: %w", k, err)
		}
	}
	return meta, nil
}

// extractChannelDataFromRanges reads channel name→data map from a
// LabelledArraysLabelledMeasuredArray handle.
func extractChannelDataFromRanges(ranges *labelledarrayslabelledmeasuredarray.Handle) (map[string][]float64, []string, error) {
	arrList, err := ranges.Arrays()
	if err != nil {
		return nil, nil, fmt.Errorf("get arrays list: %w", err)
	}
	defer arrList.Close()

	size, err := arrList.Size()
	if err != nil {
		return nil, nil, fmt.Errorf("get arrays list size: %w", err)
	}

	channelData := make(map[string][]float64, size)
	channelNames := make([]string, 0, size)

	for i := uint64(0); i < size; i++ {
		lma, err := arrList.At(i)
		if err != nil {
			continue
		}

		conn, err := lma.Connection()
		if err != nil {
			lma.Close()
			continue
		}
		chName, err := conn.Name()
		conn.Close()
		if err != nil {
			lma.Close()
			continue
		}

		data, err := lma.Data()
		lma.Close()
		if err != nil {
			continue
		}

		channelData[chName] = data
		channelNames = append(channelNames, chName)
	}

	return channelData, channelNames, nil
}

// extractChannelNamesFromPoints returns sorted unique channel names from trace points.
func extractChannelNamesFromPoints(points []TracePoint) []string {
	seen := make(map[string]struct{})
	for _, pt := range points {
		for ch := range pt.Measurements {
			seen[ch] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for ch := range seen {
		names = append(names, ch)
	}
	sort.Strings(names)
	return names
}

// gatherChannelNamesFromTraces collects all unique channel names across all traces.
func gatherChannelNamesFromTraces(traces []Trace) []string {
	seen := make(map[string]struct{})
	for _, trace := range traces {
		for _, pt := range trace.Points {
			for ch := range pt.Measurements {
				seen[ch] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(seen))
	for ch := range seen {
		names = append(names, ch)
	}
	sort.Strings(names)
	return names
}

// measurementIDToInt32 converts a measurement ID string to an int32 via FNV-1a hash.
func measurementIDToInt32(id string) int32 {
	h := fnv.New32a()
	h.Write([]byte(id))
	return int32(h.Sum32())
}
