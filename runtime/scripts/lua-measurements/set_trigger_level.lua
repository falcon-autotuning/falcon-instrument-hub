

















local function Set_Trigger_Level(
   ctx,
   getter,
   triggerLevel)

   if getter.id ~= "Meter1" then
      ctx:error("Invalid getter id: " .. getter.id)
      return nil
   end
   Mock5Meter1:setTriggerLevel(getter.id, getter.channel, triggerLevel)
end

return { main = Set_Trigger_Level }
