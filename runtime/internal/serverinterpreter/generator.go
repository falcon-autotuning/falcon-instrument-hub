// Package serverinterpreter provides Lua script generation for measurement requests.
package serverinterpreter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const setVoltageLuaTemplate = `-- Auto-generated set_voltage script
-- Generated from MeasurementRequest: {{.MeasurementName}}

function main(ctx)
    ctx:log("Executing set_voltage: {{.MeasurementName}}")
    
    {{- range .SetVoltageRequests }}
    -- Set voltage on {{.Setter.Id}}{{if ne .Setter.Channel 0}}:{{.Setter.Channel}}{{end}}
    ctx:call("{{.Setter.Id}}.SET_VOLTAGE", {
        channel = {{.Setter.Channel}},
        voltage = {{printf "%.6f" .SetVoltage}}
    })
    ctx:log("Set {{.Setter.Id}}{{if ne .Setter.Channel 0}}:{{.Setter.Channel}}{{end}} to {{printf "%.4f" .SetVoltage}} V")
    {{- end }}
    
    return nil
end
`

const getVoltageLuaTemplate = `-- Auto-generated get_voltage script
-- Generated from MeasurementRequest: {{.MeasurementName}}

function main(ctx)
    ctx:log("Executing get_voltage: {{.MeasurementName}}")
    
    local results = {}
    
    {{- range $index, $req := .GetVoltageRequests }}
    -- Get voltage from {{$req.Getter.Id}}{{if ne $req.Getter.Channel 0}}:{{$req.Getter.Channel}}{{end}}
    local response_{{$index}} = ctx:call("{{$req.Getter.Id}}.GET_VOLTAGE", {
        channel = {{$req.Getter.Channel}}
    })
    table.insert(results, response_{{$index}}:value())
    ctx:log("Read {{$req.Getter.Id}}{{if ne $req.Getter.Channel 0}}:{{$req.Getter.Channel}}{{end}}: " .. tostring(response_{{$index}}:value()) .. " V")
    {{- end }}
    
    return results
end
`

const measureGetSetLuaTemplate = `-- Auto-generated measure_get_set script
-- Generated from MeasurementRequest: {{.MeasurementName}}

function main(ctx)
    ctx:log("Executing measure_get_set: {{.MeasurementName}}")
    
    -- First, set all voltages
    {{- range .SetVoltageRequests }}
    ctx:call("{{.Setter.Id}}.SET_VOLTAGE", {
        channel = {{.Setter.Channel}},
        voltage = {{printf "%.6f" .SetVoltage}}
    })
    {{- end }}
    
    -- Then, read all getters
    local results = {}
    
    {{- range $index, $req := .GetVoltageRequests }}
    local response_{{$index}} = ctx:call("{{$req.Getter.Id}}.GET_VOLTAGE", {
        channel = {{$req.Getter.Channel}}
    })
    table.insert(results, {
        instrument = "{{$req.Getter.Id}}",
        channel = {{$req.Getter.Channel}},
        value = response_{{$index}}:value()
    })
    {{- end }}
    
    return results
end
`

// ScriptGenerator generates Lua scripts from measurement requests.
type ScriptGenerator struct {
	outputDir       string
	setVoltageTempl *template.Template
	getVoltageTempl *template.Template
	measureTempl    *template.Template
}

// NewScriptGenerator creates a new script generator with the specified output directory.
func NewScriptGenerator(outputDir string) (*ScriptGenerator, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	setVoltageTempl, err := template.New("set_voltage").Parse(setVoltageLuaTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse set_voltage template: %w", err)
	}

	getVoltageTempl, err := template.New("get_voltage").Parse(getVoltageLuaTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse get_voltage template: %w", err)
	}

	measureTempl, err := template.New("measure_get_set").Parse(measureGetSetLuaTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse measure_get_set template: %w", err)
	}

	return &ScriptGenerator{
		outputDir:       outputDir,
		setVoltageTempl: setVoltageTempl,
		getVoltageTempl: getVoltageTempl,
		measureTempl:    measureTempl,
	}, nil
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

// GenerateSetVoltageScript generates a Lua script for set_voltage operations.
func (g *ScriptGenerator) GenerateSetVoltageScript(data SetVoltageScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_set_voltage.lua"
	filepath := filepath.Join(g.outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	if err := g.setVoltageTempl.Execute(file, data); err != nil {
		return "", fmt.Errorf("failed to execute set_voltage template: %w", err)
	}

	return filepath, nil
}

// GenerateGetVoltageScript generates a Lua script for get_voltage operations.
func (g *ScriptGenerator) GenerateGetVoltageScript(data GetVoltageScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_get_voltage.lua"
	filepath := filepath.Join(g.outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	if err := g.getVoltageTempl.Execute(file, data); err != nil {
		return "", fmt.Errorf("failed to execute get_voltage template: %w", err)
	}

	return filepath, nil
}

// GenerateMeasureGetSetScript generates a Lua script for combined get/set operations.
func (g *ScriptGenerator) GenerateMeasureGetSetScript(data MeasureGetSetScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_measure.lua"
	filepath := filepath.Join(g.outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	if err := g.measureTempl.Execute(file, data); err != nil {
		return "", fmt.Errorf("failed to execute measure_get_set template: %w", err)
	}

	return filepath, nil
}

// GenerateFromParsedRequest generates the appropriate script(s) from a parsed measurement request.
func (g *ScriptGenerator) GenerateFromParsedRequest(req *ParsedMeasurementRequest) ([]string, error) {
	var scripts []string

	setVoltageReqs := req.ToSetVoltageRequests()
	getVoltageReqs := req.ToGetVoltageRequests()

	// If we have both setters and getters, generate a combined script
	if len(setVoltageReqs) > 0 && len(getVoltageReqs) > 0 {
		path, err := g.GenerateMeasureGetSetScript(MeasureGetSetScriptData{
			MeasurementName:    req.MeasurementName,
			SetVoltageRequests: setVoltageReqs,
			GetVoltageRequests: getVoltageReqs,
		})
		if err != nil {
			return nil, err
		}
		scripts = append(scripts, path)
	} else {
		// Generate separate scripts
		if len(setVoltageReqs) > 0 {
			path, err := g.GenerateSetVoltageScript(SetVoltageScriptData{
				MeasurementName:    req.MeasurementName,
				SetVoltageRequests: setVoltageReqs,
			})
			if err != nil {
				return nil, err
			}
			scripts = append(scripts, path)
		}

		if len(getVoltageReqs) > 0 {
			path, err := g.GenerateGetVoltageScript(GetVoltageScriptData{
				MeasurementName:    req.MeasurementName,
				GetVoltageRequests: getVoltageReqs,
			})
			if err != nil {
				return nil, err
			}
			scripts = append(scripts, path)
		}
	}

	return scripts, nil
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
