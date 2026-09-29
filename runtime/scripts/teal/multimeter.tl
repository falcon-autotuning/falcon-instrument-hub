-- Auto-generated Teal module from instrument YAML (vendor=Mock model=5 id=Meter1)

global instrument_call_stack: any

Mock5Meter1 = {}

--- Set the sample rate
--- @param id string Instrument instance identifier
--- @param channel number Voltage meter channel number (1-8)
--- @param sample_rate any
--- @return any
function Mock5Meter1:setSampleRate(id: string, channel: number, sample_rate: any): any
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "SET_SAMPLE_RATE",
    channel = channel,
  })
  return context:call(cs, sample_rate)
end

--- Set the number of bins to average for each data point
--- @param id string Instrument instance identifier
--- @param channel number Voltage meter channel number (1-8)
--- @param bins any
--- @return any
function Mock5Meter1:setBins(id: string, channel: number, bins: any): any
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "SET_BINS",
    channel = channel,
  })
  return context:call(cs, bins)
end

--- Set the slope to adjust output reading sensitivity to input
--- @param id string Instrument instance identifier
--- @param channel number Voltage meter channel number (1-8)
--- @param slope any
--- @return any
function Mock5Meter1:setSlope(id: string, channel: number, slope: any): any
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "SET_SLOPE",
    channel = channel,
  })
  return context:call(cs, slope)
end

--- Query slope for channel
--- @param id string Instrument instance identifier
--- @param channel number Voltage meter channel number (1-8)
--- @return number slope
function Mock5Meter1:getSlope(id: string, channel: number): number
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "GET_SLOPE",
    channel = channel,
  })
  return context:call(cs)
end

--- Set trigger level for channel
--- @param id string Instrument instance identifier
--- @param channel number Voltage meter channel number (1-8)
--- @param trigger_level any
--- @return any
function Mock5Meter1:setTriggerLevel(id: string, channel: number, trigger_level: any): any
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "SET_TRIGGER_LEVEL",
    channel = channel,
  })
  return context:call(cs, trigger_level)
end

--- Set output voltage
--- @param id string Instrument instance identifier
--- @param channel number Voltage meter channel number (1-8)
--- @return MeasureResponse stream
function Mock5Meter1:measureStream(id: string, channel: number): MeasureResponse
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "MEASURE_STREAM",
    channel = channel,
  })
  return context:call(cs)
end

--- Measure a single data point
--- @param id string Instrument instance identifier
--- @param channel number Voltage meter channel number (1-8)
--- @return number voltage
function Mock5Meter1:getDatapoint(id: string, channel: number): number
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "GET_DATAPOINT",
    channel = channel,
  })
  return context:call(cs)
end

--- Query trigger level for channel
--- @param id string Instrument instance identifier
--- @param channel number Voltage meter channel number (1-8)
--- @return number trigger_level
function Mock5Meter1:getTriggerLevel(id: string, channel: number): number
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "GET_TRIGGER_LEVEL",
    channel = channel,
  })
  return context:call(cs)
end

--- resets the mock instrument
--- @param id string Instrument instance identifier
--- @return any
function Mock5Meter1:reset(id: string): any
  local cs = instrument_call_stack.new({
    instrument = id,
    command = "RESET",
  })
  return context:call(cs)
end

return Mock5Meter1
