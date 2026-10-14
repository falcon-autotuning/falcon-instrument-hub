-- set many voltages on various instrument channels.
local source = require("source")

local function Get_Many_Voltages(
   ctx,
   getters -- assumes a 1:1 mapping of getters to voltages in the same order
)

   for _, getter in ipairs(getters) do
      source:getVoltage(getter.id, getter.channel)
   end
end

return { main = Get_Many_Voltages }