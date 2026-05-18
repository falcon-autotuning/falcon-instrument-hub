//go:build !cgo || !falcon_core

package handlers

import (
	"encoding/json"
	"fmt"
)

// gateNameFromConnectionJSON parses a cereal-format connection JSON and returns
// the gate name (e.g. "P1") without requiring the falcon-core C library.
func gateNameFromConnectionJSON(connectionJSON string) (string, error) {
	if connectionJSON == "" {
		return "", fmt.Errorf("gateNameFromConnectionJSON: empty JSON")
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(connectionJSON), &raw); err != nil {
		return "", fmt.Errorf("gateNameFromConnectionJSON: %w", err)
	}

	// Try direct "name" field (simplified format).
	if name, ok := raw["name"].(string); ok {
		return name, nil
	}

	// Try cereal ptr_wrapper.data.name (polymorphic type with named field).
	if pw, ok := raw["ptr_wrapper"].(map[string]interface{}); ok {
		if data, ok := pw["data"].(map[string]interface{}); ok {
			if name, ok := data["name"].(string); ok {
				return name, nil
			}
			// Try cereal simple serialization: ptr_wrapper.data.value0
			if name, ok := data["value0"].(string); ok {
				return name, nil
			}
		}
	}

	// Try root-level value0 (cereal direct serialization).
	if name, ok := raw["value0"].(string); ok {
		return name, nil
	}

	return "", fmt.Errorf("gateNameFromConnectionJSON: cannot extract gate name from: %s", connectionJSON)
}
