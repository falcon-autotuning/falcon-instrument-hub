



Mock5Meter1 = {}






function Mock5Meter1:setSampleRate(id, channel_group, channel, sample_rate)
   local cs = instrument_call_stack.new({
      instrument = id,
      group = channel_group,
      command = "SET_SAMPLE_RATE",
      channel = channel,
   })
   return context:call(cs, sample_rate)
end






function Mock5Meter1:setBins(id, channel_group, channel, bins)
   local cs = instrument_call_stack.new({
      instrument = id,
      group = channel_group,
      command = "SET_BINS",
      channel = channel,
   })
   return context:call(cs, bins)
end






function Mock5Meter1:setSlope(id, channel_group, channel, slope)
   local cs = instrument_call_stack.new({
      instrument = id,
      group = channel_group,
      command = "SET_SLOPE",
      channel = channel,
   })
   return context:call(cs, slope)
end





function Mock5Meter1:getSlope(id, channel_group, channel)
   local cs = instrument_call_stack.new({
      instrument = id,
      group = channel_group,
      command = "GET_SLOPE",
      channel = channel,
   })
   return context:call(cs)
end






function Mock5Meter1:setTriggerLevel(id, channel_group, channel, trigger_level)
   local cs = instrument_call_stack.new({
      instrument = id,
      group = channel_group,
      command = "SET_TRIGGER_LEVEL",
      channel = channel,
   })
   return context:call(cs, trigger_level)
end





function Mock5Meter1:measureStream(id, channel_group, channel)
   local cs = instrument_call_stack.new({
      instrument = id,
      group = channel_group,
      command = "MEASURE_STREAM",
      channel = channel,
   })
   return context:call(cs)
end





function Mock5Meter1:getDatapoint(id, channel_group, channel)
   local cs = instrument_call_stack.new({
      instrument = id,
      group = channel_group,
      command = "GET_DATAPOINT",
      channel = channel,
   })
   return context:call(cs)
end





function Mock5Meter1:getTriggerLevel(id, channel_group, channel)
   local cs = instrument_call_stack.new({
      instrument = id,
      group = channel_group,
      command = "GET_TRIGGER_LEVEL",
      channel = channel,
   })
   return context:call(cs)
end




function Mock5Meter1:reset(id)
   local cs = instrument_call_stack.new({
      instrument = id,
      command = "RESET",
   })
   return context:call(cs)
end

return Mock5Meter1
