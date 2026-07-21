-- dc_get_set.lua
-- Performs get/set operations for DC measurements
--
-- This script handles the common case of setting multiple voltages
-- and measuring multiple currents.
--
-- Parameters:
--   sets: array of {instrument: string, channel: number, voltage: number}
--   gets: array of {instrument: string, channel: number, label?: string}
--   settlingTimeMs: number (optional) - Time to wait after sets before gets
--
-- Returns: Object with measurement results keyed by label or index

---@param ctx RuntimeContext
---@param params {sets: table[], gets: table[], settlingTimeMs?: number}
---@return table<string, number> Measurement results keyed by label
function main(ctx, params)
    local sets = params.sets or {}
    local gets = params.gets or {}
    local settlingTimeMs = params.settlingTimeMs or 10
    
    ctx:log(string.format("DC measurement: %d sets, %d gets", #sets, #gets))
    
    for i, s in ipairs(sets) do
        local cs = instrument_call_stack.new({
            instrument = s.instrument,
            command = "SET_VOLTAGE"
        })
        ctx:call(cs, {
            channel = s.channel,
            voltage = s.voltage
        })
    end
    ctx:log(string.format("Set %d voltages", #sets))
    
    -- Note: Settling time would be handled here if ctx:sleep is available
    -- For now, we rely on instrument-level settling
    
    local results = {}
    for i, g in ipairs(gets) do
        local cs = instrument_call_stack.new({
            instrument = g.instrument,
            command = "GET_VOLTAGE"
        })
        local resp = ctx:call(cs, {
            channel = g.channel
        })
        local label = g.label or string.format("%s_ch%d", g.instrument, g.channel)
        local value = resp:value()
        results[label] = value
        ctx:log(string.format("  %s = %.6f", label, value))
    end
    
    return results
end
