-- Math utilities for measurement scripts.
-- NOTE: don't name this file math.lua. require("math") returns Lua's built-in
-- math library (already in package.loaded), so your module would never load.

local M = {}

-- Linearly spaced values from vmin to vmax (inclusive), nSteps points
function M.linspace(vmin, vmax, nSteps)
  local v = {}
  if nSteps == 1 then
    v[1] = vmin
    return v
  end
  local dv = (vmax - vmin) / (nSteps - 1)
  for i = 1, nSteps do
    v[i] = vmin + (i - 1) * dv
  end
  return v
end

-- 1D sweep along a voltage vector: one vector per step, one voltage per setter.
-- limits[k] = {min, max} for setter k. All setters move together, each from
-- its own min to its own max.
function M.build1DSweep(limits, nSteps)
  local sweep = {}
  for i = 1, nSteps do
    local t = (nSteps == 1) and 0 or (i - 1) / (nSteps - 1)
    local vec = {}
    for k, lim in ipairs(limits) do
      vec[k] = lim[1] + t * (lim[2] - lim[1])
    end
    sweep[i] = vec
  end
  return sweep
end

-- Convert voltage domains to the {min, max} arrays build1DSweep expects,
-- in the same order as `setters`.
-- Domains are keyed by setter.id with {min=, max=} fields, matching how
-- resolveEndVoltage reads them in measure_2D_buffered.lua. A setter with no
-- domain is held at 0 V.
function M.limitsFromDomains(setters, domains)
  local limits = {}
  for k, setter in ipairs(setters) do
    local d = domains[setter.id]
    if d then
      limits[k] = { d.min, d.max }
    else
      limits[k] = { 0.0, 0.0 }
    end
  end
  return limits
end

-- Raster sweep: the Y vector steps once per row, and the X vector is swept in
-- full along each row.
-- Returns xSweep[i] (voltages for the X setters) and ySweep[j] (voltages for
-- the Y setters). The full grid is every (ySweep[j], xSweep[i]) pair, which
-- avoids storing numXSteps * numYSteps vectors.
function M.buildRasterSweep(xSetters, xDomains, numXSteps,
                            ySetters, yDomains, numYSteps)
  local xSweep = M.build1DSweep(M.limitsFromDomains(xSetters, xDomains), numXSteps)
  local ySweep = M.build1DSweep(M.limitsFromDomains(ySetters, yDomains), numYSteps)
  return xSweep, ySweep
end

return M
