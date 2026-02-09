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
	"text/template"
)

// Deprecated: ScriptGenerator auto-generates Lua scripts, which is no longer recommended.
// Use MeasurementOrchestrator with user-provided scripts instead.

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

const sweep1DLuaTemplate = `-- Auto-generated 1D voltage sweep script
-- Generated from MeasurementRequest: {{.MeasurementName}}
-- Sweeps {{.SweepGate}} from {{printf "%.4f" .StartVoltage}}V to {{printf "%.4f" .StopVoltage}}V

function main(ctx)
    ctx:log("Starting 1D sweep: {{.MeasurementName}}")
    ctx:log("Sweep gate: {{.SweepGate}}, Start: {{printf "%.4f" .StartVoltage}}V, Stop: {{printf "%.4f" .StopVoltage}}V, Points: {{.NumPoints}}")
    
    local results = {}
    local start_voltage = {{printf "%.6f" .StartVoltage}}
    local stop_voltage = {{printf "%.6f" .StopVoltage}}
    local num_points = {{.NumPoints}}
    local step = (stop_voltage - start_voltage) / (num_points - 1)
    
    -- Set static gate voltages first
    {{- range .StaticSetters }}
    ctx:call("{{.Setter.Id}}.SET_VOLTAGE", {
        channel = {{.Setter.Channel}},
        voltage = {{printf "%.6f" .SetVoltage}}
    })
    ctx:log("Static: {{.Setter.Id}}:{{.Setter.Channel}} = {{printf "%.4f" .SetVoltage}} V")
    {{- end }}
    
    -- Perform the sweep
    for i = 0, num_points - 1 do
        local voltage = start_voltage + (i * step)
        
        -- Set sweep gate voltage
        ctx:call("{{.SweepSetter.Id}}.SET_VOLTAGE", {
            channel = {{.SweepSetter.Channel}},
            voltage = voltage
        })
        
        -- Brief settling time
        {{- if gt .SettlingTimeMs 0.0 }}
        ctx:sleep({{printf "%.3f" .SettlingTimeMs}})
        {{- end }}
        
        -- Read all measurement channels
        local point_data = {
            sweep_voltage = voltage,
            measurements = {}
        }
        
        {{- range $index, $req := .GetVoltageRequests }}
        local response_{{$index}} = ctx:call("{{$req.Getter.Id}}.GET_VOLTAGE", {
            channel = {{$req.Getter.Channel}}
        })
        table.insert(point_data.measurements, {
            instrument = "{{$req.Getter.Id}}",
            channel = {{$req.Getter.Channel}},
            value = response_{{$index}}:value()
        })
        {{- end }}
        
        table.insert(results, point_data)
        
        -- Log progress every 10%
        if i % math.floor(num_points / 10) == 0 then
            ctx:log(string.format("Sweep progress: %d%% (V=%.4f)", math.floor(i * 100 / num_points), voltage))
        end
    end
    
    ctx:log("1D sweep complete: " .. #results .. " points collected")
    return results
end
`

const dcGetSetLuaTemplate = `-- Auto-generated DC GetSet measurement script
-- Generated from MeasurementRequest: {{.MeasurementName}}
-- Sets voltages on specified gates and measures currents

function main(ctx)
    ctx:log("Starting DC GetSet: {{.MeasurementName}}")
    
    -- Set all voltages in parallel
    ctx:parallel(function()
        {{- range .SetVoltageRequests }}
        ctx:call("{{.Setter.Id}}.SET_VOLTAGE", {
            channel = {{.Setter.Channel}},
            voltage = {{printf "%.6f" .SetVoltage}}
        })
        {{- end }}
    end)
    ctx:log("All voltages set")
    
    -- Brief settling time
    {{- if gt .SettlingTimeMs 0.0 }}
    ctx:sleep({{printf "%.3f" .SettlingTimeMs}})
    {{- end }}
    
    -- Acquire measurements in parallel
    local results = {}
    ctx:parallel(function()
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
    end)
    
    ctx:log("DC GetSet complete: " .. #results .. " measurements")
    return results
end
`

