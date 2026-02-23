-- sweep_2d.lua
-- Performs a 2D voltage sweep with current measurement
--
-- This script orchestrates a full 2D sweep by calling the 1D sweep script
-- multiple times - once for each Y-axis value. It provides a Lua-level
-- implementation of 2D sweep orchestration that can be used standalone
-- or as a template for custom multi-dimensional sweeps.
--
-- The sweep follows this pattern:
--   For each Y value (slow axis):
--     1. Set Y gate to target voltage
--     2. Wait for settling
--     3. Execute 1D sweep along X axis (fast axis)
--     4. Collect the 1D trace data
--     5. Ramp X gate back to start voltage
--   Aggregate all traces into 2D result
--
-- This script defines a plane in multi-variable voltage space using two
-- perpendicular axes:
--   - X-axis (fast): sweep direction defined by xGate, xStart, xStop
--   - Y-axis (slow): perpendicular direction defined by yGate, yStart, yStop
--
-- The perpendicularity is maintained by sweeping one gate while holding
-- the other constant at each Y slice.
--
-- Parameters:
--   xGate: string - Gate name for X sweep (e.g., "P1")
--   xInstrument: string - Instrument for X sweep (e.g., "QDAC1")
--   xChannel: number - Channel for X sweep
--   xStartV: number - X start voltage
--   xStopV: number - X stop voltage
--   xNumPoints: number - Number of X points per line
--
--   yGate: string - Gate name for Y sweep (e.g., "P2")
--   yInstrument: string - Instrument for Y sweep
--   yChannel: number - Channel for Y sweep
--   yStartV: number - Y start voltage
--   yStopV: number - Y stop voltage
--   yNumPoints: number - Number of Y lines
--
--   currentMeter: string - Instrument for current measurement
--   currentChannel: number - Channel for current reading
--   settlingTimeMs: number - Settling time after voltage changes (ms)
--   rampSlopeVPerS: number - Ramp rate for returning to start (V/s)
--
--   staticVoltages: table (optional) - Map of gate->voltage for static gates
--
-- Returns: 2D sweep result with structure:
--   {
--     xVoltages: number[] - X-axis voltage values
--     yVoltages: number[] - Y-axis voltage values
--     currentData: number[][] - [y][x] array of current values
--     lines: Sweep1DLine[] - Individual 1D sweep results
--   }

---@class Sweep2DParams
---@field xGate string
---@field xInstrument string
---@field xChannel number
---@field xStartV number
---@field xStopV number
---@field xNumPoints number
---@field yGate string
---@field yInstrument string
---@field yChannel number
---@field yStartV number
---@field yStopV number
---@field yNumPoints number
---@field currentMeter string
---@field currentChannel number
---@field settlingTimeMs number
---@field rampSlopeVPerS number
---@field staticVoltages? table<string, number>

---@class Sweep1DLine
---@field yVoltage number
---@field yIndex number
---@field xVoltages number[]
---@field currents number[]
---@field timestamp number

---@class Sweep2DResult
---@field xVoltages number[]
---@field yVoltages number[]
---@field currentData number[][]
---@field lines Sweep1DLine[]
---@field xGate string
---@field yGate string

