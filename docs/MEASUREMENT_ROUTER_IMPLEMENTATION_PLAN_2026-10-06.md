# Complete MeasurementRequest → Router → MeasurementResponse

Reviewed: 2026-10-06, hub `cf1c33b`. Start with one scalar `get_voltage` measurement, then extend to other workflows.

## Starting point and dependencies

The [NATS handler](../runtime/internal/handlers/measure/measure_command_handler.go) already deserializes requests, calls the router, and publishes successful responses. The router registers no handlers; the draft handler and helpers under `internal/interpreter/commands` are stale and live in a separate Go package.

Use the catalog/identity contract from the [Step 2 plan](STEP_2_PORT_CONFIGURATION_PLAN_2026-10-06.md). Local handler work can proceed with fixtures while routine/std-lib changes continue. The supported request shapes, response data location, and failure envelopes must be agreed before declaring end-to-end completion.

## Structural matching contract

This revision supersedes the earlier instruction to require `RouteKey()`/`Message()` routing. The existing `CanHandle` interface supports classification by request structure. An explicit operation identifier is needed only if different intended workflows remain indistinguishable from the available request and configuration information.

For the first scalar `get_voltage` handler, `CanHandle` should match all of:

- No waveforms and exactly one getter.
- Getter role `Meter`, readable access, and voltage-compatible units.
- A supported instrument/API capability for reading voltage; instrument category alone is insufficient.
- Meter transforms and time-domain semantics supported by this scalar implementation. Initially accept a documented simple case and reject the rest explicitly; the mandatory time-domain object being present is not itself a mismatch.

`pseudo_name = P1` identifies the endpoint to resolve, not the generic handler. The same predicate should accept another correctly configured gate. A meter on a `dc_voltage_source` can qualify when its API supports voltage readback. Ordinary core meter constructors set characteristic to `None`, so do not require a setting characteristic such as `SourceVoltage` unless the advertised meter actually carries it.

Waveform factories such as Cartesian 1D/2D return the same `Waveform` type. Inspect `Waveform.Space()`, the discrete space's `Knobs()`, axes, unit-space `Dimension()`/`Shape()`, and transforms; waveform count is not sweep dimensionality. Measurement getters remain in the request's separate getter list, and a sweep may contain both controls and getters.

Use `CanHandle` for a side-effect-free match (`false, nil` for a valid unsupported shape; an error for malformed/unreadable data). Use `Handle` for target resolution/execution/response construction. The dispatcher's local `MeasurementRequest{Script, Variables}` is an execution job, distinct from the core measurement request; it should not classify Falcon requests. `RunAll` runs jobs concurrently, so dependent set/read operations should stay in an ordered Lua workflow or be sequenced explicitly.

## Implementation instructions

1. **Analyze the typed request.** Add shared inspection helpers in `falcon_core.go` for waveform count, waveform space dimensions/shape, knob ports and transforms, measurement getters, meter transforms, and time domain. Prefer a request profile analyzed once by the router, with catalog resolution available where capability matching needs it. Use native accessors and propagate extraction errors. An empty waveform list means no control waveform; settings are a separate `SettingRequest`, not the fallback for waveform-free measurements.

2. **Repair the package and register the handler.** Move/rewrite the minimal live get-voltage handler and response helpers beside `router.go`; remove their broken duplicate implementations from `commands`. Match the interface's `config.WireMap` value type. Register `getVoltageHandler` in `NewRouter`; replace its unconditional `CanHandle` with the structural predicate below. Require exactly one match, or document an explicit specialization priority; update the current first-match-wins tests so overlapping predicates cannot silently select the wrong workflow. Keep deprecated monolithic code inactive and ensure `go test ./...` does not encounter stale active files.

3. **Resolve the actual request getter.** Read the getter through the core handle and call `config.ConnectedPorts.ResolveConnectedPort(getter)`. Use current `ChannelGroup`/`Channel` fields and retain the original port for response attribution. Resolve against the complete identity, including its connection where applicable; do not identify the operation from a gate name alone. If `CanHandle` needs resolved capabilities, pass the analyzed/resolved request to it or inject the catalog explicitly; the current signature receives only the raw request. Close native handles on every exit path.

4. **Dispatch a typed target.** Serialize `instrumenttarget.Target{Instrument, Group, Channel}` and pass it as `instrumentserver.InstrumentTarget` in the `getter` variable to `dispatcher.RunAll`, with script `get_voltage`. Replace the draft's nonexistent `Descriptor`, obsolete resolver call, and `CallStack` variable type.

5. **Complete the Lua contract.** Update [get_voltage.lua](../runtime/scripts/lua/get_voltage.lua) to load `local source = require("source")` and use `getter:get_instrument_name()`, `getter:get_channel_group()`, and `getter:get_channel()`. The generated source wrapper owns command selection and CallStack construction. Fix its currently omitted channel group in Teal/generator output and regenerate Lua. Configure the measurement directory as `runtime/scripts/lua` and make the compiled helper available through ISS helper loading.

6. **Validate results and construct the response.** Require one successful dispatched result with the expected instrument/group/channel and output identity, one finite numeric value, and compatible units. Convert units explicitly if necessary. Build `acquisitioncontext.NewFromPort(originalGetter)`, a labelled measured array, and `measurementresponse.New(arrays)` using current bindings. Return `FalconMeasurementResponse`; preserve port attributes and release intermediate handles even on partial failure.

7. **Finish the transport contract.** Reuse the outer handler's data-first publication and timestamp/hash correlation. Verify data-location fields with routine/comms: the hub currently puts the data subject in `Stream` and leaves `Channel` empty, while routine calls `pull_measurement_data(resp.stream, resp.channel, 1)`. Align these fields and add agreed correlated failures for valid request envelopes when routing/execution/serialization fails; avoid publishing a success after a failed data publication.

8. **Verify the whole path.** Update the stale get-voltage integration test to current constructors and typed targets. Cover matching/nonmatching request shapes, overlapping handlers, invalid getters, unresolved ports, unsupported transforms/time domains, ISS errors, wrong result identity/units, non-finite values, and response attribution. Confirm that changing the descriptive message or choosing another valid gate does not change the operation selected. Then run one real ISS + NATS/JetStream round-trip and verify the consumer can retrieve and deserialize the correlated response. A fake executor alone does not prove Lua or transport compatibility.

## Completion boundary

A serialized request using an advertised meter resolves uniquely, executes the generated Lua command through ISS, and returns a correlated, retrievable Falcon response preserving the getter identity. Failure behavior is tested against the agreed consumer contract.

Run focused interpreter, measurement-handler, and command tests with `-tags cgo,falcon_core` using the configured native environment, then the broader suite. Add setters, multiple targets, buffers, and waveform handlers after this first round-trip passes; these remain required before claiming all measurement workflows are complete. Settings requests are a separate path.

Source-reviewed instructions only; no implementation or tests were run for this document.
