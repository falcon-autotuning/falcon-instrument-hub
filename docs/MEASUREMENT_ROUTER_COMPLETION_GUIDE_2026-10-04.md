# Measurement Router Completion Review

Date: 2026-10-04  
Updated against hub revision: `569a68e`

## Scope

This review covers the current `falcon-instrument-hub` source plus the downloaded `falcon-core-main`, `falcon-core-libs-main`, and `instrument-script-server-main` source snapshots under `~/Downloads/tmp_falcon`. `falcon-routine` and the configuration generators were not available in that directory, so routing ownership and generated API policy still need confirmation there.

No tests were run for this review. The findings come from reading the current source and downloaded dependency snapshots. The hub still pins `falcon-core-libs/go/falcon-core v0.0.6`; downloaded `main` sources describe proposed/current upstream work and are not automatically the interfaces used by the hub build.

The task is to complete this successful path:

```text
MEASURE_COMMAND
    -> deserialize falcon-core MeasurementRequest
    -> choose one measurement handler
    -> resolve request ports to physical instrument targets
    -> invoke the selected Lua measurement through ISS
    -> convert attributed ISS results to MeasurementResponse
    -> publish the correlated MEASURE_RESPONSE
```

## Confirmed cross-repository contracts

### MeasurementRequest still has no dedicated route field

The downloaded `falcon-core` and `falcon-core-libs` sources confirm that `MeasurementRequest` contains:

- the inherited base-message `message` string;
- waveforms;
- getter ports;
- meter transforms;
- a time domain.

There is no `measurement_name`, script name, or route-key field. Therefore `Message()` is the only presently visible routing candidate, but its use as a stable handler identifier still needs confirmation from `falcon-routine`. Keep that decision in one `RouteKey()` adapter.

### ISS typed target behavior is now confirmed

The downloaded ISS protobuf defines scalar and array types for `InstrumentTarget` and `InstrumentDomain`. ISS deserializes these values according to the request `TypeManifest` before calling `main(ctx, ...)`.

The current Lua target interface uses methods, not fields:

```lua
target:get_instrument_name()
target:get_channel_group()
target:get_channel()
```

The ISS `target.lua` test constructs a CallStack from those three values. Hub scripts that use `getter.id` or `getter.channel` are stale against this contract.

### ISS helper loading requires an explicit module load

ISS reads `INSTRUMENT_SCRIPT_SERVER_OPT_LUA_LIB`. Directory entries are added to `package.preload`, but they are not executed automatically. A measurement script must call `require("source")` to obtain a generated source module. If the environment variable names a bundle file instead, ISS executes it and registers a returned table under the bundle filename stem.

### The current ISS API schema cannot fully define Falcon ports

The downloaded ISS `instrument_api.schema.json` requires top-level `io` and `commands`; `channel_groups` is optional. It does not define:

- Falcon instrument type;
- Falcon scope;
- Falcon access;
- Falcon instrument characteristic.

It also permits roles beyond the hub parser's current comment: `input`, `output`, `inout`, trigger roles, clock roles, and `setting`. The hub parser currently ignores top-level `io`, requires the non-schema field `instrument.instrument_type`, and reads only channel-group IO entries. This is a real schema mismatch, not a missing local mapping function.

Do not silently manufacture the missing Falcon attributes. Either extend the shared instrument API schema and generator, or introduce a separate hub-owned mapping configuration with explicit defaults and validation.

### The downloaded Falcon sources are internally transitional

The downloaded `falcon-core` `InstrumentPort.hpp` still declares the enum-based `Instrument`, while `Instrument.hpp` defines `Instrument` as `std::string`. The downloaded Go bindings still expose the enum values used by the hub. Treat the enum-to-string instrument migration as unfinished and keep the hub adapter compatible with its pinned binding until the upstream C/C++ and Go interfaces agree.

## Visible changes from the recent pull

### Port responses now use the richer Falcon Port model

Commit `69499c0` added settings to `PortPayload` and started passing the instrument name into Falcon port construction. The response now contains `knobs`, `meters`, and `settings` in [api.go](../runtime/internal/api/api.go).

The current port model in [connections.go](../runtime/internal/ports/connections.go) contains:

