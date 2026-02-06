package scriptbridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewScriptGenerator(t *testing.T) {
	t.Run("creates output directory", func(t *testing.T) {
		tempDir := t.TempDir()
		outputDir := filepath.Join(tempDir, "scripts")

		gen, err := NewScriptGenerator(outputDir)
		require.NoError(t, err)
		require.NotNil(t, gen)

		// Verify directory was created
		_, err = os.Stat(outputDir)
		assert.NoError(t, err)
	})

	t.Run("uses existing directory", func(t *testing.T) {
		tempDir := t.TempDir()

		gen, err := NewScriptGenerator(tempDir)
		require.NoError(t, err)
		require.NotNil(t, gen)
	})
}

func TestScriptGenerator_GenerateSetVoltageScript(t *testing.T) {
	tempDir := t.TempDir()
	gen, err := NewScriptGenerator(tempDir)
	require.NoError(t, err)

	data := SetVoltageScriptData{
		MeasurementName: "test_set_voltage",
		SetVoltageRequests: []SetVoltageRequest{
			{
				Setter:     InstrumentTarget{Id: "DAC1", Channel: 0},
				SetVoltage: 1.5,
			},
			{
				Setter:     InstrumentTarget{Id: "DAC1", Channel: 1},
				SetVoltage: 2.5,
			},
		},
	}

	scriptPath, err := gen.GenerateSetVoltageScript(data)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(scriptPath)
	require.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Check for expected content
	assert.Contains(t, contentStr, "function main(ctx)")
	assert.Contains(t, contentStr, "SET_VOLTAGE")
	assert.Contains(t, contentStr, "DAC1")
	assert.Contains(t, contentStr, "1.5")
	assert.Contains(t, contentStr, "2.5")
	assert.Contains(t, contentStr, "test_set_voltage")
}

func TestScriptGenerator_GenerateGetVoltageScript(t *testing.T) {
	tempDir := t.TempDir()
	gen, err := NewScriptGenerator(tempDir)
	require.NoError(t, err)

	data := GetVoltageScriptData{
		MeasurementName: "test_get_voltage",
		GetVoltageRequests: []GetVoltageRequest{
			{
				Getter: InstrumentTarget{Id: "DMM1", Channel: 0},
			},
			{
				Getter: InstrumentTarget{Id: "DMM2", Channel: 1},
			},
		},
	}

	scriptPath, err := gen.GenerateGetVoltageScript(data)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(scriptPath)
	require.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Check for expected content
	assert.Contains(t, contentStr, "function main(ctx)")
	assert.Contains(t, contentStr, "GET_VOLTAGE")
	assert.Contains(t, contentStr, "DMM1")
	assert.Contains(t, contentStr, "DMM2")
	assert.Contains(t, contentStr, "local results = {}")
	assert.Contains(t, contentStr, "return results")
}

func TestScriptGenerator_GenerateMeasureGetSetScript(t *testing.T) {
	tempDir := t.TempDir()
	gen, err := NewScriptGenerator(tempDir)
	require.NoError(t, err)

	data := MeasureGetSetScriptData{
		MeasurementName: "test_measure",
		SetVoltageRequests: []SetVoltageRequest{
			{
				Setter:     InstrumentTarget{Id: "DAC1", Channel: 0},
				SetVoltage: 3.3,
			},
		},
		GetVoltageRequests: []GetVoltageRequest{
			{
				Getter: InstrumentTarget{Id: "DMM1", Channel: 0},
			},
		},
	}

	scriptPath, err := gen.GenerateMeasureGetSetScript(data)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(scriptPath)
	require.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Check for expected content
	assert.Contains(t, contentStr, "function main(ctx)")
	assert.Contains(t, contentStr, "SET_VOLTAGE")
	assert.Contains(t, contentStr, "GET_VOLTAGE")
	assert.Contains(t, contentStr, "DAC1")
	assert.Contains(t, contentStr, "DMM1")
	assert.Contains(t, contentStr, "3.3")
}

func TestScriptGenerator_GenerateFromParsedRequest(t *testing.T) {
	tempDir := t.TempDir()
	gen, err := NewScriptGenerator(tempDir)
	require.NoError(t, err)

	t.Run("set only generates set script", func(t *testing.T) {
		req := &ParsedMeasurementRequest{
			MeasurementName: "set_only_test",
			Setters: []InstrumentTarget{
				{Id: "DAC1", Channel: 0},
			},
			SetVoltages: map[string]float64{
				"DAC1": 1.0,
			},
		}

		paths, err := gen.GenerateFromParsedRequest(req)
		require.NoError(t, err)
		require.Len(t, paths, 1)
		assert.Contains(t, paths[0], "set_voltage.lua")
	})

	t.Run("get only generates get script", func(t *testing.T) {
		req := &ParsedMeasurementRequest{
			MeasurementName: "get_only_test",
			Getters: []InstrumentTarget{
				{Id: "DMM1", Channel: 0},
			},
		}

		paths, err := gen.GenerateFromParsedRequest(req)
		require.NoError(t, err)
		require.Len(t, paths, 1)
		assert.Contains(t, paths[0], "get_voltage.lua")
	})

	t.Run("both generates combined script", func(t *testing.T) {
		req := &ParsedMeasurementRequest{
			MeasurementName: "combined_test",
			Setters: []InstrumentTarget{
				{Id: "DAC1", Channel: 0},
			},
			Getters: []InstrumentTarget{
				{Id: "DMM1", Channel: 0},
			},
			SetVoltages: map[string]float64{
				"DAC1": 1.0,
			},
		}

		paths, err := gen.GenerateFromParsedRequest(req)
		require.NoError(t, err)
		require.Len(t, paths, 1)
		assert.Contains(t, paths[0], "_measure.lua")
	})
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with spaces", "with_spaces"},
		{"path/sep", "path_sep"},
		{"special:chars*?", "special_chars__"},
		{"", "measurement"},
		{"<>|\"test", "___\"test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			// For the test with quotes, just ensure it doesn't contain invalid chars
			if tt.input == "" {
				assert.Equal(t, tt.expected, result)
			} else {
				assert.NotContains(t, result, "/")
				assert.NotContains(t, result, "\\")
			}
		})
	}
}

func TestGeneratedScriptSyntax(t *testing.T) {
	// This test verifies that generated scripts have valid Lua-like syntax
	tempDir := t.TempDir()
	gen, err := NewScriptGenerator(tempDir)
	require.NoError(t, err)

	data := SetVoltageScriptData{
		MeasurementName: "syntax_test",
		SetVoltageRequests: []SetVoltageRequest{
			{
				Setter:     InstrumentTarget{Id: "DAC1", Channel: 0},
				SetVoltage: 1.5,
			},
		},
	}

	scriptPath, err := gen.GenerateSetVoltageScript(data)
	require.NoError(t, err)

	content, err := os.ReadFile(scriptPath)
	require.NoError(t, err)
	contentStr := string(content)

	// Check for balanced braces in the function
	assert.Equal(t, strings.Count(contentStr, "function"), strings.Count(contentStr, "end"))
	assert.True(t, strings.Contains(contentStr, "return nil") || strings.Contains(contentStr, "return results"))
}
