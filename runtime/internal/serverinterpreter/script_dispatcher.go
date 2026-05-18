// Package serverinterpreter provides script dispatch to instrument-script-server.
package serverinterpreter

import (
	"fmt"
	"path/filepath"
	"time"
)

// ScriptDispatcher communicates with the instrument-script-server to execute
// user-provided Lua measurement scripts.
type ScriptDispatcher struct {
	client      *ScriptServerClient
	scriptsPath string // Base path where Lua scripts are stored
	pollTimeout time.Duration
}

// ScriptDispatcherConfig configures the script dispatcher.
type ScriptDispatcherConfig struct {
	ServerURL      string        // HTTP URL of instrument-script-server (e.g., "http://localhost:8080")
	ServerHost     string        // Host of ISS (used if ServerURL is empty)
	ServerPort     int           // Port of ISS (used if ServerURL is empty)
	ScriptsPath    string        // Local directory containing Lua scripts
	RequestTimeout time.Duration // Timeout for script execution
}

// NewScriptDispatcher creates a new dispatcher.
func NewScriptDispatcher(config ScriptDispatcherConfig) *ScriptDispatcher {
	timeout := config.RequestTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	host := config.ServerHost
	port := config.ServerPort
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 8555
	}

	client := NewScriptServerClient(host, port)

	return &ScriptDispatcher{
		client:      client,
		scriptsPath: config.ScriptsPath,
		pollTimeout: timeout,
	}
}

// ResolvedCallResult extends ISSCallResult with buffer data resolved inline.
type ResolvedCallResult struct {
	ISSCallResult
	BufferData []float64 // populated when Return.Type == "buffer"
}

// RunMeasurement calls ISS measure (sync), resolves all buffer results, returns
// the full call list with buffer data populated.
// typeManifest, if non-nil, tells ISS to call main with positional arguments
// (required for Teal-compiled scripts with named parameters).
func (d *ScriptDispatcher) RunMeasurement(scriptName string, globals map[string]interface{}, typeManifest map[string]interface{}) ([]ResolvedCallResult, error) {
	scriptPath := filepath.Join(d.scriptsPath, scriptName+".lua")
	results, err := d.client.Measure(scriptPath, globals, typeManifest)
	if err != nil {
		return nil, fmt.Errorf("measure script %s: %w", scriptName, err)
	}

	resolved := make([]ResolvedCallResult, len(results))
	for i, r := range results {
		resolved[i] = ResolvedCallResult{ISSCallResult: r}
		if r.Return.Type == "buffer" && r.Return.BufferID != "" {
			data, err := d.client.ReadBuffer(r.Return.BufferID)
			if err != nil {
				return nil, fmt.Errorf("read_buffer %s: %w", r.Return.BufferID, err)
			}
			resolved[i].BufferData = data
		}
	}
	return resolved, nil
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
