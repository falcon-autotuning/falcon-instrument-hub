// Package serverinterpreter bridges falcon-core MeasurementRequest objects
// to instrument-script-server RPC commands using falcon-measurement-lib types.
package serverinterpreter

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
// Fields are top-level in the ISS wire format (not nested under a "result" key).
type RPCResponse struct {
	OK           bool            `json:"ok"`                     // Whether the request succeeded
	Error        string          `json:"error,omitempty"`        // Error message if failed
	JobID        string          `json:"job_id,omitempty"`       // Returned by submit_measure
	Status       string          `json:"status,omitempty"`       // Returned by job_status
	Result       interface{}     `json:"result,omitempty"`       // Returned by job_result (collect_results_json array)
	Instruments  []string        `json:"instruments,omitempty"`  // Returned by list
	Instrument   string          `json:"instrument,omitempty"`   // Returned by start
	// read_buffer response fields
	BufferID     string          `json:"buffer_id,omitempty"`
	ElementCount int             `json:"element_count,omitempty"`
	Data         []float64       `json:"data,omitempty"`
	DataType     string          `json:"data_type,omitempty"`
	// measure command response fields
	Script       string          `json:"script,omitempty"`
	Results      json.RawMessage `json:"results,omitempty"`
}

// ISSCallResult is a single entry in the results array returned by the ISS
// synchronous `measure` command (collect_results_json).
type ISSCallResult struct {
	Index        int             `json:"index"`
	Instrument   string          `json:"instrument"`
	Verb         string          `json:"verb"`
	ExecutedAtMs int64           `json:"executed_at_ms"`
	Return       ISSReturnValue  `json:"return"`
}

// ISSReturnValue is the return field of an ISSCallResult.
type ISSReturnValue struct {
	Type        string          `json:"type"`                  // "float","integer","string","boolean","buffer","void"
	Value       interface{}     `json:"value,omitempty"`       // scalar value
	BufferID    string          `json:"buffer_id,omitempty"`   // set when type=="buffer"
	ElementCount int            `json:"element_count,omitempty"`
	DataType    string          `json:"data_type,omitempty"`
}

// ---------------------------------------------------------------------------
// Script data types — used by quantum_dot_device.go, averaged_sweep_manager.go
// and elsewhere to describe measurement parameters.
// ---------------------------------------------------------------------------

// Sweep1DScriptData describes the parameters for a 1D voltage sweep.
type Sweep1DScriptData struct {
	MeasurementName    string              // Human-readable name
	SweepGate          string              // Name of the gate being swept (e.g., "P1")
	SweepSetter        InstrumentTarget    // The DAC/channel for the sweep gate
	StartVoltage       float64             // Starting voltage
	StopVoltage        float64             // Ending voltage
	NumPoints          int                 // Number of points in sweep
	SettlingTimeMs     float64             // Time to wait after setting voltage (ms)
	StaticSetters      []SetVoltageRequest // Static gate voltages during sweep
	GetVoltageRequests []GetVoltageRequest // Measurement channels (e.g., DMM for current)
}

// AveragedSweep1DScriptData describes the parameters for an N-averaged 1D sweep.
type AveragedSweep1DScriptData struct {
	MeasurementName    string              // Human-readable name
	MeasurementID      string              // Unique ID for trace buffering
	SweepGate          string              // Name of gate being swept
	SweepSetter        InstrumentTarget    // DAC/channel for sweep gate
	StartVoltage       float64             // Start voltage
	StopVoltage        float64             // End voltage
	NumPoints          int                 // Points per sweep
	NumAverages        int                 // Number of sweeps to average
	SettlingTimeMs     float64             // Settling time after each set
	StaticSetters      []SetVoltageRequest // Static gate voltages
	GetVoltageRequests []GetVoltageRequest // Measurement channels
}

// SetVoltageScriptData describes parameters for set_voltage operations.
type SetVoltageScriptData struct {
	MeasurementName    string
	SetVoltageRequests []SetVoltageRequest
}

// GetVoltageScriptData describes parameters for get_voltage operations.
type GetVoltageScriptData struct {
	MeasurementName    string
	GetVoltageRequests []GetVoltageRequest
}

// MeasureGetSetScriptData describes parameters for combined set/get operations.
type MeasureGetSetScriptData struct {
	MeasurementName    string
	SetVoltageRequests []SetVoltageRequest
	GetVoltageRequests []GetVoltageRequest
}

// DCGetSetScriptData describes parameters for DC get/set measurement operations.
type DCGetSetScriptData struct {
	MeasurementName    string
	SetVoltageRequests []SetVoltageRequest
	GetVoltageRequests []GetVoltageRequest
	SettlingTimeMs     float64
}
