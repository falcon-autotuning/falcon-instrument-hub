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
				if !seen[info.DefaultName] {
					seen[info.DefaultName] = true
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

// ExtractWaveformDataFromRequest extracts waveform data using pure-Go JSON parsing.
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

	// Extract waveform data
	waveformData := &WaveformData{
		RawTimeTrace: [][]float64{{0.0}},
		AxisDomains:  [][]LabelledDomainInfo{},
		TimeDomain:   DomainBounds{Min: 0, Max: 0.001},
		Shape:        []int{1},
	}

	// Try to extract time domain
	if td, ok := req.parsedData["time_domain"].(map[string]interface{}); ok {
		if domain, ok := td["domain"].(map[string]interface{}); ok {
			if bounds, ok := domain["bounds"].([]interface{}); ok && len(bounds) >= 2 {
				waveformData.TimeDomain.Min = convertToFloat64(bounds[0])
				waveformData.TimeDomain.Max = convertToFloat64(bounds[1])
			}
		}
	}

	// Extract setter domains from waveforms
	setters, _ := req.ExtractSetters()
	for _, setter := range setters {
		info := LabelledDomainInfo{
			LabelJSON:    setter.PortJSON,
			DomainBounds: DomainBounds{Min: -1.0, Max: 1.0},
		}
		waveformData.AxisDomains = append(waveformData.AxisDomains, []LabelledDomainInfo{info})
	}

	return waveformData, getters, nil
}

// convertToFloat64 converts an interface{} to float64.
func convertToFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	default:
		return 0
	}
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
