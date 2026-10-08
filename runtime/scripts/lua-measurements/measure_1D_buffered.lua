local source = require("source")
local multimeter = require("multimeter")

---@param ctx RuntimeContext
---@param setter InstrumentTarget
---@param getter InstrumentTarget
---@param voltages number[]
---@param sampleRate integer
function main(ctx, setter, getter, voltages, sampleRate)
    multimeter:setSampleRate(
        getter:get_instrument_name(),
        getter:get_channel_group(),
        getter:get_channel(),
        sampleRate
    )
    multimeter:setBins(
        getter:get_instrument_name(),
        getter:get_channel_group(),
        getter:get_channel(),
        1
    )

    -- Each stream call returns a one-sample DataBuffer. The Go handler
    -- consumes them in this order to build the 1D measurement array.
    for _, voltage in ipairs(voltages) do
        source:setVoltage(
            setter:get_instrument_name(),
            setter:get_channel_group(),
            setter:get_channel(),
            voltage
        )
        multimeter:measureStream(
            getter:get_instrument_name(),
            getter:get_channel_group(),
            getter:get_channel()
        )
    end
end
