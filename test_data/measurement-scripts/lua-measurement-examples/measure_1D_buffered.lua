-- Set source voltages, configure the meters, then acquire one stream per meter.
-- Each setter/getter is a native InstrumentTarget resolved by the Go handler.
local source = require("source")
local multimeter = require("multimeter")
local math_utils = require("math")

local function applyVector(setters, vec)
  for k, setter in ipairs(setters) do
   -- NOTE: use source command defined in source instrument's ISA
   source:setVoltage(
      setter:get_instrument_name(),
      setter:get_channel_group(),
      setter:get_channel(),
      vec[k]
    )
  end
end

local function readGetters(getters)
  local point = {}
  for g, getter in ipairs(getters) do
   -- NOTE: use multimeter command defined in multimeter instrument's ISA
    point[g] = multimeter:getDatapoint(
      getter:get_instrument_name(),
      getter:get_channel_group(),
      getter:get_channel()
    )
  end
  return point
end

-- Generic 1D buffered measurement: set all setters to their end voltages, then
local function Measure1DBuffered(
   ctx,
   setVoltageDomains,
   getters,
   numSteps,
   setters)

   for _, setter in ipairs(setters) do
      source:setVoltage(
      setter.id,
      setter.channel,
      resolveEndVoltage(setVoltageDomains, setter))

   end

   local sweep = math_utils.build1DSweep(limits, nSteps)
   for _, vec in ipairs(sweep) do
      applyVector(setters, vec)
      readGetters(getters)
   end
end

return { main = Measure1DBuffered }
