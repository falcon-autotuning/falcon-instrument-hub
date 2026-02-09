-- measure_current.lua
-- Measures current from a specified ohmic/channel
--
-- This is a simple measurement script for reading current values.
-- It wraps the instrument call and returns a properly formatted
-- MeasurementResponse-compatible result.
--
-- Parameters:
--   instrument: string - Current meter instrument name (e.g., "DMM1", "SR830")
--   channel: number - Channel to read
--   averaging: number (optional) - Number of readings to average
--   label: string (optional) - Label for this measurement
--
-- Returns: MeasurementResponse-compatible table

---@param ctx RuntimeContext
---@param params {instrument: string, channel: number, averaging?: number, label?: string}
---@return MeasurementResponse
function main(ctx, params)
    local instrument = params.instrument
    local channel = params.channel
    local averaging = params.averaging or 1
    local label = params.label or string.format("%s_ch%d", instrument, channel)
    
    ctx:log(string.format("Measuring current: %s:%d (avg=%d)",
        instrument, channel, averaging))
    
    local sum = 0
    local readings = {}
    
    for i = 1, averaging do
        local resp = ctx:call(instrument .. ".GET_VOLTAGE", {
            channel = channel
        })
        local value = resp:value()
        table.insert(readings, value)
        sum = sum + value
    end
    
    local avgValue = sum / averaging
    
    -- Calculate standard deviation if averaging > 1
    local stdDev = 0
    if averaging > 1 then
        local sumSq = 0
        for _, v in ipairs(readings) do
            sumSq = sumSq + (v - avgValue)^2
        end
        stdDev = math.sqrt(sumSq / (averaging - 1))
    end
    
    ctx:log(string.format("Current: %.6e A (std=%.6e)", avgValue, stdDev))
    
    -- Return MeasurementResponse-compatible table
    return {
        value = function() return avgValue end,
        type = function() return "current" end,
        instrument = function() return instrument end,
        verb = function() return "GET_VOLTAGE" end,
        -- Extended metadata
        metadata = {
            label = label,
            averaging = averaging,
            stdDev = stdDev,
            readings = readings
        }
    }
end
