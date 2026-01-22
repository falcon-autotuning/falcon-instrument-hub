package compiler

import (
	"strings"
	"testing"
)

func TestCompileMeasurement(t *testing.T) {
	compiler := NewCompiler()

	tests := []struct {
		name      string
		req       *MeasurementRequest
		wantError bool
	}{
		{
			name: "basic measurement",
			req: &MeasurementRequest{
				InstrumentName: "DMM",
				Command:        "MEASURE",
			},
			wantError: false,
		},
		{
			name: "measurement with parameters",
			req: &MeasurementRequest{
				InstrumentName: "QDAC",
				Command:        "SET_VOLTAGE",
				Parameters: map[string]interface{}{
					"channel": 1,
					"voltage": 2.5,
				},
			},
			wantError: false,
		},
		{
			name: "missing instrument name",
			req: &MeasurementRequest{
				Command: "MEASURE",
			},
			wantError: true,
		},
		{
			name: "missing command",
			req: &MeasurementRequest{
				InstrumentName: "DMM",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, err := compiler.CompileMeasurement(tt.req)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if script.ScriptContent == "" {
				t.Error("Expected non-empty script content")
			}

			// Verify script contains required Lua function
			if !strings.Contains(script.ScriptContent, "function main(ctx)") {
				t.Error("Script should contain main function")
			}

			// Verify script contains instrument command
			expectedCommand := tt.req.InstrumentName + "." + tt.req.Command
			if !strings.Contains(script.ScriptContent, expectedCommand) {
				t.Errorf("Script should contain command %s", expectedCommand)
			}
		})
	}
}

func TestFormatLuaValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"string", "hello", "\"hello\""},
		{"int", 42, "42"},
		{"float", 3.14, "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, "nil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLuaValue(tt.value)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGenerateLuaScript(t *testing.T) {
	req := &MeasurementRequest{
		InstrumentName: "DMM",
		Command:        "MEASURE",
		Parameters: map[string]interface{}{
			"range":  "10V",
			"rate":   1000,
			"enable": true,
		},
	}

	script := generateLuaScript(req)

	// Check for required elements
	required := []string{
		"function main(ctx)",
		"ctx:log(",
		"ctx:call(",
		"DMM.MEASURE",
		"return result",
	}

	for _, req := range required {
		if !strings.Contains(script, req) {
			t.Errorf("Script should contain: %s", req)
		}
	}

	// Check for parameters
	if !strings.Contains(script, "local params = {") {
		t.Error("Script should contain parameters table")
	}
}

func TestSelectMeasurementScript(t *testing.T) {
	compiler := NewCompiler()

	req := &MeasurementRequest{
		InstrumentName: "DMM",
		Command:        "MEASURE",
	}

	// This should return an error as it's not implemented yet
	_, err := compiler.SelectMeasurementScript(req)
	if err == nil {
		t.Error("Expected error for unimplemented function")
	}
}
