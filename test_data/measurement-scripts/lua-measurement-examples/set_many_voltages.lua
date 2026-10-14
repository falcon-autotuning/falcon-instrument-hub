-- set many voltages on various instrument channels.
local source = require("source")

local function target_key(target)
   return target.id .. ":" .. tostring(target.channel)
end

local function Set_Many_Voltages(
   ctx,
   setters,
   setVoltages -- assumes a 1:1 mapping of setters to voltages in the same order
)

   for idx, setter in ipairs(setters) do
      local setVoltage = setVoltages[idx]
      source:setVoltage(setter.id, setter.channel, setVoltage)
   end
end

return { main = Set_Many_Voltages }