- instrument name, channel group, and channel;
- Falcon `Instrument` type;
- Falcon `PortType` for knob, meter, or setting;
- access mode;
- instrument characteristic;
- units and description.

Commit `5207bc8` embedded `PortEntry` inside `ConnectedPort`, reducing duplicated fields. `ResolveConnectedPort` now compares a Falcon `InstrumentPort` with the connected-port catalog using its device name, instrument name, instrument type, role, access, and characteristic.

This is useful for measurements: the request already carries Falcon `InstrumentPort` objects, so the measurement handler should resolve those objects directly instead of inferring a capability from metadata.

### Typed ISS inputs were added

The hub now has native serializers for:

- [InstrumentTarget](../runtime/internal/instrumenttarget/target.go), containing instrument, group, and channel;
- [InstrumentDomain](../runtime/internal/instrumentdomain/domain.go), containing minimum and maximum values.

[client.go](../runtime/internal/instrumentserver/client.go) maps these values and their array forms into the ISS type manifest. This is the intended replacement for passing an entire command from Go.

The older [callstack.go](../runtime/internal/callstack/callstack.go) is explicitly marked deprecated. A measurement handler should send `InstrumentTarget` values to Lua. The generated Teal API should create the command-specific CallStack.

### The outer measurement transport is mostly connected

[measure_command_handler.go](../runtime/internal/handlers/measure/measure_command_handler.go) already:

1. subscribes to `INSTRUMENTHUB.MEASURE_COMMAND`;
2. deserializes the NATS envelope;
3. creates a Falcon request from `cmd.Request`;
4. calls the router;
5. serializes the returned Falcon response;
6. publishes measurement data and the correlated response envelope.

The dispatcher in [dispatcher.go](../runtime/internal/interpreter/dispatcher.go) already resolves a script name to `<scriptsPath>/<name>.lua`, submits typed variables to ISS, and registers returned DataBuffers.

The missing work is primarily between request deserialization and dispatcher invocation.

## Current blockers

### 1. Connected-port construction has started but is not complete

`DefaultConnectedPortsBuilder.NewConnectedPorts` now parses the API files and calls `ConnectWireMap`. Three concrete gaps remain:

- `apis` is parsed but never used to populate `PortLibrary`;
- the empty library produces no connections;
- `ports.newConnectedPorts` is unexported and cannot be called from the `handlers` package.

`ConnectWireMap` returns `[]ConnectedPort`, while the builder interface returns `*ConnectedPorts`. Export the partitioning constructor, for example `NewConnectedPortsFromConnections`, or expose one higher-level function in `ports` that performs parse, library construction, wire-map joining, and partitioning.

The library builder must create one entry per addressable instrument/group/channel/IO capability. Its key must include the channel and capability so expanded channel groups do not overwrite one another.

This cannot yet be completed faithfully from the latest ISS YAML schema alone. Decide where Falcon-only port attributes live before filling the library with zero enum values.

### 2. Setting serialization is incomplete

`serializePortsToCerealJSON` currently constructs a knob when `IsKnob()` is true and constructs a meter for everything else. A setting is therefore serialized as a meter.

The pinned Falcon binding provides `instrumentport.NewSetting`, as well as the general `NewPort` constructor. The serializer should have explicit knob, meter, and setting branches. A setting must preserve its scope, access, characteristic, instrument name, instrument type, connection, units, and description.

The current instrument API parser does not populate all of those fields. The upcoming configuration contract must define their source or define approved defaults.

`PortEntry` also lacks `Scope`, so even a correctly recognized setting cannot currently be serialized through `NewSetting` without losing data. Top-level/global settings need a separate connection rule because they may not correspond to a channel gate in the wire map.

### 3. The active measurement implementation was moved across a Go package boundary

The files under `runtime/internal/interpreter/commands` still declare `package interpreter`. Go packages cannot span directories: that directory is a separate import path from `runtime/internal/interpreter` even though the package clause has the same name.

As a result, the moved files cannot use the parent directory's `FalconMeasurementRequest`, `MeasurementDispatcher`, or `FalconMeasurementResponse` types without importing a shared package. The parent interpreter package also lost the getter/setter extraction and response helpers that the handler expects.

For the smallest repair, move the active files back beside `router.go`:

- the live `get_voltage` handler;
- the minimal request-port extraction helpers it needs;
- the measurement response builder;
- the connection decoding helper.

