-- sweep_1d.lua
-- Performs a 1D voltage sweep with current measurement
--
-- This is the fundamental sweep script used by the hub to orchestrate
-- more complex measurements like 2D sweeps and averaged sweeps.
--
-- Parameters:
--   sweepInstrument: string - DAC instrument for voltage sweep (e.g., "QDAC1")
--   sweepChannel: number - Channel to sweep
--   startVoltage: number - Start voltage in V
--   stopVoltage: number - Stop voltage in V
--   numPoints: number - Number of points in sweep
--   settlingTimeMs: number - Settling time after each voltage set (ms)
--   currentMeter: string - Instrument for current measurement (e.g., "DMM1")
--   currentChannel: number - Channel for current reading
--
-- Returns: Array of {voltage, current} pairs

---@param ctx RuntimeContext
---@param params {sweepInstrument: string, sweepChannel: number, startVoltage: number, stopVoltage: number, numPoints: number, settlingTimeMs: number, currentMeter: string, currentChannel: number}
---@return table Array of {voltage: number, current: number}
function main(ctx, params)
    ctx:log(string.format("1D sweep: %s:%d from %.4f to %.4f V (%d points)",
        params.sweepInstrument, params.sweepChannel,
        params.startVoltage, params.stopVoltage, params.numPoints))
    
    local results = {}
    local numPoints = params.numPoints
    local step = (params.stopVoltage - params.startVoltage) / (numPoints - 1)
    
    -- Perform the sweep
    for i = 0, numPoints - 1 do
        local voltage = params.startVoltage + (i * step)
        
        -- Set sweep voltage
        ctx:call(params.sweepInstrument .. ".SET_VOLTAGE", {
            channel = params.sweepChannel,
            voltage = voltage
        })
        
        -- Settling time (if ctx:sleep is available)
        -- Note: Actual implementation depends on instrument-script-server
        -- providing a sleep function. For hardware timing, this may be
        -- built into the instrument driver instead.
        
        -- Read current
        local current_resp = ctx:call(params.currentMeter .. ".GET_VOLTAGE", {
            channel = params.currentChannel
        })
        
        table.insert(results, {
            voltage = voltage,
            current = current_resp:value()
        })
        
        -- Progress logging every 10%
        local logInterval = math.max(1, math.floor(numPoints / 10))
        if i % logInterval == 0 then
            ctx:log(string.format("Sweep progress: %d%% (V=%.4f)", 
                math.floor(i * 100 / numPoints), voltage))
        end
    end
    
    ctx:log(string.format("Sweep complete: %d points collected", #results))
    return results
end
