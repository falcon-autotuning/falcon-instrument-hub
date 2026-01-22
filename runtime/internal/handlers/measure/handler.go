package measure

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/client"
	"github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/compiler"
)

// Handler manages measurement execution through the instrument-script-server
type Handler struct {
	client   *client.InstrumentServerClient
	compiler *compiler.Compiler
	scriptDir string
}

// NewHandler creates a new measurement handler
func NewHandler(client *client.InstrumentServerClient) *Handler {
	// Create a temporary directory for generated scripts
	scriptDir := filepath.Join(os.TempDir(), "falcon-instrument-hub-scripts")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		log.Printf("Warning: failed to create script directory: %v", err)
	}

	return &Handler{
		client:    client,
		compiler:  compiler.NewCompiler(),
		scriptDir: scriptDir,
	}
}

// ExecuteMeasurement executes a measurement based on the provided request
//
// This method:
// 1. Compiles the measurement request into a Lua script (using falcon-core bindings)
// 2. Writes the script to a temporary file
// 3. Sends the measurement request to instrument-script-server
// 4. Returns the measurement results
//
// NOTE: This API endpoint will be refactored in future versions to support
// more advanced measurement patterns and direct script execution
func (h *Handler) ExecuteMeasurement(ctx context.Context, req *compiler.MeasurementRequest) (*client.MeasurementResult, error) {
	// Compile the measurement request into a Lua script
	script, err := h.compiler.CompileMeasurement(req)
	if err != nil {
		return nil, fmt.Errorf("failed to compile measurement: %w", err)
	}

	// Write script to temporary file
	scriptPath, err := h.writeScriptFile(script.ScriptContent)
	if err != nil {
		return nil, fmt.Errorf("failed to write script file: %w", err)
	}
	defer os.Remove(scriptPath) // Clean up after execution

	// Execute the measurement via instrument-script-server
	measureReq := client.MeasureRequest{
		ScriptPath: scriptPath,
		Globals:    script.Globals,
		OutputJSON: true,
	}

	result, err := h.client.Measure(ctx, measureReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute measurement: %w", err)
	}

	log.Printf("Measurement executed successfully on instrument %s", req.InstrumentName)
	return result, nil
}

// ExecuteMeasurementFromScript executes a measurement from a pre-existing Lua script
//
// This method allows direct execution of Lua measurement scripts without compilation.
// Useful for custom or complex measurements that are already in Lua format.
//
// NOTE: This API endpoint will be refactored in future versions
func (h *Handler) ExecuteMeasurementFromScript(ctx context.Context, scriptPath string, globals map[string]interface{}) (*client.MeasurementResult, error) {
	measureReq := client.MeasureRequest{
		ScriptPath: scriptPath,
		Globals:    globals,
		OutputJSON: true,
	}

	result, err := h.client.Measure(ctx, measureReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute measurement from script: %w", err)
	}

	log.Printf("Measurement executed from script: %s", scriptPath)
	return result, nil
}

// writeScriptFile writes a Lua script to a temporary file and returns the path
func (h *Handler) writeScriptFile(content string) (string, error) {
	// Create a temporary file for the script
	tmpFile, err := os.CreateTemp(h.scriptDir, "measurement-*.lua")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	// Write the script content
	if _, err := tmpFile.WriteString(content); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write script content: %w", err)
	}

	return tmpFile.Name(), nil
}

// TODO: Future enhancements for measurement handling:
//
// 1. Script Template Management
//    - Load measurement script templates from falcon-core
//    - Cache frequently used scripts
//    - Support script versioning and selection
//
// 2. Measurement Script Selection (CURRENTLY MISSING - per problem statement)
//    - Implement SelectMeasurementScript to choose the right script
//    - Use falcon-core Go bindings to match requests to scripts
//    - Support different measurement types (sweeps, single-shot, etc.)
//
// 3. Advanced Features
//    - Batch measurement execution
//    - Measurement result caching
//    - Real-time measurement streaming
//    - Measurement scheduling and queuing
//
// 4. Integration with falcon-core
//    - Replace compiler placeholder with real falcon-core bindings
//    - Support all measurement patterns from falcon-core
//    - Validate measurement parameters using falcon-core
