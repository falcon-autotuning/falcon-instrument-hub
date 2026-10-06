# Measurement Router Completion Review

Original review: 2026-10-04

Source review updated: 2026-10-05

Updated against hub revision: `abe4166` (previous review: `569a68e`)

## Scope

This review now uses the local Git checkouts under `/home/zach/Documents/github/FALCon`, replacing the earlier downloaded snapshots. The primary review covers `falcon-core`, `falcon-core-libs`, `falcon-routine`, `falcon-instrument-hub`, and `instrument-script-server`; supporting target bindings and downstream controller pins were also inspected. The revision inventory and mapping to the original refactor checklist appear under [Repo observations](#repo-observations-2026-10-05).

This is a source review, not certification that every refactor or integration test passes. An offline, compile-only Go check was attempted with Go 1.25.2 and the hub's configured pkg-config directory. It failed because `falcon-core-c-api.pc` is unavailable there; no runtime tests were executed. The hub still selects Go bindings `v0.0.6` and native core `1.2.17`, while the sibling core and binding checkouts target `1.2.18`. No dependency pins or implementation files were changed for this review.

The implementation steps below remain recommendations. They describe future work, while the observations distinguish source already present from outstanding work.

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

The cloned `falcon-core` and `falcon-core-libs` sources confirm that `MeasurementRequest` contains:

- the inherited base-message `message` string;
- waveforms;
- getter ports;
- meter transforms;
- a time domain.

There is no `measurement_name`, script name, or route-key field. `Message()` remains a possible future routing carrier, but **it is not an established route identifier in the inspected routine producer**: [`ramp()`](../../falcon-routine/src/hub.cpp) constructs a request with `"Performing a ramp measurement"`, and `request_measurement()` forwards the supplied request unchanged. An exact `get_voltage`/`ramp` dispatch convention therefore needs a coordinated producer change or a dedicated shared request field. Keep the eventual decision in one `RouteKey()` adapter; do not treat a hub-only adapter as completion of that contract.

### ISS typed target behavior is now confirmed

The cloned ISS protobuf defines scalar and array types for `InstrumentTarget` and `InstrumentDomain`. ISS deserializes these values according to the request `TypeManifest` before calling `main(ctx, ...)`.

The current Lua target interface uses methods, not fields:

```lua
target:get_instrument_name()
target:get_channel_group()
target:get_channel()
```

The [`instrument-target` Lua binding](../../instrument-target/src/instrument-target-lua.c) checks userdata in argument 1, so use colon calls as above (or pass the receiver explicitly). The ISS `target.lua` fixture attempts to construct a CallStack from those three values but uses dot calls without a receiver; that fixture is not proof of working accessor invocation. Hub scripts that use `getter.id` or `getter.channel` are stale against this contract.

### ISS helper loading requires an explicit module load

ISS reads `INSTRUMENT_SCRIPT_SERVER_OPT_LUA_LIB`. Directory entries are added to `package.preload`, but they are not executed automatically. A measurement script must call `require("source")` to obtain a generated source module. If the environment variable names a bundle file instead, ISS executes it and registers a returned table under the bundle filename stem.

### The current ISS API schema cannot fully define Falcon ports

The cloned ISS [`instrument_api.schema.json`](../../instrument-script-server/schemas/instrument_api.schema.json) requires top-level `io` and `commands`; `channel_groups` is optional. It does not define:

- Falcon instrument type;
- Falcon scope;
- Falcon access;
- Falcon instrument characteristic.

It also permits roles beyond the hub parser's current comment: `input`, `output`, `inout`, trigger roles, clock roles, and `setting`. The hub parser currently ignores top-level `io`, requires the non-schema field `instrument.instrument_type`, and reads only channel-group IO entries. This is a real schema mismatch, not a missing local mapping function. In addition, the schema declares `instrument.model` as a string while the hub uses an `int`, and channel-group IO identifies a capability with required `suffix` while the new hub builder requires `IoType.Name`. The schema sets `instrument.additionalProperties: false`, so adding `instrument_type` directly to an otherwise valid ISS API is not compatible with that schema.

Do not silently manufacture the missing Falcon attributes. Either extend the shared instrument API schema and generator, or introduce a separate hub-owned mapping configuration with explicit defaults and validation.

### The active core/C/Go port contracts use enums and are aligned in the clones

The current [`InstrumentPort.hpp`](../../falcon-core/include/falcon-core/instrument_interfaces/names/InstrumentPort.hpp), [`InstrumentPort_c_api.h`](../../falcon-core/include/falcon-core/instrument_interfaces/names/InstrumentPort_c_api.h), and [Go instrument binding](../../falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument/instrument.go) all use an instrument-type enum. Concrete `instrument_name` is a separate string. Scope, access, port type, and the expanded instrument characteristics are represented across these interfaces.

A leftover [`Instrument.hpp`](../../falcon-core/include/falcon-core/instrument_interfaces/names/Instrument.hpp) still aliases `Instrument` to `std::string`, but no include of that header was found in core's `include` or `src` trees. This is a header-cleanup concern, not evidence that consumers should implement an enum/string compatibility shim. The earlier migration conclusion is superseded by this inspection.

Core provides the vocabulary for instrument categories and characteristics; its generic `InstrumentPort` constructor checks non-null units but does not enforce a per-instrument characteristic allowlist. A real-instrument configuration and validator still need to connect that vocabulary to actual hardware, scope, access, units, and supported operations.

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

### Connected-port construction is now implemented and selected in production

Commit `e1195aa` added `BuildPortLibrary` and the exported `NewConnectedPortsFromConnections`; `abe4166` switched `ProductionDependancies` to `DefaultConnectedPortsBuilder`. The builder now parses APIs, expands channel-group IO into unique `<instrument>.<group>.<channel>.<IO name>` keys, joins the wire map, and partitions knobs/meters/settings. Duplicate library keys are rejected, and source tests cover the new builder and partitioning.

This closes the previous empty-library and unexported-constructor blockers. The remaining limitations are configuration coverage and identity correctness, described next.

## Current blockers

### 1. Connected-port configuration and identity remain incomplete

The construction pipeline exists, but it only handles the hub's channel-group API dialect:

- Top-level/global IO is not parsed or built, and `PortEntry` has no `Scope` field or separate IO-capability field.
- `portAttributesFromRole` hard-codes `output -> Knob/Write`, `input -> Meter/Read`, and `setting -> Setting/ReadWrite`; every characteristic is assigned `None`. These defaults do not establish the intended settings policy.
- `ConnectWireMap` silently leaves unmatched wire-map entries out of the result; its error slice is never populated. Unique library keys alone do not validate all physical mappings.
- The serializer puts `ConnectedPort.PortName` in Falcon `default_name`, but `ResolveConnectedPort` compares that value with `ConnectedPort.DeviceName`. A returned port such as `Source1.analog.4.voltage` will not round-trip against device name `P1`.
- Resolution does not compare the request's connection, scope, units, or a separate IO capability. Distinct capabilities can remain ambiguous when their compared attributes coincide.

Finish the shared configuration and round-trip identity rules before treating a non-empty catalog as a working measurement path. The parser/schema differences above also mean valid upstream ISS APIs are not automatically valid hub inputs.

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

Both the cached pinned binding (`v0.0.6`) and cloned upstream sources expose `Message()`, getters, waveforms, meter transforms, and a time domain. Neither source exposes the previous `MeasurementName()` method or a replacement route field.

The router needs an explicit, stable key such as `get_voltage`. Routine currently emits descriptive text for ramps, so agree and implement a message-based route convention across producers or introduce a dedicated request field. A local `RouteKey()` wrapper alone cannot establish that shared convention.

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

The cloned ISS source confirms that a target must be read through `get_instrument_name()`, `get_channel_group()`, and `get_channel()`. The generated [source.tl](../runtime/scripts/teal/source.tl) maps `getVoltage` to `GET_VOLTAGE` and constructs the CallStack, which preserves the intended ownership: Go supplies the endpoint and generated Lua owns the command.

The generated source module must also be loaded explicitly. When its compiled Lua is supplied through an ISS helper directory, use `local source = require("source")` and call `source:getVoltage(...)`.

### 8. The new Lua directory must be the configured script path

The dispatcher appends `<measurement>.lua` directly to the configured scripts directory. The `--user-measurement-luas` value must therefore point to `runtime/scripts/lua`, not `runtime/scripts`.

## Recommended implementation order

### Step 1: Write down the cross-repository contract

Two earlier questions are now answered: current ISS target accessors are known, and helper directories require `require(...)`. Confirm the remaining items before spreading assumptions through handlers:

1. Which coordinated route contract will replace the current descriptive routine message (`"Performing a ramp measurement"`)? The inspected routine does not establish `get_voltage` as a message convention.
2. Which shared configuration owns Falcon scope, access, characteristic, and instrument type?
3. Does an API role describe flow relative to Falcon or relative to the instrument command?
4. How are global settings represented when they have no channel gate in the wire map?
5. Which synchronized native core/Go binding versions will the hub select? Current core `1.2.18` and the cloned bindings use enums; no enum-to-string migration is required by these active APIs.

The role question is essential. Existing hub source fixtures label settable voltage as `output` and measured voltage as `input`. The cloned ISS example labels voltage-to-set as `input` and measured voltage as `output`. The builder now hard-codes the hub fixture convention. Reconcile it with the shared schema/generator before using upstream APIs directly.

Keep temporary compatibility logic inside adapters. Do not put fallback guesses into every handler.

### Step 2: Complete configuration and port identity around the existing builder

The previously proposed `ParseInstrumentAPIs -> BuildPortLibrary -> ConnectWireMap -> NewConnectedPortsFromConnections` chain is implemented and selected in production. Do not reimplement it or restore the mock builder.

Extend it to consume the agreed hub configuration, including top-level/global settings and explicit scope, access, characteristic, units, and instrument identity. Preserve channel and capability uniqueness, reject invalid/unmatched mappings, and make the advertised Falcon port round-trip through `ResolveConnectedPort` without renaming or dropping identity fields.

The library already keys entries by `<instrument identifier>.<channel group>.<channel>.<IO name>`. Keep the capability in the entry as well as in the key, reconcile schema `suffix` versus the hub's required `name`, and define how global settings bypass channel-gate wire-map joining.

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

The target also exposes `get_channel_group()`. The current generated `source.tl` wrapper drops that value and creates a CallStack with only instrument, command, and channel. Confirm that `teal-api-gen` writes the command's configured channel group into the CallStack, especially for instruments with more than one channel group. Fix that in the generator or Teal source, not in each measurement script.

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
| `runtime/internal/handlers/manager.go` | Keep the real builder; extend its inputs to the agreed validated configuration. |
| `runtime/go.mod` and `ports/falcon-core/vcpkg.json` | Select and verify a matching Go/native core pair; the default builder is already used in production. |
| `runtime/internal/ports/api.go` | Parse or adapt the approved new port configuration fields. |
| `runtime/internal/ports/connections.go` | Complete scope/capability policy and port round-trip identity; construction APIs already exist. |
| `runtime/internal/handlers/port_request_handler.go` | Serialize settings with the correct Falcon constructor. |
| `runtime/internal/interpreter/falcon_core.go` | Restore the minimal request extraction and route-key adapter. |
| `runtime/internal/interpreter/router.go` | Register one exact handler. |
| `runtime/internal/interpreter/commands/get_voltage_handler.go` | Move/rewrite it against `InstrumentTarget` and current port fields. |
| `runtime/internal/interpreter/commands/measure_command_handler_response.go` | Move the active response builder back to a usable package. |
| `runtime/scripts/lua/get_voltage.lua` | Use the generated target-based API contract. |
| `runtime/scripts/teal/source.tl` or `teal-api-gen` | Change only if its generated target/CallStack contract differs from ISS. |

## Avoid during this task

- Do not restore measurement metadata as the routing source.
- Do not infer a measurement from port capability.
- Do not make Go choose Lua command verbs.
- Do not migrate all measurement scripts at once.
- Do not copy the deprecated monolithic measurement handler back into service.
- Do not hard-code assumptions from unmerged external repositories throughout the hub; isolate them in adapters.

## Repo observations (2026-10-05)

### Reviewed revisions and dependency selection

| Repository | Local HEAD | Observation |
|---|---|---|
| `falcon-core` | `fe23fc76` (`v1.2.18`) | Expanded characteristics, richer ports, Measurement/Setting messages, and corresponding C APIs are present. |
| `falcon-core-libs` | `08e8d0dc` | Local tags `go/falcon-core/v0.0.7` and `go/falcon-core/v0.0.8` point here; native overlay targets core `1.2.18`. Selected Go port/message bindings match the current C API. |
| `falcon-routine` | `847b840` | Updated native dependency to core `1.2.18`; producer and settings gaps remain below. |
| `falcon-instrument-hub` | `abe4166` | Production port construction is wired; measurement routing and settings handling remain unfinished. |
| `instrument-script-server` | `3da21b0` | Typed targets/domains and explicit helper loading are implemented; its API schema is incompatible with several hub parser assumptions. |
| `instrument-target` | `4002d76` | Lua methods require the userdata receiver; confirms the target accessor contract. |
| `instrument-domain` | `5b19217` | Supporting domain library is cloned; waveform integration was not executed. |
| `instrument-call-stack` | `6c34897` | Supporting command descriptor/Lua binding is present; generated wrappers must preserve its channel-group identity. |
| `instrument-data` | `053205f` | Supporting data library is cloned; buffer lifecycle integration was not executed. |
| `instrument-controller` | `9d9df1c` | Downstream overlay still selects core `1.2.16`; it is not automatically using the new sibling core checkout. |

The hub's [`go.mod`](../runtime/go.mod) and `go.sum` still select bindings `v0.0.6`, with no local `replace`; its [native overlay](../ports/falcon-core/vcpkg.json) selects core `1.2.17`. In contrast, [routine's overlay](../../falcon-routine/ports/falcon-core/vcpkg.json) and [core-libs' overlay](../../falcon-core-libs/ports/falcon-core/vcpkg.json) both select `1.2.18`. Updating cloned repositories does not update these consumers' builds. These are checked-in selections, not proof of what native library is installed on the machine.

The active C++/C/Go definitions inspected here support the view that the main core API refactor has landed and that the selected Go bindings have followed it. This review did not build all of core or audit every generated binding, so “fully refactored” should not be read as an all-repository test result. The older standalone `Instrument.hpp` alias still needs cleanup or an explicit compatibility decision.

### Original refactor checklist mapped to current code

The repeated measurement-router item in the initial plan is consolidated below.

| Original work item | Observed status | Evidence / remaining boundary |
|---|---|---|
| Update falcon-routine to improved API | **Partially integrated** | Core `1.2.18` dependency and current five-argument `MeasurementRequest` construction are present. Routing, settings consumption, and hub tests are unfinished. |
| Update std-lib to improved core/routine binding | **Not verified** | No standalone `std-lib` checkout is present in this workspace. Its consumers and generated bindings cannot be certified from these repositories. |
| Update mock-hub to invert routine communications | **Not verified** | No standalone `mock-hub` or `falcon-comms` checkout is present. Routine calls `subscribe_measure_response(json_req, timeout_ms, timestamp)`, but the transport implementation and mock inversion are external. |
| Add integration tests for Falcon library | **Not demonstrated end to end** | Core has C/C++ serialization integration tests and Go has binding tests. Routine's CMake target lists only database/log tests. Hub's named get-voltage integration test uses obsolete APIs and a fake executor. No successful cross-repository run was obtained. |
| Add hub SettingRequest -> SettingResponse handler | **Missing in inspected hub/routine paths** | Core C++/C messages and Go wrappers exist; hub API/manager/handlers do not define or register a settings transport handler, and routine exposes no settings request helper. |
| Set up router and fulfill settings requests | **Missing** | `MeasurementHandler`/`Router` only accept measurement requests; no setting router or dispatch path was found. |
| Connect measurements through router to response | **Partially integrated** | NATS envelope, deserialization, dispatcher, and response publication exist. Router registers zero handlers; command package, target script, route key, and port identity remain blockers. |
| Update falcon-core API | **Present for inspected contracts** | Rich ports, current measurement constructor, and settings request/response are implemented in core `1.2.18`; full build/test completion not established here. |
| Core contains real instruments and allowed properties | **Vocabulary present; hardware policy incomplete** | Instrument name, category enum, scope/access, and characteristic enum exist. The generic port constructor does not validate a per-instrument characteristic allowlist or map physical devices to ISS endpoints. |
| Update falcon-core C API | **Present for inspected contracts** | Port constructors/enums and Measurement/Setting message C APIs reflect the reviewed C++ interfaces. |
| Update falcon-core-libs C API bindings | **Present for inspected Go contracts; hub pin behind** | Go uses the new C port constructors and settings wrappers, and targets core `1.2.18`; hub still selects bindings `v0.0.6`/core `1.2.17`. |
| New validated hub config wrapping instrument API | **Incomplete** | Existing wire-map validation and API parser are not the proposed full hardware/property configuration. ISS rejects the hub-only `instrument_type` field; scope/characteristic/global settings policy is absent. |
| Add setting options to hub Port and connect configs | **Partial** | `PortEntry`, catalog partitioning, and payload include settings; missing scope, forced defaults, ignored global IO, and lack of allowed-property validation prevent completion. |
| Update hub PortRequest -> Response | **Partial** | `PortPayload` contains knobs/meters/settings, but settings serialize as meters and advertised names fail resolver round-trip. Routine still consumes only knobs/meters. |

### Concrete falcon-routine follow-up

The latest commit is a focused compatibility update: it changes the core pin and the `Timer()` call, adds the validator overlay, and removes an empty `test_hub.cpp`. It is not evidence that the remaining multi-repository plan has been completed.

In [`src/hub.cpp`](../../falcon-routine/src/hub.cpp) and [`include/falcon-routine/hub.hpp`](../../falcon-routine/include/falcon-routine/hub.hpp):

- `request_measurement()` serializes the caller's request, awaits a response through `RoutineComms`, and pulls the first measurement data item. It does not add or normalize a route key.
- `ramp()` supplies the descriptive message `"Performing a ramp measurement"`. Establish its route contract together with the hub and other producers.
- `request_port_payload()` deserializes only `resp.knobs` and `resp.meters` and returns a two-element tuple. Extending the hub payload with `settings` has not yet extended routine's public interface.
- No `SettingRequest`/`SettingResponse` request helper is exposed. Add the agreed communication envelope, correlated response, and settings port access together with the hub implementation.
- [`tests/CMakeLists.txt`](../../falcon-routine/tests/CMakeLists.txt) registers `test_database.cpp` and `test_log.cpp`, with no hub request/response tests. Add producer-to-hub coverage for ports, measurement route selection, settings, and error/timeout behavior.

Two additional source-level concerns should be covered while changing these helpers: `ramp()` ignores the boolean result of `safe_voltage_change()` and does not reject a zero/negative `max_ramp_rate`; `get_ohmics_connected_to_voltage_sources()` starts its reverse loop at unsigned `size() - 1`, which underflows for an empty result. These were observed in source, not reproduced in a runtime test.

### Settings need their own complete contract

Core already defines [`SettingRequest`](../../falcon-core/include/falcon-core/communications/messages/SettingRequest.hpp) as message + getter ports + a port-to-Quantity setter map, and [`SettingResponse`](../../falcon-core/include/falcon-core/communications/messages/SettingResponse.hpp) as message + a port-to-Quantity getter map. Matching [Go request](../../falcon-core-libs/go/falcon-core/communications/messages/settingrequest/settingRequest.go) and [response](../../falcon-core-libs/go/falcon-core/communications/messages/settingresponse/settingResponse.go) wrappers are present.

The missing work is transport and execution: define subjects/envelopes and correlation with the communications consumer, expose settings through routine, resolve local/global setting identity, enforce allowed access and characteristics, execute typed ISS operations, and return attributed quantities or an agreed failure response. Advertising a `settings` string in `PortPayload` does not provide this execution path. C++ permits a setting without a gate connection; the complete C/Go/config path for global settings still needs explicit validation.

### Schema, generator, and script observations

The new hub library builder uses its own older API dialect. In addition to missing Falcon-specific attributes, the upstream schema uses string model names, requires top-level IO, and uses channel-group `suffix` identifiers. An agreed wrapper configuration should preserve the ISS API and attach Falcon policy explicitly, or the schema/generator/parser must be revised together.

The generated [`source.tl`](../runtime/scripts/teal/source.tl) omits `channel_group` from its CallStack. The active get-voltage Lua still consumes a Go-built CallStack. The guide's target-based Lua example is therefore a proposed replacement, not the current behavior. The `teal-api-gen` repository is not cloned here; its hub [dependency recipe](../ports/teal-api-gen/portfile.cmake) and generated output are available, but the generator implementation was not reviewed.

The ISS target fixture's dot-call issue also means the accessor example above should be validated against the actual [`instrument-target` binding](../../instrument-target/src/instrument-target-lua.c), not copied verbatim from that fixture. Directory helper loading and scalar/array target/domain manifest entries are present in [`CommandHandlers.cpp`](../../instrument-script-server/src/daemon/CommandHandlers.cpp) and the [protobuf](../../instrument-script-server/proto/instserver/daemon/v1/daemon_messages.proto).

### Validation and next completion boundary

The offline check used the cached Go 1.25.2 executable, `CGO_ENABLED=1`, `GOPROXY=off`, `-mod=readonly`, the native pkg-config directory from the hub's CMake cache, and:

```sh
go test -tags cgo,falcon_core -run '^$' \
  ./internal/interpreter/... ./internal/ports ./internal/handlers/...
```

This is a compile-only check. It stopped at missing `falcon-core-c-api.pc` for the core-dependent packages; the device-config handler package reported `[no tests to run]`. Consequently the package-boundary and stale-signature findings above are source observations, not claimed compiler diagnostics from a completed native build. Initial toolchain-launch attempts also encountered local Go 1.22.6/checksum configuration issues; invoking the cached 1.25.2 binary directly reached the native dependency failure. No dependencies were installed and no passing integration result is claimed.

The existing [`get_voltage_handler_integration_test.go`](../runtime/cmd/get_voltage_handler_integration_test.go) calls `NewMeter` with the former string-based signature, calls `measurementrequest.New` with both message and measurement name, constructs old connected-port fields, and asserts a serialized CallStack from a fake executor. Update it alongside the implementation; its filename alone is not evidence of a current ISS integration test. The [port tests](../runtime/internal/ports/connections_test.go) already cover construction/resolver pieces, but a serialize-then-resolve test is needed to expose the current name mismatch.

Prioritize the remaining work in this order:

1. Select matching core/C/Go versions and make the native dependency available to the hub build.
2. Agree producer route keys, settings transport, and validated instrument/property configuration across routine/comms/hub.
3. Complete port scope/capability/identity and setting serialization around the existing production builder; extend routine's port response consumption.
4. Repair the interpreter package, register one exact route, and complete the typed-target/get-voltage path including generated channel-group preservation.
5. Update stale tests and demonstrate the full correlated measurement round-trip; implement and verify a settings getter/setter round-trip with the same identity rules.
6. Verify std-lib, mock-hub, communications, and generator consumers in their own repositories before calling the original multi-repository refactor complete.
