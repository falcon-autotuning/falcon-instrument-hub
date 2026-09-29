local _tl_compat; if (tonumber((_VERSION or ''):match('[%d.]*$')) or 0) < 5.3 then local p, m = pcall(require, 'compat53.module'); if p then _tl_compat = m end end; local ipairs = _tl_compat and _tl_compat.ipairs or ipairs

















local function target_key(target)
   return target.id .. ":" .. tostring(target.channel)
end

local function Set_Many_Voltages(
   ctx,
   setters,
   setVoltages)

   for _, setter in ipairs(setters) do
      local key = target_key(setter)
      local setVoltage = setVoltages[key]
      if setVoltage == nil then
         ctx:error("Missing set voltage for target " .. key)
         return nil
      end
      Mock1Source1:setVoltage(setter.id, setter.channel, setVoltage)
   end
end

return { main = Set_Many_Voltages }
