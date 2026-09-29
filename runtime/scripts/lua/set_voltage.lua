

















local function Set_Voltage(
   ctx,
   setter,
   setVoltage)

   if setter.id ~= "Source1" then
      ctx:error("Invalid setter id: " .. setter.id)
      return nil
   end
   Mock1Source1:setVoltage(setter.id, setter.channel, setVoltage)
end
return { main = Set_Voltage }
