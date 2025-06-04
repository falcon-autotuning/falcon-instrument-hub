package instrument

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// ensureScriptExists extracts the embedded script if needed
func (h *Handler) ensureScriptExists() error {
	// Create scripts directory if it doesn't exist
	if err := os.MkdirAll(ScriptsDir, 0755); err != nil {
		return fmt.Errorf("failed to create scripts directory: %w", err)
	}

	// Build the full script path
	scriptPath := filepath.Join(ScriptsDir, LaunchInstrumentScriptName)

	// Extract embedded script - read the file by its embedded name
	scriptContent, err := embeddedScript.ReadFile("launch_instrument_daemon.py")
	if err != nil {
		return fmt.Errorf("failed to read embedded script: %w", err)
	}

	// Write script to filesystem
	if err := os.WriteFile(scriptPath, scriptContent, 0755); err != nil {
		return fmt.Errorf("failed to write script file: %w", err)
	}

	h.logger.Info(
		HandlerName,
		fmt.Sprintf("Script updated at %s", scriptPath),
	)
	return nil
}

// unmarshalAndValidate handles the common unmarshaling and validation logic
func (h *Handler) unmarshalAndValidate(
	data []byte,
	req any,
	commandName string,
) error {
	if err := json.Unmarshal(data, req); err != nil {
		h.logger.Error(
			HandlerName,
			fmt.Sprintf("Failed to unmarshal %s: %v", commandName, err),
		)
		return err
	}

	// Use reflection to get the Name field
	v := reflect.ValueOf(req).Elem()
	nameField := v.FieldByName("Name")
	if !nameField.IsValid() || nameField.String() == "" {
		h.logger.Error(
			HandlerName,
			fmt.Sprintf("%s missing instrument name", commandName),
		)
		return fmt.Errorf("missing instrument name")
	}

	return nil
}
