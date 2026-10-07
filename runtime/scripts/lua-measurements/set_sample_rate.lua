

















local function Set_Sample_Rate(
   ctx,
   getter,
   sampleRate)

   if getter.id ~= "Meter1" then
      ctx:error("Invalid getter id: " .. getter.id)
      return nil
   end
   Mock5Meter1:setSampleRate(getter.id, getter.channel, sampleRate)
end

return { main = Set_Sample_Rate }
