//go:build cgo

package settinginterpreter

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
)

// FalconSettingRequest wraps a falcon-core settingrequest handle
// and provides convenience methods for extracting instrument information.
type FalconSettingRequest struct {
	handle *settingrequest.Handle
}

// NewFalconSettingRequestFromJSON deserializes a settingrequest from JSON
// using the falcon-core API.
func NewFalconSettingRequestFromJSON(jsonStr string) (*FalconSettingRequest, error) {
	handle, err := settingrequest.FromJSON(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize settingrequest from JSON: %w", err)
	}

	return &FalconSettingRequest{handle: handle}, nil
}

// Close releases the underlying falcon-core handle.
// Must be called when done with the request.
func (r *FalconSettingRequest) Close() error {
	if r.handle != nil {
		return r.handle.Close()
	}
	return nil
}

func (r *FalconSettingRequest) Getters() ([]config.ConnectedPort, error) {
	if r.handle != nil {
		return nil, r.handle.Close()
	}
	_, err := r.handle.Getters()
	// FIX: do something with Getters
	return nil, err
}

type Quantity struct {
	Value float64
	Unit  string
}

func (r *FalconSettingRequest) Setters() (map[config.ConnectedPort]Quantity, error) {
	if r.handle != nil {
		return nil, r.handle.Close()
	}
	_, err := r.handle.Setters()
	// FIX: do something with Setters
	return nil, err
}

// Handle returns the underlying settingrequest.Handle for direct API access.
func (r *FalconSettingRequest) Handle() *settingrequest.Handle {
	return r.handle
}

// ToJSON serializes the settingrequest to JSON using the falcon-core API.
func (r *FalconSettingRequest) ToJSON() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.ToJSON()
}

// Message returns the message string from the request.
func (r *FalconSettingRequest) Message() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.Message()
}

// FalconSettingResponse wraps a falcon-core settingresponse handle.
type FalconSettingResponse struct {
	handle *settingresponse.Handle
}

// NewFalconSettingResponseFromJSON deserializes a settingresponse from JSON.
func NewFalconSettingResponseFromJSON(jsonStr string) (*FalconSettingResponse, error) {
	handle, err := settingresponse.FromJSON(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize settingresponse from JSON: %w", err)
	}

	return &FalconSettingResponse{handle: handle}, nil
}

// Close releases the underlying handle.
func (r *FalconSettingResponse) Close() error {
	if r.handle != nil {
		return r.handle.Close()
	}
	return nil
}

// Handle returns the underlying settingresponse.Handle.
func (r *FalconSettingResponse) Handle() *settingresponse.Handle {
	return r.handle
}

// ToJSON serializes the settingresponse to JSON.
func (r *FalconSettingResponse) ToJSON() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.ToJSON()
}

// Message returns the message string from the response.
func (r *FalconSettingResponse) Message() (string, error) {
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
