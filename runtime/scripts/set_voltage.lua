-- set_voltage.lua
-- Sets a single gate voltage
--
-- Parameters:
--   instrument: string - Instrument ID (e.g., "QDAC1")
--   channel: number - Channel number
--   voltage: number - Target voltage in V
--
-- Returns: nil

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
