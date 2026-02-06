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

// SetVoltageFromMeasurementRequest extracts set_voltage operations from a MeasurementRequest
// and generates appropriate script/command execution.
type SetVoltageFromMeasurementRequest struct {
	bridge *Bridge
}

// NewSetVoltageFromMeasurementRequest creates a helper for extracting and executing
// set_voltage commands from falcon MeasurementRequest objects.
func NewSetVoltageFromMeasurementRequest(bridge *Bridge) *SetVoltageFromMeasurementRequest {
	return &SetVoltageFromMeasurementRequest{bridge: bridge}
}

// ExtractCommands extracts SetVoltageCommand objects from a FalconMeasurementRequest.
func (s *SetVoltageFromMeasurementRequest) ExtractCommands(req *FalconMeasurementRequest) ([]SetVoltageCommand, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	var commands []SetVoltageCommand

	// Extract setters
	setters, err := req.ExtractSetters()
	if err != nil {
		return nil, fmt.Errorf("failed to extract setters: %w", err)
	}

	// Extract voltage values from raw data if available
	rawData := req.RawData()
	setVoltages := make(map[string]float64)

	if voltages, ok := rawData["setVoltages"].(map[string]interface{}); ok {
		for k, v := range voltages {
			if voltage, ok := v.(float64); ok {
				setVoltages[k] = voltage
			}
		}
	}

	// Also try to extract from waveform transforms
	if waveforms, ok := rawData["waveforms"].([]interface{}); ok {
		for _, wf := range waveforms {
			wfMap, ok := wf.(map[string]interface{})
			if !ok {
				continue
			}

			// Extract constant value if present
			if constVal, ok := wfMap["constant_value"].(float64); ok {
				if transforms, ok := wfMap["transforms"].([]interface{}); ok {
					for _, t := range transforms {
						tMap, ok := t.(map[string]interface{})
						if !ok {
							continue
						}
						if port, ok := tMap["port"].(map[string]interface{}); ok {
							if name, ok := port["default_name"].(string); ok {
								setVoltages[name] = constVal
							}
						}
					}
				}
			}
		}
	}

	// Create commands for each setter
	for _, setter := range setters {
		cmd := SetVoltageCommand{
			Setter: &InstrumentPortTarget{
				InstrumentID: setter.DefaultName,
			},
		}

		// Try to find voltage for this setter
		if voltage, ok := setVoltages[setter.DefaultName]; ok {
			cmd.SetVoltage = voltage
		} else {
			// Use 0 as default, or skip this setter
			cmd.SetVoltage = 0.0
		}

		commands = append(commands, cmd)
	}

	return commands, nil
}

// ExecuteFromRequest extracts and executes all set_voltage operations from a request.
func (s *SetVoltageFromMeasurementRequest) ExecuteFromRequest(req *FalconMeasurementRequest) ([]ExecutionResult, error) {
	commands, err := s.ExtractCommands(req)
	if err != nil {
		return nil, err
	}

	handler := NewSetVoltageHandler(s.bridge)
	return handler.ExecuteBatch(commands)
}

// SetVoltageLuaGenerator generates Lua script snippets for set_voltage operations.
type SetVoltageLuaGenerator struct{}

// NewSetVoltageLuaGenerator creates a Lua generator for set_voltage.
func NewSetVoltageLuaGenerator() *SetVoltageLuaGenerator {
	return &SetVoltageLuaGenerator{}
}

// GenerateCallStatement generates a Lua ctx:call() statement for set_voltage.
func (g *SetVoltageLuaGenerator) GenerateCallStatement(cmd SetVoltageCommand) string {
	if cmd.Setter == nil {
		return "-- Invalid: no setter specified"
	}

	return fmt.Sprintf(`ctx:call("%s.SET_VOLTAGE", {
    channel = %d,
    voltage = %.6f
})`, cmd.Setter.InstrumentID, cmd.Setter.GetChannel(), cmd.SetVoltage)
}

// GenerateParallelBlock generates a Lua parallel block for multiple set_voltage commands.
func (g *SetVoltageLuaGenerator) GenerateParallelBlock(cmds []SetVoltageCommand) string {
	if len(cmds) == 0 {
		return "-- No set_voltage commands"
	}

	if len(cmds) == 1 {
		return g.GenerateCallStatement(cmds[0])
	}

	script := "ctx:parallel(function()\n"
	for _, cmd := range cmds {
		script += "    " + g.GenerateCallStatement(cmd) + "\n"
	}
	script += "end)"

	return script
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
