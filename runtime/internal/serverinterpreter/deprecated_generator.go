// Package serverinterpreter provides Lua script generation for measurement requests.
//
// DEPRECATED: This file contains auto-generation of Lua scripts which is no longer
// the recommended approach. Experimenters should create their own Lua measurement
// scripts and the hub should orchestrate calls to those scripts.
//
// See measurement_orchestrator.go and script_dispatcher.go for the new architecture.
// See docs/LUA_SCRIPT_AUTHORING.md for how to create measurement scripts.
//
// This file is kept for backwards compatibility with existing tests but should not
// be used for new development.
package serverinterpreter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Deprecated: ScriptGenerator auto-generates Lua scripts, which is no longer recommended.
// Use MeasurementOrchestrator with user-provided scripts instead.
type ScriptGenerator struct {
	outputDir string
}

// NewScriptGenerator creates a new script generator with the specified output directory.
func NewScriptGenerator(outputDir string) (*ScriptGenerator, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	return &ScriptGenerator{
		outputDir: outputDir,
	}, nil
}

// Sweep1DScriptData is the data for generating 1D voltage sweep scripts.
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

// AveragedSweep1DScriptData is the data for generating N-averaged 1D sweep scripts.
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

// SetVoltageScriptData is the data passed to the set_voltage template.
type SetVoltageScriptData struct {
	MeasurementName    string
	SetVoltageRequests []SetVoltageRequest
}

// GetVoltageScriptData is the data passed to the get_voltage template.
type GetVoltageScriptData struct {
	MeasurementName    string
	GetVoltageRequests []GetVoltageRequest
}

// MeasureGetSetScriptData is the data passed to the measure_get_set template.
type MeasureGetSetScriptData struct {
	MeasurementName    string
	SetVoltageRequests []SetVoltageRequest
	GetVoltageRequests []GetVoltageRequest
}

// DCGetSetScriptData is the data for generating DC get/set measurement scripts.
type DCGetSetScriptData struct {
	MeasurementName    string
	SetVoltageRequests []SetVoltageRequest
	GetVoltageRequests []GetVoltageRequest
	SettlingTimeMs     float64
}

// GenerateAveragedSweep1DScript generates a Lua script for N-averaged 1D sweeps.
func (g *ScriptGenerator) GenerateAveragedSweep1DScript(data AveragedSweep1DScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_avg_sweep.lua"
	filepath := filepath.Join(g.outputDir, filename)

	// Create a minimal script file for backwards compatibility
	content := fmt.Sprintf(`-- DEPRECATED: Auto-generated script for %s
-- This is a stub. Use predefined scripts in runtime/scripts/ instead.
function main(ctx)
    ctx:log("DEPRECATED: Use MeasurementOrchestrator with predefined scripts")
    return {measurement_id = "%s", error = "deprecated"}
end
`, data.MeasurementName, data.MeasurementID)

	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("failed to write script content: %w", err)
	}

	return filepath, nil
}

// GenerateFromParsedRequest generates the appropriate script(s) from a parsed measurement request.
func (g *ScriptGenerator) GenerateFromParsedRequest(req *ParsedMeasurementRequest) ([]string, error) {
	// Deprecated stub - return empty script list
	return []string{}, nil
}

// GenerateSetVoltageScript generates a Lua script for set_voltage operations.
func (g *ScriptGenerator) GenerateSetVoltageScript(data SetVoltageScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_set_voltage.lua"
	filepath := filepath.Join(g.outputDir, filename)
	return g.createStubScript(filepath, "set_voltage", data.MeasurementName)
}

// GenerateGetVoltageScript generates a Lua script for get_voltage operations.
func (g *ScriptGenerator) GenerateGetVoltageScript(data GetVoltageScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_get_voltage.lua"
	filepath := filepath.Join(g.outputDir, filename)
	return g.createStubScript(filepath, "get_voltage", data.MeasurementName)
}

// GenerateMeasureGetSetScript generates a Lua script for combined get/set operations.
func (g *ScriptGenerator) GenerateMeasureGetSetScript(data MeasureGetSetScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_measure.lua"
	filepath := filepath.Join(g.outputDir, filename)
	return g.createStubScript(filepath, "measure_get_set", data.MeasurementName)
}

// GenerateDCGetSetScript generates a Lua script for DC get/set with parallel execution.
func (g *ScriptGenerator) GenerateDCGetSetScript(data DCGetSetScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_dc_getset.lua"
	filepath := filepath.Join(g.outputDir, filename)
	return g.createStubScript(filepath, "dc_get_set", data.MeasurementName)
}

// GenerateSweep1DScript generates a Lua script for 1D voltage sweeps.
func (g *ScriptGenerator) GenerateSweep1DScript(data Sweep1DScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_sweep.lua"
	filepath := filepath.Join(g.outputDir, filename)
	return g.createStubScript(filepath, "sweep_1d", data.MeasurementName)
}

// createStubScript creates a minimal stub script
func (g *ScriptGenerator) createStubScript(filepath, scriptType, measurementName string) (string, error) {
	content := fmt.Sprintf(`-- DEPRECATED: Auto-generated %s script for %s
-- This is a stub. Use predefined scripts in runtime/scripts/ instead.
function main(ctx)
    ctx:log("DEPRECATED: Use MeasurementOrchestrator with predefined scripts")
    return {error = "deprecated"}
end
`, scriptType, measurementName)

	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("failed to write script content: %w", err)
	}

	return filepath, nil
}

// sanitizeFilename removes or replaces characters that are not safe for filenames.
func sanitizeFilename(name string) string {
	// Replace spaces and special characters
	replacer := strings.NewReplacer(
		" ", "_",
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	sanitized := replacer.Replace(name)

	// Ensure the filename is not empty
	if sanitized == "" {
		sanitized = "measurement"
	}

	return sanitized
}
