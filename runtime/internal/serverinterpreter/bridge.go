// Package serverinterpreter provides the main bridge between falcon MeasurementRequest
// objects and the instrument-script-server.
//
// There are two modes of operation:
//  1. Direct mode (Bridge): Uses HTTP RPC to communicate directly with instrument-script-server
//  2. Internal API mode (InterpreterDaemon): Uses NATS internal messaging aligned with falcon-api specs
package serverinterpreter

import (
	"encoding/json"
	"fmt"
	"time"
)

// Bridge orchestrates the conversion of falcon MeasurementRequest objects
// to instrument-script-server commands and handles the response flow.
type Bridge struct {
	client    *ScriptServerClient
	generator *ScriptGenerator
}

// BridgeConfig holds configuration for the Bridge.
type BridgeConfig struct {
	// ScriptServerHost is the host address of the instrument-script-server.
	ScriptServerHost string
	// ScriptServerPort is the port of the instrument-script-server RPC API.
	ScriptServerPort int
	// ScriptOutputDir is the directory where generated Lua scripts are stored.
	ScriptOutputDir string
}

// DefaultBridgeConfig returns a default configuration.
func DefaultBridgeConfig() BridgeConfig {
	return BridgeConfig{
		ScriptServerHost: "127.0.0.1",
		ScriptServerPort: 8555,
		ScriptOutputDir:  "/tmp/falcon-scripts",
	}
}

// NewBridge creates a new Bridge with the given configuration.
func NewBridge(config BridgeConfig) (*Bridge, error) {
	generator, err := NewScriptGenerator(config.ScriptOutputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create script generator: %w", err)
	}

	client := NewScriptServerClient(config.ScriptServerHost, config.ScriptServerPort)

	return &Bridge{
		client:    client,
		generator: generator,
	}, nil
}

