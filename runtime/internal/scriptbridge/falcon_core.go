// Package scriptbridge provides falcon-core integration types and wrappers.
//
// This file contains the integration with falcon-core-libs Go bindings.
// It wraps the falcon-core MeasurementRequest type and provides methods
// to extract instrument information for script generation.
//
// IMPORTANT: This package requires the falcon-core C library to be installed
// and configured via pkg-config (falcon_core_c_api). The falcon-core-libs
// Go bindings use CGO to interface with the C library.
//
// # Dependencies
//
// Download and install the following:
//
//  1. Go bindings library:
//     https://github.com/falcon-autotuning/falcon-core-libs/releases/download/v0.0.1/falcon-core-go.zip
//
//  2. Falcon-core C++ library (Linux x64):
//     https://github.com/falcon-autotuning/falcon-core/releases/download/v1.0.0/falcon-core-cpp-linux-x64.tar.gz
//
//  3. Falcon-core C API (Linux x64):
//     https://github.com/falcon-autotuning/falcon-core/releases/download/v1.0.0/falcon-core-c-api-linux-x64.tar.gz
//
// # Setup
//
// After extracting the archives:
//
//  1. Set PKG_CONFIG_PATH to include the path to falcon_core_c_api.pc
//  2. Set LD_LIBRARY_PATH to include the falcon-core library path (or configure rpath)
//
// Example:
//
//	export PKG_CONFIG_PATH="/path/to/falcon-core/lib/pkgconfig:$PKG_CONFIG_PATH"
//	export LD_LIBRARY_PATH="/path/to/falcon-core/lib:$LD_LIBRARY_PATH"
package scriptbridge

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listwaveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/port-transforms/porttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/waveform"
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

// MeasurementName returns the measurement name from the request.
func (r *FalconMeasurementRequest) MeasurementName() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.MeasurementName()
}

// ExtractedInstrumentInfo contains information extracted from a falcon-core InstrumentPort.
type ExtractedInstrumentInfo struct {
	DefaultName          string // The default/configured name (e.g., "DAC1")
	InstrumentFacingName string // The instrument-facing name
	InstrumentType       string // Type of instrument (e.g., "VoltageSource", "Voltmeter")
	IsKnob               bool   // True if this is a setter/knob
	IsMeter              bool   // True if this is a getter/meter
	Description          string // Human-readable description
}

// ExtractGetters extracts getter instrument information from the MeasurementRequest.
func (r *FalconMeasurementRequest) ExtractGetters() ([]ExtractedInstrumentInfo, error) {
	if r.handle == nil {
		return nil, fmt.Errorf("handle is nil")
	}

	gettersHandle, err := r.handle.Getters()
	if err != nil {
		return nil, fmt.Errorf("failed to get getters: %w", err)
	}
	defer gettersHandle.Close()

	return extractInstrumentInfoFromPorts(gettersHandle)
}

// ExtractSetters extracts setter instrument information from the MeasurementRequest's waveforms.
// Setters are InstrumentPorts found in the waveform's PortTransforms.
func (r *FalconMeasurementRequest) ExtractSetters() ([]ExtractedInstrumentInfo, error) {
	if r.handle == nil {
		return nil, fmt.Errorf("handle is nil")
	}

	waveformsHandle, err := r.handle.Waveforms()
	if err != nil {
		return nil, fmt.Errorf("failed to get waveforms: %w", err)
	}
	defer waveformsHandle.Close()

	return extractInstrumentInfoFromWaveforms(waveformsHandle)
}

// extractInstrumentInfoFromPorts extracts instrument info from a Ports handle.
func extractInstrumentInfoFromPorts(portsHandle *ports.Handle) ([]ExtractedInstrumentInfo, error) {
	size, err := portsHandle.Size()
	if err != nil {
		return nil, fmt.Errorf("failed to get ports size: %w", err)
	}

	results := make([]ExtractedInstrumentInfo, 0, size)

	for i := uint64(0); i < size; i++ {
		portHandle, err := portsHandle.At(i)
		if err != nil {
			return nil, fmt.Errorf("failed to get port at index %d: %w", i, err)
		}

		info, err := extractInfoFromInstrumentPort(portHandle)
		portHandle.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to extract info from port at index %d: %w", i, err)
		}

		results = append(results, info)
	}

	return results, nil
}