const averagedSweep1DLuaTemplate = `-- Auto-generated N-averaged 1D voltage sweep script
-- Generated from MeasurementRequest: {{.MeasurementName}}
-- Sweeps {{.SweepGate}} from {{printf "%.4f" .StartVoltage}}V to {{printf "%.4f" .StopVoltage}}V
-- Performs {{.NumAverages}} sweeps and returns averaged trace
-- MeasurementID: {{.MeasurementID}}

function main(ctx)
    ctx:log("Starting N-averaged 1D sweep: {{.MeasurementName}}")
    ctx:log("Sweep gate: {{.SweepGate}}, Start: {{printf "%.4f" .StartVoltage}}V, Stop: {{printf "%.4f" .StopVoltage}}V")
    ctx:log("Points: {{.NumPoints}}, Averages: {{.NumAverages}}")
    
    local num_points = {{.NumPoints}}
    local num_averages = {{.NumAverages}}
    local start_voltage = {{printf "%.6f" .StartVoltage}}
    local stop_voltage = {{printf "%.6f" .StopVoltage}}
    local step = (stop_voltage - start_voltage) / (num_points - 1)
    local measurement_id = "{{.MeasurementID}}"
    
    -- Set static gate voltages first
    {{- range .StaticSetters }}
    ctx:call("{{.Setter.Id}}.SET_VOLTAGE", {
        channel = {{.Setter.Channel}},
        voltage = {{printf "%.6f" .SetVoltage}}
    })
    ctx:log("Static: {{.Setter.Id}}:{{.Setter.Channel}} = {{printf "%.4f" .SetVoltage}} V")
    {{- end }}
    
    -- Storage for all traces (each sweep produces one trace)
    local all_traces = {}
    
    -- Perform N sweeps
    for sweep_idx = 1, num_averages do
        ctx:log(string.format("Starting sweep %d of %d", sweep_idx, num_averages))
        
        local trace = {}
        
        for i = 0, num_points - 1 do
            local voltage = start_voltage + (i * step)
            
            -- Set sweep gate voltage
            ctx:call("{{.SweepSetter.Id}}.SET_VOLTAGE", {
                channel = {{.SweepSetter.Channel}},
                voltage = voltage
            })
            
            -- Settling time
            {{- if gt .SettlingTimeMs 0.0 }}
            ctx:sleep({{printf "%.3f" .SettlingTimeMs}})
            {{- end }}
            
            -- Read measurement channel(s)
            local point_measurements = {}
            {{- range $index, $req := .GetVoltageRequests }}
            local response_{{$index}} = ctx:call("{{$req.Getter.Id}}.GET_VOLTAGE", {
                channel = {{$req.Getter.Channel}}
            })
            point_measurements["{{$req.Getter.Id}}_{{$req.Getter.Channel}}"] = response_{{$index}}:value()
            {{- end }}
            
            table.insert(trace, {
                voltage = voltage,
                measurements = point_measurements
            })
        end
        
        table.insert(all_traces, trace)
        
        -- Report trace to hub for buffering (before averaging)
        ctx:report_trace({
            measurement_id = measurement_id,
            sweep_index = sweep_idx,
            total_sweeps = num_averages,
            trace = trace
        })
        
        ctx:log(string.format("Sweep %d complete, %d points collected", sweep_idx, #trace))
    end
    
    ctx:log("All sweeps complete. Computing averages...")
    
    -- Compute averaged trace
    local averaged_trace = {}
    for i = 1, num_points do
        local voltage = start_voltage + ((i - 1) * step)
        local avg_measurements = {}
        
        -- For each measurement channel
        {{- range $index, $req := .GetVoltageRequests }}
        local sum_{{$index}} = 0
        for _, trace in ipairs(all_traces) do
            sum_{{$index}} = sum_{{$index}} + trace[i].measurements["{{$req.Getter.Id}}_{{$req.Getter.Channel}}"]
        end
        avg_measurements["{{$req.Getter.Id}}_{{$req.Getter.Channel}}"] = sum_{{$index}} / num_averages
        {{- end }}
        
        table.insert(averaged_trace, {
            voltage = voltage,
            measurements = avg_measurements
        })
    end
    
    ctx:log("N-averaged sweep complete: " .. #averaged_trace .. " points")
    
    return {
        measurement_id = measurement_id,
        sweep_gate = "{{.SweepGate}}",
        start_voltage = start_voltage,
        stop_voltage = stop_voltage,
        num_points = num_points,
        num_averages = num_averages,
        all_traces = all_traces,
        averaged_trace = averaged_trace
    }
end
`

