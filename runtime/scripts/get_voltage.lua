-- The hub creates this CallStack from the resolved instrument port and ISS
-- converts the typed serialized value into CallStack userdata.
---@param ctx RuntimeContext
---@param getter CallStack
function main(ctx, getter)
    ctx:call(getter)
end
