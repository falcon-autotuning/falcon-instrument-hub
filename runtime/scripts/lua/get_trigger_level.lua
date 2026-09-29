

















local function Get_Trigger_Level(
   ctx,
   getter)

   return Mock5Meter1:getTriggerLevel(getter.id, getter.channel)
end

return { main = Get_Trigger_Level }
