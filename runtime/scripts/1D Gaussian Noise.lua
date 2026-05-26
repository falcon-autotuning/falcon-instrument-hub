











local function Gaussian1D(
   ctx,
   getters,
   numPoints,
   sampleRate,
   setVoltages,
   setters)


   ctx:parallel(function()
      for setter in setters do
         local voltage = setVoltages[setter.id]
         if voltage == nil then
            ctx:error("No voltage specified for setter id: " .. setter.id)
            return nil
         end
         if setter.id ~= "Source1" then
            ctx:error("Invalid setter id: " .. setter.id)
            return nil
         end
         Mock1Source1:setVoltage(setter.channel, voltage)
      end
      for getter in getters do
         Mock5Meter1:setSampleRate(getter.channel, sampleRate)
         Mock5Meter1:setBins(getter.channel, numPoints)
      end
   end)

   ctx:parallel(function()
      for getter in getters do
         Mock5Meter1:measureStream(getter.channel)
      end
   end)
   return ""
end
return { main = Gaussian1D }
