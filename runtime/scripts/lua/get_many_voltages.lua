local _tl_compat; if (tonumber((_VERSION or ''):match('[%d.]*$')) or 0) < 5.3 then local p, m = pcall(require, 'compat53.module'); if p then _tl_compat = m end end; local ipairs = _tl_compat and _tl_compat.ipairs or ipairs















local function Get_Many_Voltages(
   ctx,
   getters)

   for _, getter in ipairs(getters) do
      Mock1Source1:getVoltage(getter.id, getter.channel)
   end
   return ""
end

return { main = Get_Many_Voltages }
