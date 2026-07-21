-- get_voltage.lua
-- Reads voltage from an instrument
--
-- Parameters:
--   instrument: string - Instrument ID (e.g., "DMM1")
--   channel: number - Channel number
--
-- Returns: MeasurementResponse with voltage value

---@param ctx RuntimeContext
---@param params {instrument: string, channel: number}
---@return table MeasurementResponse-like table
function main(ctx, params)
    local cs = instrument_call_stack.new({
        instrument = params.instrument,
        command = "GET_VOLTAGE",
        channel = params.channel
    })
    local response = ctx:call(cs)
    
    ctx:log(string.format("Read %s:%d = %.6f V", 
        params.instrument, params.channel, response:value()))
    
    return {
        instrument = params.instrument,
        channel = params.channel,
        value = response:value(),
        type = response:type()
    }
end
