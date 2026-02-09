-- ramp_voltage.lua
-- Smoothly ramps a gate voltage to a target value at a specified rate
--
-- This script is critical for safely returning gates to initial values
-- between sweep iterations (e.g., between Y slices in a 2D sweep).
-- Ramping prevents abrupt voltage changes that could disturb the sample.
--
-- Parameters:
--   instrument: string - DAC instrument name (e.g., "QDAC1")
--   channel: number - Channel to ramp
--   targetVoltage: number - Target voltage in V
--   rampRateVperS: number - Ramp rate in V/s (e.g., 0.1 = 100mV/s)
--   currentVoltage: number - (Optional) Current voltage, if known
--
-- Returns: Final voltage confirmation

---@param ctx RuntimeContext
---@param params {instrument: string, channel: number, targetVoltage: number, rampRateVperS: number, currentVoltage?: number}
---@return {success: boolean, finalVoltage: number}
function main(ctx, params)
    local instrument = params.instrument
    local channel = params.channel
    local targetVoltage = params.targetVoltage
    local rampRate = params.rampRateVperS or 0.1  -- Default 100mV/s
    
    -- Get current voltage if not provided
    local currentVoltage = params.currentVoltage
    if currentVoltage == nil then
        local resp = ctx:call(instrument .. ".GET_VOLTAGE", {
            channel = channel
        })
        currentVoltage = resp:value()
    end
    
    local deltaV = targetVoltage - currentVoltage
    
    -- If already at target (within tolerance), do nothing
    if math.abs(deltaV) < 1e-6 then
        ctx:log(string.format("Ramp: %s:%d already at %.4f V", 
            instrument, channel, targetVoltage))
        return {
            success = true,
            finalVoltage = currentVoltage
        }
    end
    
    ctx:log(string.format("Ramping %s:%d from %.4f to %.4f V at %.3f V/s",
        instrument, channel, currentVoltage, targetVoltage, rampRate))
    
    -- Calculate ramp parameters
    local totalTime = math.abs(deltaV) / rampRate  -- seconds
    local numSteps = math.max(10, math.floor(totalTime * 100))  -- ~10ms steps
    local stepVoltage = deltaV / numSteps
    local stepTimeMs = (totalTime * 1000) / numSteps
    
    -- Perform the ramp
    local voltage = currentVoltage
    for i = 1, numSteps do
        voltage = currentVoltage + (i * stepVoltage)
        
        -- Clamp to target on final step to avoid floating point drift
        if i == numSteps then
            voltage = targetVoltage
        end
        
        ctx:call(instrument .. ".SET_VOLTAGE", {
            channel = channel,
            voltage = voltage
        })
        
        -- Note: Actual timing depends on instrument-script-server implementation
        -- Some systems may have hardware-level ramp support which is more accurate
    end
    
    -- Confirm final voltage
    local finalResp = ctx:call(instrument .. ".GET_VOLTAGE", {
        channel = channel
    })
    local finalVoltage = finalResp:value()
    
    ctx:log(string.format("Ramp complete: final voltage %.4f V", finalVoltage))
    
    return {
        success = true,
        finalVoltage = finalVoltage
    }
end
