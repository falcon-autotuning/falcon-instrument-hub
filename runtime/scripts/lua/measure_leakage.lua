















local function Measure_Leakage(
   ctx,
   getter,
   voltage)

   ctx:log("Leakage voltage for " .. getter.id .. ": " .. tostring(voltage))
   return voltage
end

return { main = Measure_Leakage }