---@param ctx RuntimeContext
---@param params Sweep2DParams
---@return Sweep2DResult
function main(ctx, params)
    ctx:log(string.format("Starting 2D sweep: %s x %s", params.xGate, params.yGate))
    ctx:log(string.format("  X: %s:%d [%.4f to %.4f V] %d points",
        params.xInstrument, params.xChannel,
        params.xStartV, params.xStopV, params.xNumPoints))
    ctx:log(string.format("  Y: %s:%d [%.4f to %.4f V] %d points",
        params.yInstrument, params.yChannel,
        params.yStartV, params.yStopV, params.yNumPoints))
    
    -- Initialize result structure
    local result = {
        xGate = params.xGate,
        yGate = params.yGate,
        xVoltages = {},
        yVoltages = {},
        currentData = {},
        lines = {},
        startTime = os.time()
    }
    
    -- Pre-compute voltage arrays
    local xStep = (params.xStopV - params.xStartV) / (params.xNumPoints - 1)
    local yStep = (params.yStopV - params.yStartV) / (params.yNumPoints - 1)
    
    for i = 0, params.xNumPoints - 1 do
        table.insert(result.xVoltages, params.xStartV + i * xStep)
    end
    
    for i = 0, params.yNumPoints - 1 do
        table.insert(result.yVoltages, params.yStartV + i * yStep)
    end
    
    -- Step 1: Set static gate voltages if provided
    if params.staticVoltages then
        ctx:log("Setting static gate voltages...")
        for gate, voltage in pairs(params.staticVoltages) do
            ctx:call("set_voltage", {
                gate = gate,
                voltage = voltage
            })
        end
    end
    
    -- Step 2: Execute Y sweep (slow axis)
    for yIdx = 0, params.yNumPoints - 1 do
        local yVoltage = result.yVoltages[yIdx + 1]
        
        -- Progress reporting
        local progress = (yIdx * 100) / params.yNumPoints
        ctx:log(string.format("2D sweep progress: %.1f%% (Y line %d/%d at %.4f V)",
            progress, yIdx + 1, params.yNumPoints, yVoltage))
        
        -- 2a. Set Y gate voltage
        ctx:call("set_voltage", {
            instrument = params.yInstrument,
            channel = params.yChannel,
            voltage = yVoltage
        })
        
        -- 2b. Wait for settling (if supported by runtime)
        -- Note: Actual settling implementation depends on instrument-script-server
        -- Some systems may rely on hardware settling times instead
        if ctx.sleep and params.settlingTimeMs > 0 then
            ctx:sleep(params.settlingTimeMs)
        end
        
        -- 2c. Execute 1D sweep along X axis
        local sweep1DParams = {
            sweepInstrument = params.xInstrument,
            sweepChannel = params.xChannel,
            startVoltage = params.xStartV,
            stopVoltage = params.xStopV,
            numPoints = params.xNumPoints,
            settlingTimeMs = params.settlingTimeMs,
            currentMeter = params.currentMeter,
            currentChannel = params.currentChannel
        }
        
        local sweep1DResult = ctx:call("sweep_1d", sweep1DParams)
        
        -- 2d. Extract current values from 1D sweep result
        local currents = {}
        local sweep1DData = sweep1DResult:value()
        
        -- Parse the 1D sweep result - it should be an array of {voltage, current} pairs
        if type(sweep1DData) == "table" then
            for i, point in ipairs(sweep1DData) do
                if type(point) == "table" and point.current then
                    table.insert(currents, point.current)
                elseif type(point) == "number" then
                    -- Fallback: if it's just numbers, assume they're current values
                    table.insert(currents, point)
                end
            end
        end
        
        -- Verify we got the expected number of points
        if #currents ~= params.xNumPoints then
            ctx:error(string.format(
                "1D sweep returned %d points, expected %d",
                #currents, params.xNumPoints))
            return nil
        end
        
        -- Store the line data
        local line = {
            yVoltage = yVoltage,
            yIndex = yIdx,
            xVoltages = result.xVoltages,
            currents = currents,
            timestamp = os.time()
        }
        table.insert(result.lines, line)
        table.insert(result.currentData, currents)
        
        -- 2e. Ramp X gate back to start voltage for next iteration
        if yIdx < params.yNumPoints - 1 then  -- Skip ramp on last iteration
            ctx:call("ramp_voltage", {
                instrument = params.xInstrument,
                channel = params.xChannel,
                targetVoltage = params.xStartV,
                rampRateVperS = params.rampSlopeVPerS
            })
        end
    end
    
    result.endTime = os.time()
    result.totalDurationSec = result.endTime - result.startTime
    
    ctx:log(string.format("2D sweep complete: %d x %d = %d points collected",
        params.yNumPoints, params.xNumPoints,
        params.yNumPoints * params.xNumPoints))
    ctx:log(string.format("Total duration: %d seconds", result.totalDurationSec))
    
    return result
end
