//go:build !falcon_core

// Package serverinterpreter provides stub implementations when falcon-core is not available.
//
// This file is compiled when the falcon_core build tag is NOT set.
// Unlike a pure stub that returns errors, this provides working implementations
// that parse JSON directly, allowing tests to run without the falcon-core C library.
//
// To enable real falcon-core CGO integration, build with:
//
//	go build -tags falcon_core
package serverinterpreter

import (
	"encoding/json"
	"fmt"
)

// FalconMeasurementRequest is a pure-Go implementation for when falcon-core is not available.
// It parses JSON directly rather than using the falcon-core C library.
type FalconMeasurementRequest struct {
	rawJSON    string
	parsedData map[string]interface{}
}

// NewFalconMeasurementRequestFromJSON creates a request by parsing JSON directly.
// This allows testing without falcon-core installed.
func NewFalconMeasurementRequestFromJSON(jsonStr string) (*FalconMeasurementRequest, error) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &FalconMeasurementRequest{
		rawJSON:    jsonStr,
		parsedData: parsed,
	}, nil
}

// Close is a no-op for the pure-Go implementation.
func (r *FalconMeasurementRequest) Close() error {
	return nil
}

// Handle returns nil for the pure-Go implementation.
func (r *FalconMeasurementRequest) Handle() interface{} {
	return nil
}

// ToJSON returns the original JSON string.
func (r *FalconMeasurementRequest) ToJSON() (string, error) {
	if r.rawJSON != "" {
		return r.rawJSON, nil
	}
	data, err := json.Marshal(r.parsedData)
	return string(data), err
}

// Message extracts the message field from the parsed JSON.
func (r *FalconMeasurementRequest) Message() (string, error) {
	if msg, ok := r.parsedData["message"].(string); ok {
		return msg, nil
	}
	return "", nil
}

// MeasurementName extracts the measurement_name field from the parsed JSON.
func (r *FalconMeasurementRequest) MeasurementName() (string, error) {
	// Try both formats
	if name, ok := r.parsedData["measurement_name"].(string); ok {
		return name, nil
	}
	if name, ok := r.parsedData["measurementName"].(string); ok {
		return name, nil
	}
	return "", nil
}

// ExtractNumPoints attempts to extract the number of sweep points from the
// first waveform in the parsed JSON. Returns 100 if it cannot be found.
func (r *FalconMeasurementRequest) ExtractNumPoints() (int, error) {
	waveforms, ok := r.parsedData["waveforms"].([]interface{})
	if !ok || len(waveforms) == 0 {
		return 100, nil
	}
	wf, ok := waveforms[0].(map[string]interface{})
	if !ok {
		return 100, nil
	}
	for _, key := range []string{"divisions", "num_points"} {
		if v, ok := wf[key]; ok {
			switch n := v.(type) {
			case float64:
				if int(n) > 0 {
					return int(n), nil
				}
			case int:
				if n > 0 {
					return n, nil
				}
			}
		}
	}
	return 100, nil
}

// RawData returns the parsed JSON data for direct access.
func (r *FalconMeasurementRequest) RawData() map[string]interface{} {
	return r.parsedData
}

// ExtractedInstrumentInfo contains information extracted from an InstrumentPort.
type ExtractedInstrumentInfo struct {
	DefaultName          string
	InstrumentFacingName string
	InstrumentType       string
	IsKnob               bool
	IsMeter              bool
	Description          string
	PortJSON             string // The original JSON for this port
	ConnectionJSON       string // JSON serialization of the port's pseudo-name (connection)
	UnitsJSON            string // JSON serialization of the port's units
}

// ExtractGetters extracts getter info from the parsed JSON.
func (r *FalconMeasurementRequest) ExtractGetters() ([]ExtractedInstrumentInfo, error) {
	return extractPortsFromJSON(r.parsedData, "getters")
}

// ExtractSetters extracts setter info from waveforms in the parsed JSON.
func (r *FalconMeasurementRequest) ExtractSetters() ([]ExtractedInstrumentInfo, error) {
	// Try to extract from waveforms -> transforms -> port (falcon-core structure)
	var results []ExtractedInstrumentInfo
	seen := make(map[string]bool)

	// First try waveforms structure
	waveforms, ok := r.parsedData["waveforms"].([]interface{})
	if ok {
		for _, wf := range waveforms {
			wfMap, ok := wf.(map[string]interface{})
			if !ok {
				continue
			}

			transforms, ok := wfMap["transforms"].([]interface{})
			if !ok {
				continue
			}

			for _, t := range transforms {
				tMap, ok := t.(map[string]interface{})
				if !ok {
					continue
				}

				port, ok := tMap["port"].(map[string]interface{})
				if !ok {
					continue
				}

				info := extractInfoFromPortMap(port)
				key := info.PortJSON
				if key == "" {
					key = info.ConnectionJSON
				}
				if key == "" {
					key = info.DefaultName
				}
				if !seen[key] {
					seen[key] = true
					results = append(results, info)
				}
			}
		}
	}

	// Then try simple "setters" array (simplified format)
	if len(results) == 0 {
		setters, err := extractPortsFromJSON(r.parsedData, "setters")
		if err == nil {
			results = setters
		}
	}

	return results, nil
}

