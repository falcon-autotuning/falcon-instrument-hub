//go:build !cgo || !falcon_core

package handlers

import "fmt"

// buildMeasurementResponseJSON is a stub that is compiled when the cgo and
// falcon_core build tags are not set.  Building a valid falcon-core
// MeasurementResponse requires the C library, so this path always returns an
// error to make the limitation explicit.
func buildMeasurementResponseJSON(
	bufferData []float64,
	setterConnJSON string,
	getterInstrType string,
	getterUnitsJSON string,
	hash int64,
) (string, error) {
	return "", fmt.Errorf("buildMeasurementResponseJSON requires -tags cgo,falcon_core")
}
