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
    
    local cs = instrument_call_stack.new({
        instrument = params.instrument,
        command = "SET_VOLTAGE",
        channel = params.channel
    })
    ctx:call(cs, {
        channel = params.channel,
        voltage = params.voltage
    })
    
    return nil
end
