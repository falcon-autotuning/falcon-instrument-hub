//go:build cgo

package settinginterpreter

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
)

// FalconMeasurementRequest wraps a falcon-core MeasurementRequest handle
// and provides convenience methods for extracting instrument information.
type FalconMeasurementRequest struct {
	handle *measurementrequest.Handle
}

// NewFalconMeasurementRequestFromJSON deserializes a MeasurementRequest from JSON
// using the falcon-core API.
func NewFalconMeasurementRequestFromJSON(jsonStr string) (*FalconMeasurementRequest, error) {
	handle, err := measurementrequest.FromJSON(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize MeasurementRequest from JSON: %w", err)
	}

	return &FalconMeasurementRequest{handle: handle}, nil
}

// Close releases the underlying falcon-core handle.
// Must be called when done with the request.
func (r *FalconMeasurementRequest) Close() error {
	if r.handle != nil {
		return r.handle.Close()
	}
	return nil
}

// Handle returns the underlying measurementrequest.Handle for direct API access.
func (r *FalconMeasurementRequest) Handle() *measurementrequest.Handle {
	return r.handle
}

// ToJSON serializes the MeasurementRequest to JSON using the falcon-core API.
func (r *FalconMeasurementRequest) ToJSON() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.ToJSON()
}

// Message returns the message string from the request.
func (r *FalconMeasurementRequest) Message() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.Message()
}

// FalconMeasurementResponse wraps a falcon-core MeasurementResponse handle.
type FalconMeasurementResponse struct {
	handle *measurementresponse.Handle
}

// NewFalconMeasurementResponseFromJSON deserializes a MeasurementResponse from JSON.
func NewFalconMeasurementResponseFromJSON(jsonStr string) (*FalconMeasurementResponse, error) {
	handle, err := measurementresponse.FromJSON(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize MeasurementResponse from JSON: %w", err)
	}

	return &FalconMeasurementResponse{handle: handle}, nil
}

// Close releases the underlying handle.
func (r *FalconMeasurementResponse) Close() error {
	if r.handle != nil {
		return r.handle.Close()
	}
	return nil
}

// Handle returns the underlying measurementresponse.Handle.
func (r *FalconMeasurementResponse) Handle() *measurementresponse.Handle {
	return r.handle
}

// ToJSON serializes the MeasurementResponse to JSON.
func (r *FalconMeasurementResponse) ToJSON() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.ToJSON()
}

// Message returns the message string from the response.
func (r *FalconMeasurementResponse) Message() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.Message()
}

// symbolUnitFromString maps a unit symbol string (as written in instrument API
// YAML files) to the corresponding symbolunit.Handle.  The symbols follow the
// standard SI convention used by falcon-core's Constants.cpp.
func SymbolUnitFromString(unit string) (*symbolunit.Handle, error) {
	switch unit {
	case "m":
		return symbolunit.NewMeter()
	case "kg":
		return symbolunit.NewKilogram()
	case "s":
		return symbolunit.NewSecond()
	case "A":
		return symbolunit.NewAmpere()
	case "K":
		return symbolunit.NewKelvin()
	case "mol":
		return symbolunit.NewMole()
	case "cd":
		return symbolunit.NewCandela()
	case "Hz":
		return symbolunit.NewHertz()
	case "N":
		return symbolunit.NewNewton()
	case "Pa":
		return symbolunit.NewPascal()
	case "J":
		return symbolunit.NewJoule()
	case "W":
		return symbolunit.NewWatt()
	case "C":
		return symbolunit.NewCoulomb()
	case "V":
		return symbolunit.NewVolt()
	case "F":
		return symbolunit.NewFarad()
	case "Ω", "ohm":
		return symbolunit.NewOhm()
	case "S":
		return symbolunit.NewSiemens()
	case "Wb":
		return symbolunit.NewWeber()
	case "T":
		return symbolunit.NewTesla()
	case "H":
		return symbolunit.NewHenry()
	case "min":
		return symbolunit.NewMinute()
	case "mV":
		return symbolunit.NewMillivolt()
	case "mA":
		return symbolunit.NewMilliampere()
	case "μA", "uA":
		return symbolunit.NewMicroampere()
	case "nA":
		return symbolunit.NewNanoampere()
	case "pA":
		return symbolunit.NewPicoampere()
	case "ns":
		return symbolunit.NewNanosecond()
	case "":
		return symbolunit.NewDimensionless()
	default:
		return symbolunit.NewDimensionless()
	}
}
