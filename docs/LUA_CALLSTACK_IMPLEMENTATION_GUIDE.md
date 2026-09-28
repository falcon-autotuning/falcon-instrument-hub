# Replace Lua routing metadata with CallStack arguments

Use this guide to migrate one measurement at a time. It describes changes to implement, not a completed API.

The hub should choose **instrument, group, channel, and command**, then pass that information to Lua as a `CallStack`. Lua should coordinate the calls. Keep units, output shape, calibration, and capability validation in the measurement contract; CallStack does not contain them.

## Files to work in

| File | Change |
| --- | --- |
| [interpreter/router.go](../runtime/internal/interpreter/router.go) | Register each new measurement handler. |
| New `runtime/internal/callstack` package | Validate and serialize descriptors using the native library. |
| [instrumentserver/client.go](../runtime/internal/instrumentserver/client.go) | Encode scalar CallStack arguments and preserve their manifest type. |
| [runtime/scripts](../runtime/scripts) | Accept supplied stacks instead of constructing instrument targets from metadata. |
| [ISS CommandHandlers.cpp](../../instrument-script-server/src/daemon/CommandHandlers.cpp) and [RuntimeContext.cpp](../../instrument-script-server/src/daemon/RuntimeContext.cpp) | Verify userdata injection and validate the supplied group against the instrument API. |

## 1. Prepare the native library

The hub currently pins `instrument-call-stack v1.0.6`. Before using its serialized arguments:

- Fix empty-group round-trips: the current serializer accepts an empty group, but the deserializer rejects it.
- Define handling for delimiters and reject trailing fields, invalid channel values, and oversized identifiers. Native string fields hold at most 63 bytes before the terminator.
- Verify that a serialized descriptor retains all four fields after deserialization.

Use the [native serialization implementation](../../instrument-call-stack/src/instrument-call-stack.c) as the starting point. Rebuild and update the hub/ISS dependency pins or development overrides. Editing the sibling clone alone does not change their installed libraries.

## 2. Resolve the call in the measurement handler

Start with a single voltage getter.

1. Read the requested connection from the measurement request.
2. Resolve it using the wiremap and instrument API.
3. Validate the required capability, role, group, and channel.
4. Select the command from the API.
5. Record the expected output name and unit.

Ensure startup supplies the instrument API paths; the reviewed startup currently passes an empty list.

Create a descriptor containing the resolved instrument, group, channel, and command. Use `-1` for an absent channel only when the command supports an instrument-wide call.

A CallStack has no setters. For a workflow with several commands, create separate descriptors such as `setVoltage` and `readVoltage`.

## 3. Add a small Go serialization adapter

In the proposed `internal/callstack` package:

1. Accept a typed Go descriptor with those four fields.
2. Validate strings and channel range before converting them to C.
3. Call `instrument_call_stack_create`, then `instrument_call_stack_serialize`.
4. Copy the serialized string into Go memory.
5. Free the input C strings, serialized allocation, and native stack on every applicable path.

Use `pkg-config: instrument-call-stack`. The current serialized allocation uses `malloc`; document its matching free operation in the wrapper. Return errors for failed allocations or invalid input.

Return a Go string from this adapter. Convert it to `instrumentserver.CallStack` at the caller. Keep serialization in one place rather than assembling pipe-delimited strings in handlers.

## 4. Send a typed argument to ISS

In `toGrpcVariableValue`, add the missing scalar case:

```go
case CallStack:
    return &daemonv1.VariableValue{
        Value: &daemonv1.VariableValue_S{S: string(value)},
    }, nil
```

`LuaType()` already recognizes `CallStack`. Keep that type so `buildMeasureJobRequest` generates `LUA_TYPES_CALL_STACK`, rather than an ordinary string parameter.

After the adapter produces `serializedRead`, construct the argument:

```go
variables := []instrumentserver.MeasureVariable{
    {
        Name: "readVoltage",
        Value: instrumentserver.VariableValue{
            Value: instrumentserver.CallStack(serializedRead),
        },
    },
}
```

Pass these variables through the measurement dispatcher. Their slice order must match the Lua parameters after `ctx`: ISS uses the manifest order to call `main`.

For now, use separate scalar stack arguments. The reviewed ISS converts `CallStackArray` into strings, not an array of CallStack userdata.

## 5. Simplify the Lua script

Replace instrument IDs, channel fields, and hardcoded command selection with the supplied argument:

```lua
function main(ctx, readVoltage)
    ctx:call(readVoltage)
end
```

ISS deserializes the argument and supplies the userdata. For a setter, use a signature such as `main(ctx, setVoltage, voltage)`, then call `ctx:call(setVoltage, voltage)`. Supply numeric values using the client's canonical Go types, such as `float64` and `int64`.

In ISS, validate that the stack's group agrees with the command's API group before dispatch. Currently the command API determines the channel parameter, while the result reports the supplied stack group.

## 6. Select results and remove redundant metadata

In the handler, select the expected result using instrument, group, channel, command, and output name. Repeated calls also need an agreed invocation/order rule. Validate the output unit and shape.

ISS currently returns collected `ctx:call` results; returning an arbitrary Lua table does not automatically publish a derived measurement. Define a separate result path for averages, calibration, or other script-computed values.

Once this operation works, remove its `@falcon.metadata` block or routing YAML entry. Preserve its capability/role/request-source checks in the handler. Remove shared metadata-loading code only after all remaining consumers have migrated. Instrument API metadata and output units are still needed.

Register the handler in `NewRouter`; its production list is currently empty.

## Completion checks

- [ ] Native serialization round-trips populated and empty groups.
- [ ] A real encoded request reaches Lua as CallStack userdata.
- [ ] Argument order matches the Lua signature.
- [ ] An invalid capability, command, or group fails before instrument execution.
- [ ] The expected output is selected even when setup calls also return values.
- [ ] The measurement works without its old routing metadata.

For supporting findings, see the [refactor review](REFACTOR_REVIEW_2026-09-25.md). For returned arrays, follow the [buffer guide](DATABUFFER_COMMUNICATION_GUIDE.md).
