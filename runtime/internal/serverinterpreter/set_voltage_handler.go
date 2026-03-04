// Package serverinterpreter provides set_voltage command handling.
//
// This file implements the set_voltage command following the schema from
// falcon-measurement-lib/schemas/scripts/set_voltage.json
package serverinterpreter

import (
	"encoding/json"
	"fmt"
)

// SetVoltageCommand represents a set_voltage command from the schema.
// Matches falcon-measurement-lib/schemas/scripts/set_voltage.json
type SetVoltageCommand struct {
	// Setter is the instrument target to apply voltage to
	Setter *InstrumentPortTarget `json:"setter"`
	// SetVoltage is the voltage value to set in V
	SetVoltage float64 `json:"setVoltage"`
}

// InstrumentPortTarget represents reference to an instrument port.
// This mirrors the InstrumentTarget from falcon-measurement-lib.
type InstrumentPortTarget struct {
	// InstrumentID is the unique identifier for the instrument
	InstrumentID string `json:"instrument_id"`
	// Channel is optional channel number
	Channel *int `json:"channel,omitempty"`
	// PortName is optional port name for named ports
	PortName string `json:"port_name,omitempty"`
}

// GetChannel returns the channel, defaulting to 0 if not specified.
func (t *InstrumentPortTarget) GetChannel() int {
	if t.Channel != nil {
		return *t.Channel
	}
	return 0
}

// Serialize returns the target as a string for lookup.
func (t *InstrumentPortTarget) Serialize() string {
	ch := t.GetChannel()
	if ch != 0 {
		return fmt.Sprintf("%s:%d", t.InstrumentID, ch)
	}
	return t.InstrumentID
}

// SetVoltageHandler handles set_voltage commands.
type SetVoltageHandler struct {
	bridge *Bridge
}

// NewSetVoltageHandler creates a new handler for set_voltage commands.
func NewSetVoltageHandler(bridge *Bridge) *SetVoltageHandler {
	return &SetVoltageHandler{bridge: bridge}
}

// ExecuteFromJSON executes a set_voltage command from JSON.
func (h *SetVoltageHandler) ExecuteFromJSON(jsonStr string) (*ExecutionResult, error) {
	var cmd SetVoltageCommand
	if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
		return nil, fmt.Errorf("failed to parse set_voltage command: %w", err)
	}

	return h.Execute(cmd)
}

// Execute executes a set_voltage command.
func (h *SetVoltageHandler) Execute(cmd SetVoltageCommand) (*ExecutionResult, error) {
	if cmd.Setter == nil {
		return nil, fmt.Errorf("setter is required")
	}

	return h.bridge.ExecuteSetVoltage(
		cmd.Setter.InstrumentID,
		cmd.Setter.GetChannel(),
		cmd.SetVoltage,
	)
}

// ExecuteBatch executes multiple set_voltage commands.
func (h *SetVoltageHandler) ExecuteBatch(cmds []SetVoltageCommand) ([]ExecutionResult, error) {
	results := make([]ExecutionResult, 0, len(cmds))

	for i, cmd := range cmds {
		result, err := h.Execute(cmd)
		if err != nil {
			results = append(results, ExecutionResult{
				Status: "failed",
				Error:  fmt.Sprintf("command %d failed: %v", i, err),
			})
			continue
		}
		results = append(results, *result)
	}

	return results, nil
}

// SetVoltageResult represents the result of a set_voltage operation.
type SetVoltageResult struct {
	// Success indicates if the operation succeeded
	Success bool `json:"success"`
	// Setter is the target that was set
	Setter *InstrumentPortTarget `json:"setter"`
	// SetVoltage is the voltage that was set
	SetVoltage float64 `json:"setVoltage"`
	// ActualVoltage is the confirmed voltage reading (if available)
	ActualVoltage *float64 `json:"actualVoltage,omitempty"`
	// Error message if not successful
	Error string `json:"error,omitempty"`
}

// ToJSON converts the result to JSON.
func (r *SetVoltageResult) ToJSON() (string, error) {
	data, err := json.Marshal(r)
	return string(data), err
}