// extractPortsFromJSON extracts port info from a field in the parsed JSON.
func extractPortsFromJSON(data map[string]interface{}, field string) ([]ExtractedInstrumentInfo, error) {
	var results []ExtractedInstrumentInfo

	ports, ok := data[field].([]interface{})
	if !ok {
		// Try as object with "ports" field
		if portsObj, ok := data[field].(map[string]interface{}); ok {
			ports, _ = portsObj["ports"].([]interface{})
		}
	}

	for _, p := range ports {
		portMap, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		results = append(results, extractInfoFromPortMap(portMap))
	}

	return results, nil
}

// extractInfoFromPortMap extracts ExtractedInstrumentInfo from a port map.
func extractInfoFromPortMap(port map[string]interface{}) ExtractedInstrumentInfo {
	info := ExtractedInstrumentInfo{}

	if name, ok := port["default_name"].(string); ok {
		info.DefaultName = name
	}
	// Also try "id" for simplified format
	if info.DefaultName == "" {
		if id, ok := port["id"].(string); ok {
			info.DefaultName = id
		}
	}
	if name, ok := port["instrument_facing_name"].(string); ok {
		info.InstrumentFacingName = name
	}
	if t, ok := port["instrument_type"].(string); ok {
		info.InstrumentType = t
	}
	if isKnob, ok := port["is_knob"].(bool); ok {
		info.IsKnob = isKnob
	}
	if isMeter, ok := port["is_meter"].(bool); ok {
		info.IsMeter = isMeter
	}
	if desc, ok := port["description"].(string); ok {
		info.Description = desc
	}

	// Store original JSON
	if jsonBytes, err := json.Marshal(port); err == nil {
		info.PortJSON = string(jsonBytes)
	}

	// Populate ConnectionJSON from connection or pseudo_name field.
	for _, key := range []string{"connection", "pseudo_name"} {
		if conn, ok := port[key]; ok {
			if jsonBytes, err := json.Marshal(conn); err == nil {
				info.ConnectionJSON = string(jsonBytes)
				break
			}
		}
	}

	// Populate UnitsJSON from units field.
	if units, ok := port["units"]; ok {
		if jsonBytes, err := json.Marshal(units); err == nil {
			info.UnitsJSON = string(jsonBytes)
		}
	}

	return info
}

// FalconMeasurementResponse is a pure-Go implementation.
type FalconMeasurementResponse struct {
	data map[string]interface{}
}

// NewFalconMeasurementResponseFromJSON creates a response by parsing JSON directly.
func NewFalconMeasurementResponseFromJSON(jsonStr string) (*FalconMeasurementResponse, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, err
	}
	return &FalconMeasurementResponse{data: data}, nil
}

// Close is a no-op.
func (r *FalconMeasurementResponse) Close() error {
	return nil
}

// Handle returns nil.
func (r *FalconMeasurementResponse) Handle() interface{} {
	return nil
}

// ToJSON serializes the response.
func (r *FalconMeasurementResponse) ToJSON() (string, error) {
	data, err := json.Marshal(r.data)
	return string(data), err
}

// Message returns the message field.
func (r *FalconMeasurementResponse) Message() (string, error) {
	if msg, ok := r.data["message"].(string); ok {
		return msg, nil
	}
	return "", nil
}