Do not move the large deprecated handler back. Once one measurement works, package boundaries can be redesigned around shared request/response interfaces if separation is still desired.

### 4. The router registers no handlers

`NewRouter` currently contains only a commented `getVoltageHandler`. Every request therefore reaches `no measurement handler matched request`.

The handler should only be registered after its package location and dependencies are repaired.

### 5. The routing discriminator is not settled

Both the pinned binding and downloaded upstream sources expose `Message()`, getters, waveforms, meter transforms, and a time domain. Neither source exposes the previous `MeasurementName()` method or a replacement route field.

The router needs an explicit, stable key such as `get_voltage`. Confirm with `falcon-routine` whether `MeasurementRequest.message` is now the canonical script/handler identifier. If it is not, an upstream request-field change is required.

Do not infer the handler from its ports. Several measurements can use the same ports but perform different procedures.

Put the chosen rule behind one method, for example `RouteKey()`, so an upstream contract change affects one adapter rather than every handler.

### 6. The moved get-voltage handler is stale

The handler in `internal/interpreter/commands` currently:

- returns true for every non-nil request;
- calls `ResolveConnectedPort` with arguments that no longer match its signature;
- refers to removed fields such as `ChannelName` and `ChannelIndex` instead of `ChannelGroup` and `Channel`;
- creates a deprecated Go-side CallStack;
- expects extraction and response types that now live in a different directory package.

Treat this file as a draft and update it to the current `InstrumentPort -> ConnectedPort -> InstrumentTarget` design.

### 7. The active Lua contract is stale against current ISS

The current [get_voltage.lua](../runtime/scripts/lua/get_voltage.lua) accepts a CallStack built by Go. The older [get_voltage_old.lua](../runtime/scripts/lua/get_voltage_old.lua) accepts a target, but reads obsolete fields such as `getter.id`.

The downloaded ISS source confirms that a target must be read through `get_instrument_name()`, `get_channel_group()`, and `get_channel()`. The generated [source.tl](../runtime/scripts/teal/source.tl) maps `getVoltage` to `GET_VOLTAGE` and constructs the CallStack, which preserves the intended ownership: Go supplies the endpoint and generated Lua owns the command.

The generated source module must also be loaded explicitly. When its compiled Lua is supplied through an ISS helper directory, use `local source = require("source")` and call `source:getVoltage(...)`.

### 8. The new Lua directory must be the configured script path

The dispatcher appends `<measurement>.lua` directly to the configured scripts directory. The `--user-measurement-luas` value must therefore point to `runtime/scripts/lua`, not `runtime/scripts`.

## Recommended implementation order

### Step 1: Write down the cross-repository contract

Two earlier questions are now answered: current ISS target accessors are known, and helper directories require `require(...)`. Confirm the remaining items before spreading assumptions through handlers:

1. Does `falcon-routine` put `get_voltage` in `MeasurementRequest.message`?
2. Which shared configuration owns Falcon scope, access, characteristic, and instrument type?
3. Does an API role describe flow relative to Falcon or relative to the instrument command?
4. How are global settings represented when they have no channel gate in the wire map?
5. Which synchronized Falcon Core release changes instrument type from the current enum to a string?

The role question is essential. Existing hub source fixtures label settable voltage as `output` and measured voltage as `input`. The downloaded ISS example labels voltage-to-set as `input` and measured voltage as `output`. Do not hard-code an input/output-to-knob/meter map until the generator and schema agree.

Keep temporary compatibility logic inside adapters. Do not put fallback guesses into every handler.

### Step 2: Restore connected-port construction

Implement the real builder behind `ConnectedPortsBuilder`:

```go
func (DefaultConnectedPortsBuilder) NewConnectedPorts(
    apiPaths []string,
    wiremap *config.WireMap,
) (*ports.ConnectedPorts, error) {
    apis, err := ports.ParseInstrumentAPIs(apiPaths)
    if err != nil {
        return nil, err
    }

    library, err := ports.BuildPortLibrary(apis)
    if err != nil {
        return nil, err
    }

    connected, err := ports.ConnectWireMap(wiremap, library)
    if err != nil {
        return nil, err
    }

    return ports.NewConnectedPortsFromConnections(connected), nil
}
```

The exact function names can differ. The required behavior is:

