# Lua Measurement Script Authoring Guide

This guide explains how experimenters create Lua measurement scripts for the FALCon instrument system.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              FALCON (Autotuner)                              │
│                                                                              │
│  Sends measurement requests (JSON) based on falcon-measurement-lib schemas  │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          falcon-instrument-hub                               │
│                                                                              │
│  • Parses incoming measurement requests                                      │
│  • Orchestrates complex measurements (e.g., 2D sweep = N × 1D sweeps)       │
│  • Dispatches script execution to instrument-script-server                  │
│  • Aggregates results and returns to FALCON                                 │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         instrument-script-server                             │
│                                                                              │
│  • Executes user-provided Lua measurement scripts                           │
│  • Provides RuntimeContext API for instrument control                       │
│  • Manages instrument communication via plugins                             │
│  • Returns structured measurement results                                   │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Physical Instruments                               │
│                                                                              │
│  QDAC, DMM, Lock-in Amplifier, Oscilloscope, etc.                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Key Principle

**The hub does NOT auto-generate Lua scripts.** Instead:

1. **Experimenters** create reusable Lua measurement scripts
2. **The hub** orchestrates complex measurements by calling simpler scripts multiple times
3. **Scripts** are executed on the instrument-script-server

## Script Location

Place your Lua scripts in the hub's scripts directory:

```
falcon-instrument-hub/
  runtime/
    scripts/
      set_voltage.lua
      get_voltage.lua
      sweep_1d.lua
      ramp_voltage.lua
      measure_current.lua
      dc_get_set.lua
      your_custom_script.lua
```

## RuntimeContext API

All scripts receive a `RuntimeContext` object that provides instrument control:

### ctx:call(target, params)

Execute an instrument command and get a `MeasurementResponse`.

```lua
-- Set a voltage
ctx:call("QDAC1.SET_VOLTAGE", { channel = 1, voltage = -0.5 })

-- Read a voltage (returns MeasurementResponse)
local response = ctx:call("DMM1.GET_VOLTAGE", { channel = 0 })
local value = response:value()  -- Extract the numeric value

-- Channel addressing via colon
ctx:call("QDAC1:1.SET_VOLTAGE", { voltage = -0.5 })
```

### ctx:parallel(function)

Execute multiple commands in parallel for synchronized timing:

```lua
ctx:parallel(function()
    ctx:call("QDAC1.SET_VOLTAGE", { channel = 1, voltage = -0.5 })
    ctx:call("QDAC1.SET_VOLTAGE", { channel = 2, voltage = -0.6 })
    ctx:call("QDAC1.SET_VOLTAGE", { channel = 3, voltage = -0.7 })
end)
-- All three voltage sets happen simultaneously
```

### ctx:log(message)

Log a message visible in the hub:

```lua
ctx:log("Starting 1D sweep from -1V to 0V")
ctx:log(string.format("Progress: %d%%", progress))
```

### ctx:error(message)

Report a non-fatal error:

```lua
if not response then
    ctx:error("Failed to read current")
end
```

### MeasurementResponse Methods

Responses from `ctx:call()` have these methods:

```lua
local response = ctx:call("DMM1.GET_VOLTAGE", { channel = 0 })

response:value()         -- Get the raw value
response:type()          -- "float", "integer", "string", "boolean", "buffer"
response:instrument()    -- "DMM1"
response:verb()          -- "GET_VOLTAGE"

-- Math on responses
local offset_response = response:add_offset(1e-9)      -- Add nanoampere offset
local scaled_response = response:multiply_gain(1000)   -- Scale by gain
```

## Required Scripts

The hub expects these standard scripts to exist:

### set_voltage.lua

Sets a single gate voltage.

```lua
---@param ctx RuntimeContext
---@param params {instrument: string, channel: number, voltage: number}
function main(ctx, params)
    ctx:log(string.format("Setting %s:%d to %.4f V", 
        params.instrument, params.channel, params.voltage))
    
    ctx:call(params.instrument .. ".SET_VOLTAGE", {
        channel = params.channel,
        voltage = params.voltage
    })
    
    return nil
end
```

### get_voltage.lua

Reads voltage from an instrument.

