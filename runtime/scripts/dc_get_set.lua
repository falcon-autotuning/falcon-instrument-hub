-- dc_get_set.lua
-- Performs parallel get/set operations for DC measurements
--
-- This script handles the common case of setting multiple voltages
-- and measuring multiple currents in parallel. It leverages the
-- RuntimeContext's parallel() method for efficient multi-instrument
-- operations.
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
    
    -- Build parallel set operations
    local setOps = {}
    for i, s in ipairs(sets) do
        table.insert(setOps, function()
            return ctx:call(s.instrument .. ".SET_VOLTAGE", {
                channel = s.channel,
                voltage = s.voltage
            })
        end)
    end
    
    -- Execute all sets in parallel
    if #setOps > 0 then
        local setResults = ctx:parallel(setOps)
        ctx:log(string.format("Set %d voltages in parallel", #setResults))
    end
    
    -- Note: Settling time would be handled here if ctx:sleep is available
    -- For now, we rely on instrument-level settling
    
    -- Build parallel get operations
    local getOps = {}
    local getLabels = {}
    for i, g in ipairs(gets) do
        table.insert(getOps, function()
            return ctx:call(g.instrument .. ".GET_VOLTAGE", {
                channel = g.channel
            })
        end)
        -- Use provided label or generate one
        getLabels[i] = g.label or string.format("%s_ch%d", g.instrument, g.channel)
    end
    
    -- Execute all gets in parallel
    local results = {}
    if #getOps > 0 then
        local getResults = ctx:parallel(getOps)
        
        for i, resp in ipairs(getResults) do
            local label = getLabels[i]
            local value = resp:value()
            results[label] = value
            ctx:log(string.format("  %s = %.6f", label, value))
        end
    end
    
    return results
end
