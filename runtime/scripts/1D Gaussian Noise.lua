











local function Gaussian1D(
   ctx,
   getters,
   numPoints,
   sampleRate,
   setVoltages,
   setters)


   for _, setter in ipairs(setters) do
      local voltage = setVoltages[setter.id]
      if voltage == nil then
         ctx:error("No voltage specified for setter id: " .. setter.id)
         return nil
      end
      if setter.id ~= "Source1" then
         ctx:error("Invalid setter id: " .. setter.id)
         return nil
      end
      Mock1Source1:setVoltage(setter.id, setter.channel, voltage)
   end
   for _, getter in ipairs(getters) do
      Mock5Meter1:setSampleRate(getter.id, getter.channel, sampleRate)
      Mock5Meter1:setBins(getter.id, getter.channel, numPoints)
   end

   for _, getter in ipairs(getters) do
      Mock5Meter1:measureStream(getter.id, getter.channel)
   end
   return ""
end
return { main = Gaussian1D }
