local _tl_compat; if (tonumber((_VERSION or ''):match('[%d.]*$')) or 0) < 5.3 then local p, m = pcall(require, 'compat53.module'); if p then _tl_compat = m end end; local ipairs = _tl_compat and _tl_compat.ipairs or ipairs; local math = _tl_compat and _tl_compat.math or math
































local function resolveEndVoltage(
   setVoltageDomains,
   target)

   local domain = setVoltageDomains[target.id]
   if domain == nil then
      return 0.0
   end
   return domain.max
end

local function Measure1DBuffered(
   ctx,
   sampleRate,
   setters,
   setVoltageDomains,
   bufferedGetters,
   numPoints,
   numSteps,
   bufferedSetters)

   local bins = math.max(numPoints, numSteps)

   for _, setter in ipairs(setters) do
      Mock1Source1:setVoltage(
      setter.id,
      setter.channel,
      resolveEndVoltage(setVoltageDomains, setter))

   end
   for _, setter in ipairs(bufferedSetters) do
      Mock1Source1:setVoltage(
      setter.id,
      setter.channel,
      resolveEndVoltage(setVoltageDomains, setter))

   end
   for _, getter in ipairs(bufferedGetters) do
      Mock5Meter1:setSampleRate(getter.id, getter.channel, sampleRate)
      Mock5Meter1:setBins(getter.id, getter.channel, bins)
   end

   for _, getter in ipairs(bufferedGetters) do
      Mock5Meter1:measureStream(getter.id, getter.channel)
   end

   return ""
end

return { main = Measure1DBuffered }
