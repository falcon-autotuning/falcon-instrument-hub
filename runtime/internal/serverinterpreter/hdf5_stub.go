//go:build !(cgo && hdf5)

// Package serverinterpreter – stub for when HDF5 C bindings are not available.
//
// This file is compiled by default (no build tags). It makes writeHDF5Impl
// return an error, which causes the caller to fall back to JSON.
package serverinterpreter

import "fmt"

// hdf5Available is false when native HDF5 support is not compiled in.
var hdf5Available = false

// writeHDF5Impl is the no-op stub used when the hdf5 build tag is absent.
func writeHDF5Impl(_ string, _ *AveragedMeasurementResult) error {
	return fmt.Errorf("hdf5: native HDF5 support not compiled (build with -tags hdf5)")
}
