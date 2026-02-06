// Package scriptbridge bridges falcon-core MeasurementRequest objects
// to instrument-script-server RPC commands using falcon-measurement-lib types.
package scriptbridge

import (
	"encoding/json"
	"fmt"
)

// InstrumentTarget represents a reference to an instrument, optionally with a channel.
// This mirrors the generated type from falcon-measurement-lib.
type InstrumentTarget struct {
	Id      string `json:"id"`      // Instrument identifier (e.g., "GPI1")
	Channel int    `json:"channel"` // Optional channel number
}

// Serialize returns the instrument target as a string in the format "id" or "id:channel"
func (t InstrumentTarget) Serialize() string {
	if t.Channel != 0 {
		return fmt.Sprintf("%s:%d", t.Id, t.Channel)
	}
	return t.Id
}

// SetVoltageRequest is the request structure for setting a voltage.
// Matches falcon-measurement-lib/schemas/scripts/set_voltage.json
type SetVoltageRequest struct {
	Setter     InstrumentTarget `json:"setter"`     // The instrument (and channel) to set the applied voltage to
	SetVoltage float64          `json:"setVoltage"` // The voltage value to set in V
}

// GetVoltageRequest is the request structure for getting a voltage.
// Matches falcon-measurement-lib/schemas/scripts/get_voltage.json
type GetVoltageRequest struct {
	Getter InstrumentTarget `json:"getter"` // The instrument (and channel) to collect the applied voltage from
}

// MeasurementResponse is the canonical response wrapper from instrument operations.
type MeasurementResponse struct {
	Instrument string      `json:"instrument"` // Instrument name
	Verb       string      `json:"verb"`       // Command name
	Type       string      `json:"type"`       // Value type (float, integer, string, boolean, buffer)
	Value      interface{} `json:"value"`      // The recorded value
}

// MeasurementResponseNumber is a typed MeasurementResponse for numeric values.
type MeasurementResponseNumber struct {
	Instrument string  `json:"instrument"`
	Verb       string  `json:"verb"`
	Type       string  `json:"type"`
	Value      float64 `json:"value"`
}

// RPCRequest is the structure for HTTP RPC requests to instrument-script-server.
type RPCRequest struct {
	Command string      `json:"command"` // Command name (e.g., "submit_measure")
	Params  interface{} `json:"params"`  // Command-specific parameters
}

// RPCResponse is the structure for HTTP RPC responses from instrument-script-server.
type RPCResponse struct {
	OK     bool        `json:"ok"`              // Whether the request succeeded
	Error  string      `json:"error,omitempty"` // Error message if failed
	JobID  string      `json:"job_id,omitempty"`
	Status string      `json:"status,omitempty"`
	Result interface{} `json:"result,omitempty"`
}

// SubmitMeasureParams are the parameters for the submit_measure RPC command.
type SubmitMeasureParams struct {
	ScriptPath string `json:"script_path"` // Path to Lua measurement script
}

// JobStatusParams are the parameters for the job_status RPC command.
type JobStatusParams struct {
	JobID string `json:"job_id"` // Job identifier
}

// JobResultParams are the parameters for the job_result RPC command.
type JobResultParams struct {
	JobID string `json:"job_id"` // Job identifier
}

// ParsedMeasurementRequest represents a falcon MeasurementRequest after JSON parsing.
// This is a simplified representation focusing on the voltage-related operations.
type ParsedMeasurementRequest struct {
	Message         string                 `json:"message"`
	MeasurementName string                 `json:"measurementName"`
	Setters         []InstrumentTarget     `json:"setters,omitempty"`
	Getters         []InstrumentTarget     `json:"getters,omitempty"`
	SetVoltages     map[string]float64     `json:"setVoltages,omitempty"`
	RawJSON         map[string]interface{} `json:"-"` // Original JSON for inspection
}

// ParseMeasurementRequestJSON attempts to parse a falcon MeasurementRequest JSON string
// into a simplified structure for processing.
func ParseMeasurementRequestJSON(jsonStr string) (*ParsedMeasurementRequest, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse measurement request JSON: %w", err)
	}

	result := &ParsedMeasurementRequest{
		RawJSON:     raw,
		SetVoltages: make(map[string]float64),
	}

	// Extract message if present
	if msg, ok := raw["message"].(string); ok {
		result.Message = msg
	}

	// Extract measurement name if present
	if name, ok := raw["measurementName"].(string); ok {
		result.MeasurementName = name
	}

	// Try to extract setters
	if setters, ok := raw["setters"].([]interface{}); ok {
		for _, s := range setters {
			if setter, ok := s.(map[string]interface{}); ok {
				target := InstrumentTarget{}
				if id, ok := setter["id"].(string); ok {
					target.Id = id
				}
				if ch, ok := setter["channel"].(float64); ok {
					target.Channel = int(ch)
				}
				result.Setters = append(result.Setters, target)
			}
		}
	}

	// Try to extract getters
	if getters, ok := raw["getters"].([]interface{}); ok {
		for _, g := range getters {
			if getter, ok := g.(map[string]interface{}); ok {
				target := InstrumentTarget{}
				if id, ok := getter["id"].(string); ok {
					target.Id = id
				}
				if ch, ok := getter["channel"].(float64); ok {
					target.Channel = int(ch)
				}
				result.Getters = append(result.Getters, target)
			}
		}
	}

	// Try to extract setVoltages
	if voltages, ok := raw["setVoltages"].(map[string]interface{}); ok {
		for k, v := range voltages {
			if voltage, ok := v.(float64); ok {
				result.SetVoltages[k] = voltage
			}
		}
	}

	return result, nil
}

// ToSetVoltageRequests converts a ParsedMeasurementRequest into a slice of SetVoltageRequest
// by matching setters with their corresponding voltages.
func (p *ParsedMeasurementRequest) ToSetVoltageRequests() []SetVoltageRequest {
	var requests []SetVoltageRequest

	for _, setter := range p.Setters {
		key := setter.Serialize()
		if voltage, ok := p.SetVoltages[key]; ok {
			requests = append(requests, SetVoltageRequest{
				Setter:     setter,
				SetVoltage: voltage,
			})
		}
	}

	return requests
}

// ToGetVoltageRequests converts a ParsedMeasurementRequest into a slice of GetVoltageRequest.
func (p *ParsedMeasurementRequest) ToGetVoltageRequests() []GetVoltageRequest {
	var requests []GetVoltageRequest

	for _, getter := range p.Getters {
		requests = append(requests, GetVoltageRequest{
			Getter: getter,
		})
	}

	return requests
}
