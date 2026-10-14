-- Set source voltages, configure the meters, then acquire one stream per meter.
-- Each setter/getter is a native InstrumentTarget resolved by the Go handler.
local source = require("source")
local multimeter = require("multimeter")

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
local function Measure_Set_Get(
   ctx,
   getters,
   numPoints,
   sampleRate,
   setVoltages,
   setters)

   -- Check inputs before making instrument calls. The handler supplies dense
   -- arrays and validates that targets have the required source/meter capability.
   if type(setters) ~= "table" or #setters == 0 or
      type(getters) ~= "table" or #getters == 0 then
      ctx:error("At least one setter and one getter are required")
      return nil
   end
   if type(setVoltages) ~= "table" or #setVoltages ~= #setters then
      ctx:error("setVoltages must contain exactly one voltage per setter")
      return nil
   end
   if type(numPoints) ~= "number" or numPoints <= 0 or
      numPoints == math.huge or numPoints ~= math.floor(numPoints) then
      ctx:error("numPoints must be a positive integer bin count")
      return nil
   end
   if type(sampleRate) ~= "number" or sampleRate <= 0 or
      sampleRate == math.huge or sampleRate ~= math.floor(sampleRate) then
      ctx:error("sampleRate must be a positive integer in Hz")
      return nil
   end
   for i, voltage in ipairs(setVoltages) do
      if type(voltage) ~= "number" or voltage ~= voltage or
         voltage == math.huge or voltage == -math.huge then
         ctx:error("Invalid voltage for setter at index " .. i)
         return nil
      end
   end

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
      multimeter:measureStream(
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