- parse the instrument configuration/API inputs;
- map configuration strings to Falcon enums in one place;
- create a unique `PortEntry` for every addressable channel and port;
- join those entries with the wire map;
- partition the result into knobs, meters, and settings;
- reject duplicate or ambiguous identities.

The current `PortLibrary` is a map. If entries are expanded per channel, its key must include the channel or the representation must change to avoid overwriting channels that share an IO name.

A suitable identity is:

```text
<instrument identifier>.<channel group>.<channel>.<IO capability>
```

Keep the IO capability in `PortEntry` as well as the map key. Add `Scope` if settings remain represented by `PortEntry`. For each channel-group IO entry, the library builder should copy the API identifier, group, channel, capability, unit, and description, then obtain the Falcon-only fields through the agreed adapter.

The builder must also handle top-level `io`. Channel-group IO can be expanded through `channel_parameter.min..max`; global IO cannot be forced through that loop.

The current manager draft should not call `ports.newConnectedPorts`: lowercase identifiers are private to the `ports` package. Export a narrow constructor rather than exporting internal slices.

Also settle port naming before measurements use the resolver. `PortRequestHandler` currently serializes `PortName` as the Falcon port's `default_name`, while `ResolveConnectedPort` compares `default_name` to `ConnectedPort.DeviceName` from the wire map. Those identities must be made the same or compared through separate, documented fields.

Then replace `mockConnectedPortsBuilder` in `ProductionDependancies` with `DefaultConnectedPortsBuilder`. Keep mock builders inside tests.

### Step 3: Finish PortRequest serialization

Update the port serializer to branch explicitly:

```go
switch {
case cp.IsKnob():
    // NewKnob or NewPort with the approved attributes
case cp.IsMeter():
    // NewMeter or NewPort with the approved attributes
case cp.IsSetting():
    // NewSetting with scope, access, and characteristic
default:
    return "", fmt.Errorf("unsupported port type for %s", cp.PortName)
}
```

This matters to measurements because a `MeasurementRequest` will return those same ports. Resolver equality only works when the response and catalog use the same identity fields.

### Step 4: Restore a valid interpreter package

For the first working vertical slice, keep the router, request adapter, response builder, and `get_voltage` handler in `runtime/internal/interpreter`.

Keep the parent package small by restoring only these operations:

- read the route key;
- obtain request getter handles;
- obtain setter handles when later needed;
- build a Falcon MeasurementResponse from attributed results;
- decode a request connection only if the direct `InstrumentPort` resolver cannot be used.

Prefer resolving the actual request `InstrumentPort` while its handle is alive:

```go
getters, err := req.Handle().Getters()
if err != nil {
    return nil, err
}
defer getters.Close()

if size, err := getters.Size(); err != nil || size != 1 {
    // return a useful validation error
}

getter, err := getters.At(0)
if err != nil {
    return nil, err
}
defer getter.Close()

connectedPort, err := connected.ResolveConnectedPort(getter)
```

This avoids converting the port to partial metadata and then trying to reconstruct its identity.

### Step 5: Add one exact route

Implement `CanHandle` with the approved route key:

```go
func (*getVoltageHandler) CanHandle(req *FalconMeasurementRequest) (bool, error) {
    name, err := req.RouteKey()
    if err != nil {
        return false, err
    }
    return name == "get_voltage", nil
}
```

Register only this handler initially:

```go
handlers: []MeasurementHandler{
    &getVoltageHandler{},
},
```

Reject missing or unknown route keys clearly. Do not let the first handler claim all requests.

### Step 6: Pass an InstrumentTarget to Lua

After resolving the request getter, serialize the endpoint:

```go
serialized, err := (instrumenttarget.Target{
    Instrument: connectedPort.InstrumentName,
    Group:      connectedPort.ChannelGroup,
    Channel:    connectedPort.Channel,
}).Serialize()
if err != nil {
    return nil, err
}

results := dispatcher.RunAll([]MeasurementRequest{{
    Script: "get_voltage",
    Variables: []instrumentserver.MeasureVariable{{
        Name: "getter",
        Value: instrumentserver.VariableValue{
            Value: instrumentserver.InstrumentTarget(serialized),
        },
    }},
}})
```

