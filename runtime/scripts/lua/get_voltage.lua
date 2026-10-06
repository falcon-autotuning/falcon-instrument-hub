local source = require("source")

---@param ctx RuntimeContext
---@param getter InstrumentTarget
function main(ctx, getter)
    return source:getVoltage(
        getter:get_instrument_name(),
        getter:get_channel_group(),
        getter:get_channel()
    )
end
