// Package serverinterpreter provides script dispatch to instrument-script-server.
package serverinterpreter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"
)

// ScriptDispatcher communicates with the instrument-script-server to execute
// user-provided Lua measurement scripts.
type ScriptDispatcher struct {
	serverURL   string
	httpClient  *http.Client
	scriptsPath string // Base path where Lua scripts are stored
}

// ScriptDispatcherConfig configures the script dispatcher.
type ScriptDispatcherConfig struct {
	ServerURL      string        // HTTP URL of instrument-script-server (e.g., "http://localhost:8080")
	ScriptsPath    string        // Local directory containing Lua scripts
	RequestTimeout time.Duration // Timeout for script execution
}

// NewScriptDispatcher creates a new dispatcher.
func NewScriptDispatcher(config ScriptDispatcherConfig) *ScriptDispatcher {
	timeout := config.RequestTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute // Default timeout for long measurements
	}

	return &ScriptDispatcher{
		serverURL:   config.ServerURL,
		scriptsPath: config.ScriptsPath,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ExecuteScriptRequest is the JSON body sent to instrument-script-server.
type ExecuteScriptRequest struct {
	ScriptName    string                 `json:"scriptName"`              // Name of the Lua script (without .lua extension)
	ScriptPath    string                 `json:"scriptPath"`              // Full path to script file
	Parameters    map[string]interface{} `json:"parameters"`              // Parameters passed to main(ctx, params)
	MeasurementID string                 `json:"measurementId,omitempty"` // Optional tracking ID
}

// ExecuteScriptResponse is the JSON response from instrument-script-server.
type ExecuteScriptResponse struct {
	Success       bool            `json:"success"`
	Result        json.RawMessage `json:"result,omitempty"`    // Script return value as JSON
	Error         string          `json:"error,omitempty"`     // Error message if failed
	ExecutionTime float64         `json:"executionTimeMs"`     // Time taken in milliseconds
	Logs          []string        `json:"logs,omitempty"`      // Log messages from ctx:log()
	BufferIDs     []string        `json:"bufferIds,omitempty"` // IDs of any data buffers created
}

// ExecuteScript implements ScriptExecutor interface.
// It sends a script execution request to the instrument-script-server.
func (d *ScriptDispatcher) ExecuteScript(ctx context.Context, scriptName string, params map[string]interface{}) ([]byte, error) {
	// Build script path
	scriptPath := filepath.Join(d.scriptsPath, scriptName+".lua")

	req := ExecuteScriptRequest{
		ScriptName: scriptName,
		ScriptPath: scriptPath,
		Parameters: params,
	}

	// Serialize request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := d.serverURL + "/api/v1/execute"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var scriptResp ExecuteScriptResponse
	if err := json.Unmarshal(respBody, &scriptResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !scriptResp.Success {
		return nil, fmt.Errorf("script execution failed: %s", scriptResp.Error)
	}

	return scriptResp.Result, nil
}

// =============================================================================
// Script Registry - Tracks available measurement scripts
// =============================================================================

// ScriptInfo describes an available Lua measurement script.
type ScriptInfo struct {
	Name        string   `json:"name"`        // Script name (e.g., "sweep_1d")
	Path        string   `json:"path"`        // Full path to .lua file
	Description string   `json:"description"` // Human-readable description
	Parameters  []string `json:"parameters"`  // Expected parameter names
	Returns     string   `json:"returns"`     // Return type description
}

// ScriptRegistry tracks available Lua scripts on the hub.
type ScriptRegistry struct {
	scripts map[string]ScriptInfo
}

// NewScriptRegistry creates a new registry.
func NewScriptRegistry() *ScriptRegistry {
	return &ScriptRegistry{
		scripts: make(map[string]ScriptInfo),
	}
}

// Register adds a script to the registry.
func (r *ScriptRegistry) Register(info ScriptInfo) {
	r.scripts[info.Name] = info
}

// Get returns script info by name.
func (r *ScriptRegistry) Get(name string) (ScriptInfo, bool) {
	info, ok := r.scripts[name]
	return info, ok
}

// List returns all registered scripts.
func (r *ScriptRegistry) List() []ScriptInfo {
	result := make([]ScriptInfo, 0, len(r.scripts))
	for _, info := range r.scripts {
		result = append(result, info)
	}
	return result
}

// RegisterBuiltinScripts adds the standard measurement scripts to the registry.
func (r *ScriptRegistry) RegisterBuiltinScripts(scriptsPath string) {
	// These are the scripts that experimenters should provide
	builtins := []ScriptInfo{
		{
			Name:        "set_voltage",
			Path:        filepath.Join(scriptsPath, "set_voltage.lua"),
			Description: "Set a single gate voltage",
			Parameters:  []string{"instrument", "channel", "voltage"},
			Returns:     "nil",
		},
		{
			Name:        "get_voltage",
			Path:        filepath.Join(scriptsPath, "get_voltage.lua"),
			Description: "Read voltage from an instrument",
			Parameters:  []string{"instrument", "channel"},
			Returns:     "MeasurementResponse<number>",
		},
		{
			Name:        "sweep_1d",
			Path:        filepath.Join(scriptsPath, "sweep_1d.lua"),
			Description: "Perform 1D voltage sweep with current measurement",
			Parameters:  []string{"sweepInstrument", "sweepChannel", "startVoltage", "stopVoltage", "numPoints", "settlingTimeMs", "currentMeter", "currentChannel"},
			Returns:     "array of {voltage, current}",
		},
		{
			Name:        "ramp_voltage",
			Path:        filepath.Join(scriptsPath, "ramp_voltage.lua"),
			Description: "Ramp a gate voltage to target at specified slope",
			Parameters:  []string{"instrument", "channel", "targetV", "slopeVPerSec"},
			Returns:     "nil",
		},
		{
			Name:        "measure_current",
			Path:        filepath.Join(scriptsPath, "measure_current.lua"),
			Description: "Measure current from specified instrument",
			Parameters:  []string{"instrument", "channel", "sampleRate"},
			Returns:     "MeasurementResponse<number>",
		},
		{
			Name:        "dc_get_set",
			Path:        filepath.Join(scriptsPath, "dc_get_set.lua"),
			Description: "Set voltages and measure currents (DC measurement)",
			Parameters:  []string{"setters", "getters", "setVoltages"},
			Returns:     "array of MeasurementResponse",
		},
	}

	for _, script := range builtins {
		r.Register(script)
	}
}