// ExecutionResult holds the result of executing a measurement request.
type ExecutionResult struct {
	JobID     string                 `json:"job_id"`
	Status    string                 `json:"status"`
	Results   []MeasurementResponse  `json:"results,omitempty"`
	RawResult map[string]interface{} `json:"raw_result,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// ExecuteMeasurementRequestJSON takes a falcon MeasurementRequest in JSON format,
// generates the appropriate Lua script, submits it to the instrument-script-server,
// waits for completion, and returns the results.
func (b *Bridge) ExecuteMeasurementRequestJSON(jsonStr string) (*ExecutionResult, error) {
	// Parse the measurement request JSON
	parsed, err := ParseMeasurementRequestJSON(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse measurement request: %w", err)
	}

	return b.ExecuteParsedRequest(parsed)
}

// ExecuteMeasurementRequestWithFalconCore uses falcon-core bindings to parse the request.
// This provides proper integration with the falcon-core type system.
func (b *Bridge) ExecuteMeasurementRequestWithFalconCore(jsonStr string) (*ExecutionResult, error) {
	// Parse using falcon-core
	falconReq, err := NewFalconMeasurementRequestFromJSON(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse with falcon-core: %w", err)
	}
	defer falconReq.Close()

	// Extract setters and getters using falcon-core API
	setters, err := falconReq.ExtractSetters()
	if err != nil {
		return nil, fmt.Errorf("failed to extract setters: %w", err)
	}

	getters, err := falconReq.ExtractGetters()
	if err != nil {
		return nil, fmt.Errorf("failed to extract getters: %w", err)
	}

	name, _ := falconReq.MeasurementName()

	// Convert to ParsedMeasurementRequest for script generation
	parsed := &ParsedMeasurementRequest{
		MeasurementName: name,
		Setters:         make([]InstrumentTarget, 0),
		Getters:         make([]InstrumentTarget, 0),
		SetVoltages:     make(map[string]float64),
	}

	for _, s := range setters {
		target := InstrumentTarget{
			Id:      s.InstrumentType,
			Channel: 0, // Would need to parse from DefaultName
		}
		parsed.Setters = append(parsed.Setters, target)
	}

	for _, g := range getters {
		target := InstrumentTarget{
			Id:      g.InstrumentType,
			Channel: 0,
		}
		parsed.Getters = append(parsed.Getters, target)
	}

	return b.ExecuteParsedRequest(parsed)
}

// ExecuteParsedRequest executes a parsed measurement request.
func (b *Bridge) ExecuteParsedRequest(req *ParsedMeasurementRequest) (*ExecutionResult, error) {
	// Generate the Lua script(s)
	scriptPaths, err := b.generator.GenerateFromParsedRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate script: %w", err)
	}

	if len(scriptPaths) == 0 {
		return nil, fmt.Errorf("no scripts generated from measurement request")
	}

	// For now, execute only the first script
	// In a more complete implementation, we might want to execute all scripts
	scriptPath := scriptPaths[0]

	// Submit the script to the instrument-script-server
	jobID, err := b.client.SubmitMeasure(scriptPath)
	if err != nil {
		return &ExecutionResult{
			Status: "failed",
			Error:  fmt.Sprintf("failed to submit measurement: %v", err),
		}, nil
	}

	// Wait for the job to complete
	status, err := b.client.WaitForJob(jobID, 100*time.Millisecond, 30*time.Second)
	if err != nil {
		return &ExecutionResult{
			JobID:  jobID,
			Status: "timeout",
			Error:  fmt.Sprintf("timeout waiting for job: %v", err),
		}, nil
	}

	result := &ExecutionResult{
		JobID:  jobID,
		Status: status,
	}

	// If completed successfully, get the results
	if status == "completed" {
		rawResult, err := b.client.JobResult(jobID)
		if err != nil {
			result.Error = fmt.Sprintf("failed to get job result: %v", err)
		} else {
			if resultMap, ok := rawResult.(map[string]interface{}); ok {
				result.RawResult = resultMap
				// Try to parse results into MeasurementResponse objects
				result.Results = parseResults(resultMap)
			}
		}
	} else if status == "failed" || status == "canceled" {
		result.Error = fmt.Sprintf("job %s", status)
	}

	return result, nil
}

// ExecuteSetVoltage is a convenience method to execute a single set_voltage operation.
func (b *Bridge) ExecuteSetVoltage(instrumentID string, channel int, voltage float64) (*ExecutionResult, error) {
	req := &ParsedMeasurementRequest{
		MeasurementName: fmt.Sprintf("set_voltage_%s_%d", instrumentID, channel),
		Setters: []InstrumentTarget{
			{Id: instrumentID, Channel: channel},
		},
		SetVoltages: map[string]float64{
			InstrumentTarget{Id: instrumentID, Channel: channel}.Serialize(): voltage,
		},
	}

	return b.ExecuteParsedRequest(req)
}

// ExecuteGetVoltage is a convenience method to execute a single get_voltage operation.
func (b *Bridge) ExecuteGetVoltage(instrumentID string, channel int) (*ExecutionResult, error) {
	req := &ParsedMeasurementRequest{
		MeasurementName: fmt.Sprintf("get_voltage_%s_%d", instrumentID, channel),
		Getters: []InstrumentTarget{
			{Id: instrumentID, Channel: channel},
		},
	}

	return b.ExecuteParsedRequest(req)
}

// ToSerializedResponse converts an ExecutionResult back to a JSON string
// suitable for sending back to falcon over NATS.
func (r *ExecutionResult) ToSerializedResponse() (string, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to serialize execution result: %w", err)
	}
	return string(data), nil
}

// parseResults attempts to parse raw results into MeasurementResponse objects.
func parseResults(raw map[string]interface{}) []MeasurementResponse {
	var responses []MeasurementResponse

	// Try to find results array in the raw response
	if results, ok := raw["results"].([]interface{}); ok {
		for _, r := range results {
			if resultMap, ok := r.(map[string]interface{}); ok {
				resp := MeasurementResponse{}

				if inst, ok := resultMap["instrument"].(string); ok {
					resp.Instrument = inst
				}
				if verb, ok := resultMap["verb"].(string); ok {
					resp.Verb = verb
				}
				if typeStr, ok := resultMap["type"].(string); ok {
					resp.Type = typeStr
				}
				if val, ok := resultMap["return"].(map[string]interface{}); ok {
					resp.Value = val["value"]
					if t, ok := val["type"].(string); ok && resp.Type == "" {
						resp.Type = t
					}
				} else {
					resp.Value = resultMap["value"]
				}

				responses = append(responses, resp)
			}
		}
	}

	return responses
}
