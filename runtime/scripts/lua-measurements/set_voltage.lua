local source = require("source")

---@param ctx RuntimeContext
---@param setter InstrumentTarget
---@param voltage number
function main(ctx, setter, voltage)
    return source:setVoltage(
        setter:get_instrument_name(),
        setter:get_channel_group(),
        setter:get_channel(),
        voltage
    )
end