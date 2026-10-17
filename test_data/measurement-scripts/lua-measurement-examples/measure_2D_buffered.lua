-- 2D raster sweep:
--   for each Y step: set the Y voltage vector,
--     then sweep the full X voltage vector, reading every getter at each X point.
local source = require("source")
local multimeter = require("multimeter")
local sweepMath = require("sweep_math")

local function applyVector(setters, vec)
  for k, setter in ipairs(setters) do
    -- NOTE: use source command defined in source instrument's ISA
    source:setVoltage(
      setter:get_instrument_name(),
      setter:get_channel_group(),
      setter:get_channel(),
      vec[k]
    )
  end
end

local function readGetters(getters)
  local point = {}
  for g, getter in ipairs(getters) do
    -- NOTE: use multimeter command defined in multimeter instrument's ISA
      multimeter:getDatapoint(
        getter:get_instrument_name(),
        getter:get_channel_group(),
        getter:get_channel()
      )
  end
end

-- xSetters / ySetters:  control elements swept along the X (fast) axis and
--                       stepped along the Y (slow) axis. Keep them disjoint:
--                       a setter in both lists is overwritten by the X sweep.
-- setXVoltageDomains:   {[setter.id] = {min=, max=}} for the X setters
-- setYVoltageDomains:   {[setter.id] = {min=, max=}} for the Y setters
local function Measure2DSweep(
   ctx,
   xSetters,
   setXVoltageDomains,
   numXSteps,
   ySetters,
   setYVoltageDomains,
   numYSteps,
   getters)

  local xSweep, ySweep = sweepMath.buildRasterSweep(
    xSetters, setXVoltageDomains, numXSteps,
    ySetters, setYVoltageDomains, numYSteps)

  -- data[j][i][g] = reading of getter g at X step i, Y step j
  for j, yVec in ipairs(ySweep) do
    applyVector(ySetters, yVec)            -- increment the Y vector

    -- OPTIONAL: utilize instrument's native sweep command to sweep the X vector in one shot, if supported.
    -- Otherwise, sweep the X vector point-by-point, reading all getters at each X step.
    local row = {}
    for i, xVec in ipairs(xSweep) do       -- full X sweep, min -> max
      applyVector(xSetters, xVec)
      readGetters(getters)
    end
  end
end

return { main = Measure2DSweep }