The handler should not choose `GET_VOLTAGE` or construct a CallStack. The generated `Mock1Source1:getVoltage` wrapper owns that command mapping.

Make the active `get_voltage.lua` use the confirmed target API and explicitly load the generated module:

```lua
local source = require("source")

function main(ctx, getter)
    return source:getVoltage(
        getter:get_instrument_name(),
        getter:get_channel()
    )
end
```

The target also exposes `get_channel_group()`. The current generated `source.tl` wrapper drops that value and creates a CallStack with only instrument, command, and channel. Confirm that `teal-gen-api` writes the command's configured channel group into the CallStack, especially for instruments with more than one channel group. Fix that in the generator or Teal source, not in each measurement script.

### Step 7: Validate and attribute ISS results

For `get_voltage`, require:

- exactly one dispatched measurement result;
- no dispatcher error;
- exactly one relevant instrument call;
- the expected instrument and channel;
- the expected command/output according to the approved generated API contract;
- one finite numeric value;
- the expected unit or a documented unit conversion.

Use result identity, rather than list position alone, before attaching data to a request port. For future multi-getter and buffered handlers, carry this association through each call and DataBuffer.

### Step 8: Build and publish the Falcon response

Construct the acquisition context from the original request port when possible. This preserves the richer Port attributes added by the recent Falcon changes.

Then return a `FalconMeasurementResponse` to `measure_command_handler.go`. That outer handler already serializes and publishes the successful response.

The current code logs and returns on parse, routing, ISS, and response-building errors. If the cross-repository contract requires every MeasurementRequest to receive a response, add a correlated failure response rather than silently ending after a log message. The failure envelope must be agreed with `falcon-routine` before inventing a local format.

## Suggested first completion boundary

Call the task complete for the first vertical slice when the code visibly supports this sequence:

1. Production constructs a non-nil connected-port catalog.
2. A PortRequest returns the same getter identity later carried by MeasurementRequest.
3. The router reads an explicit `get_voltage` route key.
4. The handler resolves exactly one request getter to exactly one connected port.
5. The handler passes a typed `InstrumentTarget` to `runtime/scripts/lua/get_voltage.lua`.
6. The script loads the generated source module and reads the target through its getter methods.
7. Generated Lua selects `GET_VOLTAGE`, preserves the channel group, and ISS executes it.
8. The handler selects the attributed numeric result.
9. The handler constructs a Falcon MeasurementResponse using the original getter port.
10. The NATS layer publishes the correlated response.

After that path is stable, add handlers in increasing complexity:

1. one scalar setter;
2. one setting getter/setter;
3. multiple scalar targets using `InstrumentTargetArray`;
4. buffered acquisition using DataBuffer ownership;
5. one-dimensional and two-dimensional waveform measurements using `InstrumentDomain` values.

## Files to change first

| File | First change |
|---|---|
| `runtime/internal/handlers/manager.go` | Implement the real connected-port builder. |
| `runtime/cmd/dependancies.go` | Use the default builder in production. |
| `runtime/internal/ports/api.go` | Parse or adapt the approved new port configuration fields. |
| `runtime/internal/ports/connections.go` | Restore construction APIs and enforce unique port identities. |
| `runtime/internal/handlers/port_request_handler.go` | Serialize settings with the correct Falcon constructor. |
| `runtime/internal/interpreter/falcon_core.go` | Restore the minimal request extraction and route-key adapter. |
| `runtime/internal/interpreter/router.go` | Register one exact handler. |
| `runtime/internal/interpreter/commands/get_voltage_handler.go` | Move/rewrite it against `InstrumentTarget` and current port fields. |
| `runtime/internal/interpreter/commands/measure_command_handler_response.go` | Move the active response builder back to a usable package. |
| `runtime/scripts/lua/get_voltage.lua` | Use the generated target-based API contract. |
| `runtime/scripts/teal/source.tl` or `teal-gen-api` | Change only if its generated target/CallStack contract differs from ISS. |

## Avoid during this task

- Do not restore measurement metadata as the routing source.
- Do not infer a measurement from port capability.
- Do not make Go choose Lua command verbs.
- Do not migrate all measurement scripts at once.
- Do not copy the deprecated monolithic measurement handler back into service.
- Do not hard-code assumptions from unmerged external repositories throughout the hub; isolate them in adapters.
