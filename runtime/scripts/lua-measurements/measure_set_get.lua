local source = require("source")
local multimeter = require("multimeter")

---@param ctx RuntimeContext
---@param setter InstrumentTarget
---@param getter InstrumentTarget
---@param voltage number
function main(ctx, setter, getter, voltage)
    source:setVoltage(
        setter:get_instrument_name(),
        setter:get_channel_group(),
        setter:get_channel(),
        voltage
    )

    return multimeter:getDatapoint(
        getter:get_instrument_name(),
        getter:get_channel_group(),
        getter:get_channel()
    )
end
