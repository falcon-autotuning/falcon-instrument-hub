

















local function Set_Number_Of_Samples(
   ctx,
   getter,
   numberOfSamples)

   if getter.id ~= "Meter1" then
      ctx:error("Invalid getter id: " .. getter.id)
      return nil
   end
   Mock5Meter1:setBins(getter.id, getter.channel, numberOfSamples)
end

return { main = Set_Number_Of_Samples }