// extractInstrumentInfoFromWaveforms extracts instrument info from waveform PortTransforms.
func extractInstrumentInfoFromWaveforms(waveformsHandle *listwaveform.Handle) ([]ExtractedInstrumentInfo, error) {
	size, err := waveformsHandle.Size()
	if err != nil {
		return nil, fmt.Errorf("failed to get waveforms size: %w", err)
	}

	var results []ExtractedInstrumentInfo
	seenPorts := make(map[string]bool) // Deduplicate by default name

	for i := uint64(0); i < size; i++ {
		wfHandle, err := waveformsHandle.At(i)
		if err != nil {
			return nil, fmt.Errorf("failed to get waveform at index %d: %w", i, err)
		}

		wfInfos, err := extractInstrumentInfoFromWaveform(wfHandle)
		wfHandle.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to extract info from waveform at index %d: %w", i, err)
		}

		for _, info := range wfInfos {
			if !seenPorts[info.DefaultName] {
				seenPorts[info.DefaultName] = true
				results = append(results, info)
			}
		}
	}

	return results, nil
}

// extractInstrumentInfoFromWaveform extracts instrument info from a single waveform's transforms.
func extractInstrumentInfoFromWaveform(wfHandle *waveform.Handle) ([]ExtractedInstrumentInfo, error) {
	transformsHandle, err := wfHandle.Transforms()
	if err != nil {
		return nil, fmt.Errorf("failed to get transforms: %w", err)
	}
	defer transformsHandle.Close()

	return extractInstrumentInfoFromListPortTransform(transformsHandle)
}

// extractInstrumentInfoFromListPortTransform extracts instrument info from a ListPortTransform.
func extractInstrumentInfoFromListPortTransform(transformsHandle *listporttransform.Handle) ([]ExtractedInstrumentInfo, error) {
	size, err := transformsHandle.Size()
	if err != nil {
		return nil, fmt.Errorf("failed to get transforms size: %w", err)
	}

	results := make([]ExtractedInstrumentInfo, 0, size)

	for i := uint64(0); i < size; i++ {
		ptHandle, err := transformsHandle.At(i)
		if err != nil {
			return nil, fmt.Errorf("failed to get transform at index %d: %w", i, err)
		}

		info, err := extractInfoFromPortTransform(ptHandle)
		ptHandle.Close()
		if err != nil {
			// Non-fatal: skip transforms we can't extract
			continue
		}

		results = append(results, info)
	}

	return results, nil
}

// extractInfoFromPortTransform extracts instrument info from a PortTransform.
func extractInfoFromPortTransform(ptHandle *porttransform.Handle) (ExtractedInstrumentInfo, error) {
	portHandle, err := ptHandle.Port()
	if err != nil {
		return ExtractedInstrumentInfo{}, fmt.Errorf("failed to get port from transform: %w", err)
	}
	defer portHandle.Close()

	return extractInfoFromInstrumentPort(portHandle)
}

// extractInfoFromInstrumentPort extracts instrument info from an InstrumentPort.
func extractInfoFromInstrumentPort(portHandle *instrumentport.Handle) (ExtractedInstrumentInfo, error) {
	info := ExtractedInstrumentInfo{}

	defaultName, err := portHandle.DefaultName()
	if err != nil {
		return info, fmt.Errorf("failed to get default name: %w", err)
	}
	info.DefaultName = defaultName

	instrumentFacingName, err := portHandle.InstrumentFacingName()
	if err != nil {
		return info, fmt.Errorf("failed to get instrument facing name: %w", err)
	}
	info.InstrumentFacingName = instrumentFacingName

	instrumentType, err := portHandle.InstrumentType()
	if err != nil {
		return info, fmt.Errorf("failed to get instrument type: %w", err)
	}
	info.InstrumentType = instrumentType

	isKnob, err := portHandle.IsKnob()
	if err != nil {
		return info, fmt.Errorf("failed to check if knob: %w", err)
	}
	info.IsKnob = isKnob

	isMeter, err := portHandle.IsMeter()
	if err != nil {
		return info, fmt.Errorf("failed to check if meter: %w", err)
	}
	info.IsMeter = isMeter

	description, err := portHandle.Description()
	if err != nil {
		// Description is optional, don't fail
		info.Description = ""
	} else {
		info.Description = description
	}

	return info, nil
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
