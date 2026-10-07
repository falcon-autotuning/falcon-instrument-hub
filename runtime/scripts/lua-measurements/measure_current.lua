















local function Measure_Current(
   ctx,
   sampleRate,
   getters)

   local getter = getters[1]
   Mock5Meter1:setSampleRate(getter.id, getter.channel, sampleRate)
   return Mock5Meter1:getDatapoint(getter.id, getter.channel)
end

return { main = Measure_Current }
