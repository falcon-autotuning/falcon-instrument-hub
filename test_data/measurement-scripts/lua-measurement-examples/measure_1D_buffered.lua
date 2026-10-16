-- Set source voltages, configure the meters, then acquire one stream per meter.
-- Each setter/getter is a native InstrumentTarget resolved by the Go handler.
local source = require("source")
local multimeter = require("multimeter")

-- NOTE: ASSUMING single channel sweep with single setVoltageDomain.

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

   for _, setter in ipairs(bufferedSetters) do
      source:setVoltage(
      setter.id,
      setter.channel,
      resolveEndVoltage(setVoltageDomains, setter))

   end

   -- start, stop := setVoltageDomains, 1.0
   -- steps := 5
   -- for i := 0; i < steps; i++ {
   --    v := start + float64(i)*(stop-start)/float64(steps-1)
   --    fmt.Println(v) // 0, 0.25, 0.5, 0.75, 1
   -- }

   for _, getter in ipairs(getters) do
      multimeter:measureStream(getter.id, getter.channel)
   end

   return ""
end

return { main = Measure1DBuffered }
