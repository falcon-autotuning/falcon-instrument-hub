-- Set source voltages, configure the meters, then acquire one stream per meter.
-- Each setter/getter is a native InstrumentTarget resolved by the Go handler.
local source = require("source")
local multimeter = require("multimeter")

-- NOTE: measure_leakage.lua and measure_current.lau are simply copies of this 
-- file with the multimeter library replaced by the ammeter library. The ammeter 
-- library must implement the same interface as the multimeter library, and the 
-- handler must support its output units.

-- The handler must pass these five arguments in this order after ctx.
-- This adapts the measurement-lib schema to the current hub transport:
-- targets use accessor methods, and setVoltages is an array rather than an
-- instrument-id map. setVoltages[i] belongs to setters[i], so separate channels
-- on the same instrument can receive different voltages.
--
-- The supplied multimeter library implements voltage acquisition. An ammeter
-- needs a matching instrument library and handler support for its output units;
-- changing the getter target alone does not change the measurement command.
---@param ctx RuntimeContext Receives validation errors.
---@param getters InstrumentTarget[] Meter channels to read, in result order.
---@param numPoints integer Positive bin count passed to SET_BINS, not sweep steps.
---@param sampleRate integer Positive sampling rate in Hz for every getter.
---@param setVoltages number[] Source voltages in V, one per setter.
---@param setters InstrumentTarget[] Source channels to set, in voltage-array order.
local function Measure_Get_Set(
   ctx,
   getters,
   numPoints,
   sampleRate,
   setVoltages,
   setters)

   -- A target identifies where to send the command; the library selects the ISA
   -- command. Source and meter targets may refer to different instruments.
   for i, setter in ipairs(setters) do
      source:setVoltage(
         setter:get_instrument_name(),
         setter:get_channel_group(),
         setter:get_channel(),
         setVoltages[i]
      )
   end

   for _, getter in ipairs(getters) do
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
         numPoints
      )
   end
   for _, getter in ipairs(getters) do

      -- GET_DATAPOINT is a mock ISA command which returns a single-sample 
      -- DataBuffer per getter.
      multimeter:getDatapoint(
         getter:get_instrument_name(),
         getter:get_channel_group(),
         getter:get_channel()
      )
   end

   -- ISS collects the results of context:call inside the instrument libraries.
   -- No Lua result table is required. The handler must extract each stream from
   -- the recorded calls, consume its buffer, and construct the Falcon response.
end
return { main = Measure_Get_Set }
