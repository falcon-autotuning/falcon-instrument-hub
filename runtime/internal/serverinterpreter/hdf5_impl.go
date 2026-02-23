//go:build cgo && hdf5

// Package serverinterpreter – native HDF5 writer using gonum/hdf5.
//
// Build with:
//
//	go build -tags hdf5
//
// Requires libhdf5-dev installed on the host.
package serverinterpreter

import (
	"fmt"

	"gonum.org/v1/hdf5"
)

// hdf5Available is true when this file is compiled (native HDF5 support).
var hdf5Available = true

// writeHDF5Impl creates the HDF5 file and writes the averaged measurement.
func writeHDF5Impl(fp string, result *AveragedMeasurementResult) error {
	f, err := hdf5.CreateFile(fp, hdf5.F_ACC_TRUNC)
	if err != nil {
		return fmt.Errorf("hdf5 create: %w", err)
	}
	defer f.Close()

	// Create a group for this measurement
	grp, err := f.CreateGroup(result.MeasurementID)
	if err != nil {
		return fmt.Errorf("hdf5 create group: %w", err)
	}
	defer grp.Close()

	numPoints := len(result.AveragedTrace.Points)
	if numPoints == 0 {
		return fmt.Errorf("no points in averaged trace")
	}

	// ── voltages dataset ────────────────────────────────────────────
	voltages := make([]float64, numPoints)
	for i, pt := range result.AveragedTrace.Points {
		voltages[i] = pt.Voltage
	}

	if err := writeFloat64Dataset(grp, "voltages", voltages, []uint{uint(numPoints)}); err != nil {
		return err
	}

	// ── averaged_trace dataset  [numPoints × numChannels] ───────────
	channels := channelNames(result)
	if len(channels) > 0 {
		flat := make([]float64, numPoints*len(channels))
		for i, pt := range result.AveragedTrace.Points {
			for j, ch := range channels {
				flat[i*len(channels)+j] = pt.Measurements[ch]
			}
		}

		dims := []uint{uint(numPoints), uint(len(channels))}
		if err := writeFloat64Dataset(grp, "averaged_trace", flat, dims); err != nil {
			return err
		}

		// Store channel names as an attribute on averaged_trace.
		if err := writeStringListAttr(grp, "channel_names", channels); err != nil {
			return err
		}
	}

	// ── metadata (attributes on the group) ──────────────────────────
	attrs := map[string]interface{}{
		"measurement_id": result.MeasurementID,
		"sweep_gate":     result.SweepGate,
		"start_voltage":  result.StartVoltage,
		"stop_voltage":   result.StopVoltage,
		"num_points":     result.NumPoints,
		"num_sweeps":     result.NumSweeps,
		"total_duration_ns": int64(result.TotalDuration),
	}

	for k, v := range attrs {
		if err := writeAttr(grp, k, v); err != nil {
			return fmt.Errorf("hdf5 attr %s: %w", k, err)
		}
	}

	return nil
}

// writeFloat64Dataset writes a float64 slice as an HDF5 dataset.
func writeFloat64Dataset(grp *hdf5.Group, name string, data []float64, dims []uint) error {
	dspace, err := hdf5.CreateSimpleDataspace(dims, nil)
	if err != nil {
		return fmt.Errorf("hdf5 dataspace %s: %w", name, err)
	}
	defer dspace.Close()

	dtype, err := hdf5.NewDatatypeFromValue(float64(0))
	if err != nil {
		return fmt.Errorf("hdf5 dtype %s: %w", name, err)
	}
	defer dtype.Close()

	dset, err := grp.CreateDataset(name, dtype, dspace)
	if err != nil {
		return fmt.Errorf("hdf5 dataset %s: %w", name, err)
	}
	defer dset.Close()

	if err := dset.Write(&data[0]); err != nil {
		return fmt.Errorf("hdf5 write %s: %w", name, err)
	}
	return nil
}

// writeAttr writes a scalar attribute (string, float64, int, int64).
func writeAttr(grp *hdf5.Group, name string, value interface{}) error {
	switch v := value.(type) {
	case string:
		dtype, err := hdf5.NewDatatypeFromValue(v)
		if err != nil {
			return err
		}
		defer dtype.Close()

		space, err := hdf5.CreateSimpleDataspace([]uint{1}, nil)
		if err != nil {
			return err
		}
		defer space.Close()

		attr, err := grp.CreateAttribute(name, dtype, space)
		if err != nil {
			return err
		}
		defer attr.Close()
		return attr.Write(&v, dtype)

	case float64:
		dtype, err := hdf5.NewDatatypeFromValue(v)
		if err != nil {
			return err
		}
		defer dtype.Close()

		space, err := hdf5.CreateSimpleDataspace([]uint{1}, nil)
		if err != nil {
			return err
		}
		defer space.Close()

		attr, err := grp.CreateAttribute(name, dtype, space)
		if err != nil {
			return err
		}
		defer attr.Close()
		return attr.Write(&v, dtype)

	case int:
		return writeAttr(grp, name, int64(v))

	case int64:
		dtype, err := hdf5.NewDatatypeFromValue(v)
		if err != nil {
			return err
		}
		defer dtype.Close()

		space, err := hdf5.CreateSimpleDataspace([]uint{1}, nil)
		if err != nil {
			return err
		}
		defer space.Close()

		attr, err := grp.CreateAttribute(name, dtype, space)
		if err != nil {
			return err
		}
		defer attr.Close()
		return attr.Write(&v, dtype)

	default:
		return fmt.Errorf("unsupported attr type %T", value)
	}
}

// writeStringListAttr writes a list of strings as an attribute.
func writeStringListAttr(grp *hdf5.Group, name string, values []string) error {
	joined := ""
	for i, v := range values {
		if i > 0 {
			joined += ","
		}
		joined += v
	}
	return writeAttr(grp, name, joined)
}

// channelNames returns the channel names from the first averaged trace point.
func channelNames(result *AveragedMeasurementResult) []string {
	if len(result.AveragedTrace.Points) == 0 {
		return nil
	}
	channels := make([]string, 0, len(result.AveragedTrace.Points[0].Measurements))
	for ch := range result.AveragedTrace.Points[0].Measurements {
		channels = append(channels, ch)
	}
	return channels
}
