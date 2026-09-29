-- Auto-generated Teal module from instrument YAML (vendor=Mock model=1 id=Source1)

global instrument_call_stack: any

Mock1Source1 = {}

--- Set output voltage
--- @param id string Instrument instance identifier
--- @param channel number Voltage source channel number (1-32)
--- @param voltage any
--- @return any
function Mock1Source1:setVoltage(id: string, channel: number, voltage: any): any
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "SET_VOLTAGE",
    channel = channel,
  })
  return context:call(cs, voltage)
end

--- Measure DC voltage
--- @param id string Instrument instance identifier
--- @param channel number Voltage source channel number (1-32)
--- @return number measured_voltage
function Mock1Source1:getVoltage(id: string, channel: number): number
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "GET_VOLTAGE",
    channel = channel,
  })
  return context:call(cs)
end

--- resets the mock instrument
--- @param id string Instrument instance identifier
--- @return any
function Mock1Source1:reset(id: string): any
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "RESET",
  })
  return context:call(cs)
end

return Mock1Source1
