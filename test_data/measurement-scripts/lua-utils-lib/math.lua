-- set of math related utility functions for measurement scripts

-- Linearly spaced values from vmin to vmax (inclusive), nSteps points
local function linspace(vmin, vmax, nSteps)
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

-- Build a 1D sweep of setter voltages, one vector per step.
-- Each vector contains one voltage per setter, in setter order.
-- Each setter has a voltage domain, which is a 2-element array of [min,
local function build1DSweep(limits, nSteps)
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