# Register one `set_voltage` measurement handler

Implement one narrow path:

```text
Falcon request → validate one setter/value → resolve connected port
→ create CallStack → run set_voltage.lua → validate ISS completion
→ create response
```

Do not copy the old generic dispatch branches. Reject input that is not part of this contract.

## Prerequisites

Complete the scalar CallStack encoder described in [LUA_CALLSTACK_IMPLEMENTATION_GUIDE.md](LUA_CALLSTACK_IMPLEMENTATION_GUIDE.md):

- Serialize `(instrument, group, channel, command)` through one Go/native adapter.
- Add scalar `instrumentserver.CallStack` handling to `toGrpcVariableValue`.
- Preserve `LUA_TYPES_CALL_STACK` in the request manifest.

Also fix production composition so it supplies a usable ISS client and instrument API paths. Otherwise the handler can pass unit tests but cannot execute in the hub.

## 1. Add the handler

Initially create `runtime/internal/interpreter/set_voltage_handler.go` with build tag `cgo && falcon_core` and package `interpreter`.

This location avoids a Go import cycle: `MeasurementHandler` uses `interpreter` types, so `interpreter.NewRouter` cannot import a separate package that imports `interpreter`. If handlers are moved later, make `NewRouter` accept `handlers ...MeasurementHandler` and construct them in the composition layer.

Implement:

```go
type setVoltageHandler struct{}

func (*setVoltageHandler) Name() string { return "set_voltage" }

func (*setVoltageHandler) CanHandle(req *FalconMeasurementRequest) (bool, error) {
    name, err := req.MeasurementName()
    if err != nil {
        return false, err
    }
    return strings.TrimSpace(name) == "set_voltage", nil
}
```

Its `Handle` method should perform the following steps and return immediately on any failure.

## 2. Validate and resolve the request

1. Call `req.ExtractSetters()` and require exactly one setter.
2. Get its physical gate with `gateNameFromConnectionJSON(setter.ConnectionJSON)`.
3. Resolve it with:

   ```go
   connected.ResolveConnectedPort(gateName, "voltage", "output")
   ```

4. Extract waveform zero using `ExtractWaveformDataFromRequestByIndex(req, 0)`.
5. Require exactly one scalar value in `RawTimeTrace`, and require it to be finite.
6. Validate the value against approved device/API bounds. Do not accept the waveform parser's placeholder zero as a real command.

The resolved port supplies:

- `InstrumentName`
- `ChannelName` for the CallStack group
- `ChannelIndex`
- `PortJSON`/connection information from the request for the response

Use command `SET_VOLTAGE` only after confirming that the resolved instrument API declares it. A capability named `voltage` alone does not prove the command name.

## 3. Dispatch one typed Lua call

Serialize this descriptor:

```text
instrument = resolved.InstrumentName
group      = resolved.ChannelName
channel    = resolved.ChannelIndex
command    = SET_VOLTAGE
```

Dispatch exactly one request:

```go
results := dispatcher.RunAll([]MeasurementRequest{{
    Script: "set_voltage",
    Variables: []instrumentserver.MeasureVariable{
        {
            Name: "setter",
            Value: instrumentserver.VariableValue{
                Value: instrumentserver.CallStack(serializedStack),
            },
        },
        {
            Name: "voltage",
            Value: instrumentserver.VariableValue{Value: voltage},
        },
    },
}})
```

Require one `MeasurementResult`, no error, and an ISS call result matching the expected instrument, group, channel, and `SET_VOLTAGE` command. Do not update cached state before this succeeds.

Update `runtime/scripts/set_voltage.lua` to accept the typed arguments:

```lua
function main(ctx, setter, voltage)
    ctx:call(setter, voltage)
end
```

Use the positional voltage argument. In the reviewed ISS implementation, the CallStack channel is injected for positional calls; table-style arguments currently still require the channel field.

## 4. Build the response

Decide and document what a successful setter response means:

- **Acknowledged setpoint:** report the requested voltage only after ISS confirms completion.
- **Verified value:** perform an explicit readback and report that observation.

Do not describe an echoed request as measured data. If the controller requires the current `MeasurementResponse` format, get approval for representing an acknowledged setpoint before using `buildMeasurementResponseJSONForTargets` with `[]float64{voltage}`.

Create the final `FalconMeasurementResponse` from the validated JSON and return it. The outer NATS handler already owns and closes that response.

## 5. Register it

In `NewRouter`, replace the empty list with:

```go
handlers: []MeasurementHandler{
    &setVoltageHandler{},
},
```

Keep the matcher specific. Unknown measurement names must continue to return `no measurement handler matched request`.

## Tests before moving to another measurement

- `CanHandle` accepts only trimmed `set_voltage`.
- Zero or multiple setters are rejected.
- Missing/ambiguous voltage-output ports are rejected.
- Malformed or non-scalar waveform values are rejected.
- The dispatcher receives one typed CallStack and one `float64` voltage.
- The serialized stack contains the resolved instrument, group, channel, and command.
- ISS failure and mismatched result identity return errors.
- Success produces the approved acknowledgement or readback response.
- A router created by `NewRouter` actually selects this handler.

After repairing the currently stale interpreter tests, run:

```sh
cd runtime
go test -count=1 -tags cgo,falcon_core ./internal/interpreter
go test -count=1 -tags cgo,falcon_core ./internal/handlers/measure
```
