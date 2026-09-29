

















local function Set_Slope(
   ctx,
   setter,
   slope)

   if setter.id ~= "Meter1" then
      ctx:error("Invalid setter id: " .. setter.id)
      return nil
   end
   Mock5Meter1:setSlope(setter.id, setter.channel, slope)
end

return { main = Set_Slope }
