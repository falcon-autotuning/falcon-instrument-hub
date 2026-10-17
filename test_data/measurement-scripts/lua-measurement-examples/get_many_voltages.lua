-- set many voltages on various instrument channels.
local source = require("source")

local function Get_Many_Voltages(
   ctx,
   getters
)

   for _, getter in ipairs(getters) do
      -- NOTE: use source command defined in source instrument's ISA
      source:getDatapoint(
         getter:get_instrument_name(),
         getter:get_channel_group(),
         getter:get_channel()
      )
   end
end

return { main = Get_Many_Voltages }