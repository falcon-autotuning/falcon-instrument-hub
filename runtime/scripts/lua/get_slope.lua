

















local function Get_Slope(
   ctx,
   getter)

   return Mock5Meter1:getSlope(getter.id, getter.channel)
end

return { main = Get_Slope }