```lua
---@param ctx RuntimeContext
---@param params {instrument: string, channel: number}
---@return MeasurementResponse
function main(ctx, params)
    local response = ctx:call(params.instrument .. ".GET_VOLTAGE", {
        channel = params.channel
    })
    
    return {
        instrument = params.instrument,
        channel = params.channel,
        value = response:value(),
        type = response:type()
    }
end
```

### sweep_1d.lua

Performs a 1D voltage sweep with current measurement.

```lua
---@param ctx RuntimeContext
---@param params {sweepInstrument: string, sweepChannel: number, startVoltage: number, stopVoltage: number, numPoints: number, settlingTimeMs: number, currentMeter: string, currentChannel: number}
---@return table Array of {voltage, current} pairs
function main(ctx, params)
    ctx:log(string.format("1D sweep: %s:%d from %.4f to %.4f V (%d points)",
        params.sweepInstrument, params.sweepChannel,
        params.startVoltage, params.stopVoltage, params.numPoints))
    
    local results = {}
    local step = (params.stopVoltage - params.startVoltage) / (params.numPoints - 1)
    
    for i = 0, params.numPoints - 1 do
        local voltage = params.startVoltage + (i * step)
        
        -- Set sweep voltage
        ctx:call(params.sweepInstrument .. ".SET_VOLTAGE", {
            channel = params.sweepChannel,
            voltage = voltage
        })
        
        -- Wait for settling
        if params.settlingTimeMs > 0 then
            -- Note: ctx:sleep() may need to be implemented
            -- For now, this is a placeholder
        end
        
        -- Read current
        local current_resp = ctx:call(params.currentMeter .. ".GET_VOLTAGE", {
            channel = params.currentChannel
        })
        
        table.insert(results, {
            voltage = voltage,
            current = current_resp:value()
        })
        
        -- Progress logging every 10%
        if i % math.floor(params.numPoints / 10) == 0 then
            ctx:log(string.format("Sweep progress: %d%%", 
                math.floor(i * 100 / params.numPoints)))
        end
    end
    
    ctx:log(string.format("Sweep complete: %d points collected", #results))
    return results
end
```

### ramp_voltage.lua

Ramps a gate voltage at a specified slope (V/sec).

```lua
---@param ctx RuntimeContext
---@param params {instrument: string, channel: number, targetV: number, slopeVPerSec: number}
function main(ctx, params)
    ctx:log(string.format("Ramping %s:%d to %.4f V at %.3f V/sec",
        params.instrument, params.channel, params.targetV, params.slopeVPerSec))
    
    -- Read current voltage
    local current_resp = ctx:call(params.instrument .. ".GET_VOLTAGE", {
        channel = params.channel
    })
    local currentV = current_resp:value()
    
    -- Calculate ramp
    local deltaV = params.targetV - currentV
    local rampTime = math.abs(deltaV / params.slopeVPerSec)
    local numSteps = math.max(1, math.floor(rampTime * 100)) -- 100 steps/sec
    local stepV = deltaV / numSteps
    
    for i = 1, numSteps do
        local v = currentV + (i * stepV)
        ctx:call(params.instrument .. ".SET_VOLTAGE", {
            channel = params.channel,
            voltage = v
        })
    end
    
    -- Final set to exact target
    ctx:call(params.instrument .. ".SET_VOLTAGE", {
        channel = params.channel,
        voltage = params.targetV
    })
    
    ctx:log("Ramp complete")
    return nil
end
```

### dc_get_set.lua

DC measurement: set voltages, then read currents.

