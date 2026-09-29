

















local function Get_Voltage(
   ctx,
   getter)

   return Mock1Source1:getVoltage(getter.id, getter.channel)
end

return { main = Get_Voltage }
