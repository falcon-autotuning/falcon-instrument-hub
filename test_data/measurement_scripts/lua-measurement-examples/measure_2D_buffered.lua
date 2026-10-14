local _tl_compat; if (tonumber((_VERSION or ''):match('[%d.]*$')) or 0) < 5.3 then local p, m = pcall(require, 'compat53.module'); if p then _tl_compat = m end end; local ipairs = _tl_compat and _tl_compat.ipairs or ipairs; local math = _tl_compat and _tl_compat.math or math































local function resolveEndVoltage(
   domains,
   target)

   local domain = domains[target.id]
   if domain == nil then
      return 0.0
   end
   return domain.max
end

local function Measure2DBuffered(
   ctx,
   bufferedXSetters,
   sampleRate,
   bufferedGetters,
   bufferedYSetters,
   numXSteps,
   setYVoltageDomains,
   setXVoltageDomains,
   numPoints,
   numYSteps,
   setters)

   local bins = math.max(numPoints, numXSteps)

   for _, setter in ipairs(setters) do
      Mock1Source1:setVoltage(setter.id, setter.channel, 0.0)
   end

   for _ = 1, numYSteps do
      for _, setter in ipairs(bufferedYSetters) do
         Mock1Source1:setVoltage(
         setter.id,
         setter.channel,
         resolveEndVoltage(setYVoltageDomains, setter))

      end
      for _, setter in ipairs(bufferedXSetters) do
         Mock1Source1:setVoltage(
         setter.id,
         setter.channel,
         resolveEndVoltage(setXVoltageDomains, setter))

      end
      for _, getter in ipairs(bufferedGetters) do
         Mock5Meter1:setSampleRate(getter.id, getter.channel, sampleRate)
         Mock5Meter1:setBins(getter.id, getter.channel, bins)
      end

      for _, getter in ipairs(bufferedGetters) do
         Mock5Meter1:measureStream(getter.id, getter.channel)
      end
   end

   return ""
end

return { main = Measure2DBuffered }
