















local function Measure_Illumination(
   ctx,
   sampleRate,
   getters,
   illuminationTime)

   local getter = getters[1]
   ctx:log("Illumination time: " .. tostring(illuminationTime))
   Mock5Meter1:setSampleRate(getter.id, getter.channel, sampleRate)
   return Mock5Meter1:getDatapoint(getter.id, getter.channel)
end

return { main = Measure_Illumination }