// ScriptGenerator generates Lua scripts from measurement requests.
type ScriptGenerator struct {
	outputDir            string
	setVoltageTempl      *template.Template
	getVoltageTempl      *template.Template
	measureTempl         *template.Template
	sweep1DTempl         *template.Template
	dcGetSetTempl        *template.Template
	averagedSweep1DTempl *template.Template
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

	sweep1DTempl, err := template.New("sweep_1d").Parse(sweep1DLuaTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sweep_1d template: %w", err)
	}

	dcGetSetTempl, err := template.New("dc_get_set").Parse(dcGetSetLuaTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dc_get_set template: %w", err)
	}

	averagedSweep1DTempl, err := template.New("averaged_sweep_1d").Parse(averagedSweep1DLuaTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse averaged_sweep_1d template: %w", err)
	}

	return &ScriptGenerator{
		outputDir:            outputDir,
		setVoltageTempl:      setVoltageTempl,
		getVoltageTempl:      getVoltageTempl,
		measureTempl:         measureTempl,
		sweep1DTempl:         sweep1DTempl,
		dcGetSetTempl:        dcGetSetTempl,
		averagedSweep1DTempl: averagedSweep1DTempl,
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

// Sweep1DScriptData is the data for generating 1D voltage sweep scripts.
type Sweep1DScriptData struct {
	MeasurementName    string
	SweepGate          string              // Name of the gate being swept (e.g., "P1")
	SweepSetter        InstrumentTarget    // The DAC/channel for the sweep gate
	StartVoltage       float64             // Starting voltage
	StopVoltage        float64             // Ending voltage
	NumPoints          int                 // Number of points in sweep
	SettlingTimeMs     float64             // Time to wait after setting voltage (ms)
	StaticSetters      []SetVoltageRequest // Static gate voltages during sweep
	GetVoltageRequests []GetVoltageRequest // Measurement channels (e.g., DMM for current)
}

// DCGetSetScriptData is the data for generating DC get/set measurement scripts.
type DCGetSetScriptData struct {
	MeasurementName    string
	SetVoltageRequests []SetVoltageRequest
	GetVoltageRequests []GetVoltageRequest
	SettlingTimeMs     float64
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

// GenerateSweep1DScript generates a Lua script for 1D voltage sweeps.
func (g *ScriptGenerator) GenerateSweep1DScript(data Sweep1DScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_sweep.lua"
	filepath := filepath.Join(g.outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	if err := g.sweep1DTempl.Execute(file, data); err != nil {
		return "", fmt.Errorf("failed to execute sweep_1d template: %w", err)
	}

	return filepath, nil
}

// GenerateDCGetSetScript generates a Lua script for DC get/set with parallel execution.
func (g *ScriptGenerator) GenerateDCGetSetScript(data DCGetSetScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_dc_getset.lua"
	filepath := filepath.Join(g.outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	if err := g.dcGetSetTempl.Execute(file, data); err != nil {
		return "", fmt.Errorf("failed to execute dc_get_set template: %w", err)
	}

	return filepath, nil
}

// GenerateAveragedSweep1DScript generates a Lua script for N-averaged 1D sweeps.
func (g *ScriptGenerator) GenerateAveragedSweep1DScript(data AveragedSweep1DScriptData) (string, error) {
	filename := sanitizeFilename(data.MeasurementName) + "_avg_sweep.lua"
	filepath := filepath.Join(g.outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create script file: %w", err)
	}
	defer file.Close()

	if err := g.averagedSweep1DTempl.Execute(file, data); err != nil {
		return "", fmt.Errorf("failed to execute averaged_sweep_1d template: %w", err)
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