// GettersToJSONList returns getter port JSONs.
// of the cereal-serialized MeasurementRequest.
//
// The cereal JSON encodes the discrete space as a normalized float array [0.0..1.0]
// inside the Waveform's DiscreteSpace._space, and the voltage domain bounds
// (min, max) inside DiscreteSpace._axes → CoupledLabelledDomain → LabelledDomain → Domain.
//
// Voltage at step i = domainMin + normalizedArr[i] * (domainMax - domainMin)
// The array has division+1 elements; we take the first division (= NUM_POINTS) steps.
func ExtractWaveformDataFromRequest(req *FalconMeasurementRequest) (*WaveformData, []GetterInfo, error) {
	if req == nil {
		return nil, nil, fmt.Errorf("request is nil")
	}

	// Extract getters
	getterInfos, err := req.ExtractGetters()
	if err != nil {
		return nil, nil, err
	}
	getters := make([]GetterInfo, len(getterInfos))
	for i, g := range getterInfos {
		getters[i] = GetterInfo{PortJSON: g.PortJSON}
	}

	// Navigate to Waveform[0] data
	// Path: value0.ptr_wrapper.data.value2.ptr_wrapper.data.value1[0].ptr_wrapper.data
	wf0 := navJSON(req.parsedData,
		"value0", "ptr_wrapper", "data",
		"value2", "ptr_wrapper", "data",
		"value1", 0, "ptr_wrapper", "data")
	if wf0 == nil {
		return stubWaveformData(), getters, nil
	}

	// Navigate to DiscreteSpace (Waveform.value1)
	ds := navJSON(wf0, "value1", "ptr_wrapper", "data")
	if ds == nil {
		return stubWaveformData(), getters, nil
	}

	// Extract normalized float array from DiscreteSpace._space (value1)
	// Path within ds: value1.ptr_wrapper.data.value2.ptr_wrapper.data.value1[0].ptr_wrapper.data.value0.value0.value1.value1
	rawArr := navJSON(ds,
		"value1", "ptr_wrapper", "data",
		"value2", "ptr_wrapper", "data",
		"value1", 0, "ptr_wrapper", "data",
		"value0", "value0", "value1", "value1")
	normalizedSlice, ok := rawArr.([]interface{})
	if !ok || len(normalizedSlice) < 2 {
		return stubWaveformData(), getters, nil
	}

	// Extract domain bounds from DiscreteSpace._axes (value2)
	// Path within ds: value2.ptr_wrapper.data.value1[0].ptr_wrapper.data.value1[0].ptr_wrapper.data.value0
	// Domain.value1 = lesser_bound (min), Domain.value2 = greater_bound (max)
	domainObj := navJSON(ds,
		"value2", "ptr_wrapper", "data",
		"value1", 0, "ptr_wrapper", "data",
		"value1", 0, "ptr_wrapper", "data",
		"value0")
	domainMin := 0.0
	domainMax := 1.0
	if dm, ok := domainObj.(map[string]interface{}); ok {
		domainMin = convertToFloat64(dm["value1"])
		domainMax = convertToFloat64(dm["value2"])
	}

	// Build voltage sweep: normalizedSlice has division+1 elements;
	// take the first division steps (half-open interval [min, max)).
	numPoints := len(normalizedSlice) - 1
	rawTimeTrace := make([][]float64, numPoints)
	for i := 0; i < numPoints; i++ {
		t := convertToFloat64(normalizedSlice[i])
		voltage := domainMin + t*(domainMax-domainMin)
		rawTimeTrace[i] = []float64{voltage}
	}

	return &WaveformData{
		RawTimeTrace: rawTimeTrace,
		AxisDomains:  [][]LabelledDomainInfo{},
		TimeDomain:   DomainBounds{Min: domainMin, Max: domainMax},
		Shape:        []int{numPoints},
	}, getters, nil
}

// GettersToJSONList returns getter port JSONs.
func GettersToJSONList(req *FalconMeasurementRequest) ([]string, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	getters, err := req.ExtractGetters()
	if err != nil {
		return nil, err
	}

	result := make([]string, len(getters))
	for i, g := range getters {
		result[i] = g.PortJSON
	}
	return result, nil
}

// SettersToJSONList returns setter port JSONs.
func SettersToJSONList(req *FalconMeasurementRequest) ([]string, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	setters, err := req.ExtractSetters()
	if err != nil {
		return nil, err
	}

	result := make([]string, len(setters))
	for i, s := range setters {
		result[i] = s.PortJSON
	}
	return result, nil
}

// ExtractWaveformDataFromRequestByIndex extracts waveform data for the waveform at wfIndex.
// Use wfIndex=0 for the fast axis and wfIndex=1 for the slow axis in 2D sweeps.
func ExtractWaveformDataFromRequestByIndex(req *FalconMeasurementRequest, wfIndex int) (*WaveformData, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	wf := navJSON(req.parsedData,
		"value0", "ptr_wrapper", "data",
		"value2", "ptr_wrapper", "data",
		"value1", wfIndex, "ptr_wrapper", "data")
	if wf == nil {
		return stubWaveformData(), nil
	}

	ds := navJSON(wf, "value1", "ptr_wrapper", "data")
	if ds == nil {
		return stubWaveformData(), nil
	}

	rawArr := navJSON(ds,
		"value1", "ptr_wrapper", "data",
		"value2", "ptr_wrapper", "data",
		"value1", 0, "ptr_wrapper", "data",
		"value0", "value0", "value1", "value1")
	normalizedSlice, ok := rawArr.([]interface{})
	if !ok || len(normalizedSlice) < 2 {
		return stubWaveformData(), nil
	}

	domainObj := navJSON(ds,
		"value2", "ptr_wrapper", "data",
		"value1", 0, "ptr_wrapper", "data",
		"value1", 0, "ptr_wrapper", "data",
		"value0")
	domainMin := 0.0
	domainMax := 1.0
	if dm, ok := domainObj.(map[string]interface{}); ok {
		domainMin = convertToFloat64(dm["value1"])
		domainMax = convertToFloat64(dm["value2"])
	}

	numPoints := len(normalizedSlice) - 1
	rawTimeTrace := make([][]float64, numPoints)
	for i := 0; i < numPoints; i++ {
		t := convertToFloat64(normalizedSlice[i])
		voltage := domainMin + t*(domainMax-domainMin)
		rawTimeTrace[i] = []float64{voltage}
	}

	return &WaveformData{
		RawTimeTrace: rawTimeTrace,
		AxisDomains:  [][]LabelledDomainInfo{},
		TimeDomain:   DomainBounds{Min: domainMin, Max: domainMax},
		Shape:        []int{numPoints},
	}, nil
}