```lua
---@param ctx RuntimeContext
---@param params {setters: table, getters: table, setVoltages: table, sampleRate: number}
function main(ctx, params)
    ctx:log("Starting DC get/set measurement")
    
    -- Set voltages in parallel
    ctx:parallel(function()
        for _, setter in ipairs(params.setters) do
            local key = setter.id
            if setter.channel ~= nil and setter.channel ~= 0 then
                key = setter.id .. ":" .. tostring(setter.channel)
            end
            local voltage = params.setVoltages[key] or 0
            
            ctx:call(setter.id .. ".SET_VOLTAGE", {
                channel = setter.channel or 0,
                voltage = voltage
            })
        end
    end)
    
    -- Brief settling
    -- ctx:sleep(1)  -- If available
    
    -- Read measurements in parallel
    local results = {}
    ctx:parallel(function()
        for _, getter in ipairs(params.getters) do
            local resp = ctx:call(getter.id .. ".GET_VOLTAGE", {
                channel = getter.channel or 0
            })
            table.insert(results, {
                instrument = getter.id,
                channel = getter.channel or 0,
                value = resp:value()
            })
        end
    end)
    
    ctx:log(string.format("DC measurement complete: %d readings", #results))
    return results
end
```

## How 2D Sweeps Work

When FALCON requests a 2D voltage sweep (`measure_2D_buffered`), the hub **orchestrates** it:

```
FALCON Request: measure_2D_buffered
    X-axis: QDAC1:1, -0.5V to 0.5V, 101 steps
    Y-axis: QDAC1:2, -0.5V to 0.5V, 101 steps

Hub Orchestration:
    FOR each Y value (0 to 100):
        1. Call set_voltage.lua: QDAC1:2 = Y_voltage[y]
        2. Wait for settling
        3. Call sweep_1d.lua: sweep QDAC1:1 from -0.5V to 0.5V
        4. Store the 1D current trace
        5. Call ramp_voltage.lua: ramp QDAC1:1 back to -0.5V
    
    Aggregate all 101 traces into 2D result
    Return to FALCON: 101 × 101 current matrix
```

The experimenter only needs to provide the primitive scripts (`set_voltage`, `sweep_1d`, `ramp_voltage`). The hub handles the orchestration logic.

## Type Definitions (for LSP)

If you want IDE autocomplete, use the Emmy headers from `falcon-measurement-lib`:

```lua
---@class RuntimeContext
---@field call fun(self: RuntimeContext, target: string, params: table): MeasurementResponse
---@field parallel fun(self: RuntimeContext, block: function)
---@field log fun(self: RuntimeContext, msg: string)
---@field error fun(self: RuntimeContext, msg: string)

---@class MeasurementResponse
---@field value fun(self: MeasurementResponse): any
---@field type fun(self: MeasurementResponse): string
---@field instrument fun(self: MeasurementResponse): string
---@field verb fun(self: MeasurementResponse): string
---@field add_offset fun(self: MeasurementResponse, offset: number): MeasurementResponse
---@field multiply_gain fun(self: MeasurementResponse, gain: number): MeasurementResponse

---@class InstrumentTarget
---@field id string
---@field channel number?
```

## Testing Scripts

Test your scripts locally before deploying:

```bash
# Start instrument-script-server in test mode
cd instrument-script-server
./bin/instrument_server --test-mode --config examples/demo_instrument.yaml

# Execute a script
curl -X POST http://localhost:8080/api/v1/execute \
  -H "Content-Type: application/json" \
  -d '{
    "scriptName": "sweep_1d",
    "scriptPath": "/path/to/scripts/sweep_1d.lua",
    "parameters": {
      "sweepInstrument": "QDAC1",
      "sweepChannel": 1,
      "startVoltage": -1.0,
      "stopVoltage": 0.0,
      "numPoints": 101,
      "settlingTimeMs": 1.0,
      "currentMeter": "DMM1",
      "currentChannel": 0
    }
  }'
```

## Best Practices

1. **Keep scripts atomic** - Each script should do one thing well
2. **Use parallel blocks** - For synchronized timing on voltage sets
3. **Log progress** - For long measurements, log progress every ~10%
4. **Handle errors gracefully** - Use `ctx:error()` for recoverable issues
5. **Document parameters** - Use Emmy annotations for IDE support
6. **Return structured data** - Return tables with named fields, not just arrays

## See Also

- `falcon-measurement-lib/schemas/scripts/` - JSON schemas for measurement types
- `falcon-measurement-lib/docs/USAGE.md` - Schema documentation
- `instrument-script-server/examples/scripts/` - Example scripts
- `falcon-instrument-hub/runtime/internal/serverinterpreter/measurement_orchestrator.go` - Hub orchestration code
