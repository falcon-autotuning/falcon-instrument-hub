-- set many voltages on various instrument channels.
local source = require("source")

local function Set_Many_Voltages(
   ctx,
   setters,
   setVoltages
)

   for idx, setter in ipairs(setters) do

      -- NOTE: use source command defined in source instrument's ISA
      source:setVoltage(
         setter:get_instrument_name(),
         setter:get_channel_group(),
         setter:get_channel(),
         setVoltages[idx]
      )
   end
end

return { main = Set_Many_Voltages }
