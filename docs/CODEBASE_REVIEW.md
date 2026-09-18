# Falcon Instrument Hub Codebase Review

Review date: 2026-09-18. Hub commit: `c92fa79`; instrument-controller
comparison commit: `b8ec0f3`.

## Scope and Review Basis

"falcon-instrument" is interpreted as `falcon-instrument-hub`, consistent with
the preceding discussion. This review covers its handwritten runtime code,
Go tests, schemas, build files, documentation, Lua examples, and fixtures.
Generated protobuf and command declarations were checked for their contracts,
not audited as handwritten implementations. Vendored dependencies, platform
triplet collections, and the bundled Plotly implementation were excluded.

The integration reference is instrument-controller's
[data-retrieval.cpp](../../instrument-controller/tests/instrument-control/data-retrieval.cpp)
and its API, Teal, and plugin fixtures. The intended flow is:

1. Load instrument APIs, wiremap, and measurement script annotations.
2. Falcon requests `PORT_PAYLOAD` and selects a published port by physical connection.
3. Falcon constructs a falcon-core `MeasurementRequest` with `measurement_name`.
4. The hub resolves script target metadata against connected API IO types.
5. ISS executes the compiled Lua script through gRPC and instrument plugins.
6. The hub constructs a cereal-compatible response with resolved units and type.

The controller tests are a reference for client usage, but some of their scripts
and assertions also need improvement. Hardcoded expected values in a unit test
are normal. The concerning assumptions are those that bypass configuration,
duplicate production decisions, silently invent data, or make a test pass
without exercising the intended contract.

`P1` means a correctness issue or substantial integration gap; `P2` means a
validation, maintainability, or coverage issue; `P3` means cleanup. Findings
below are based on source inspection unless explicitly described as executed.

## Prioritized Findings

### 1. P1: Annotations do not make arbitrary measurement scripts dispatchable

**Locations:** [measure_command_handler.go](../runtime/internal/handlers/measure_command_handler.go),
lines 426, 589, 740, 757, 947, and 1044;
[measurement_metadata.go](../runtime/internal/handlers/measurement_metadata.go), line 118.

Annotations select capability and role, but argument construction still depends
on hardcoded script names. A new annotated getter-only script reaches the
setter extraction and is rejected for having no setters. A new setter-only
script passes that step but reaches the generic getter extraction and is
rejected for having no getters. Other unknown scripts receive a fixed 1D/2D
sweep argument signature rather than their own declared signature.

**Consequence:** Adding a correctly annotated Lua script is insufficient to
extend the user's measurement library. The hub still needs Go changes for many
new measurement names.

**Suggested correction:** Define an explicit script argument contract and
dispatch adapter registry, or extend metadata to describe argument bindings.
Keep capability annotations focused on target resolution. Add tests using new,
unrecognized getter-only and setter-only measurement names.

### 2. P1: Successful responses can contain cached or echoed values instead of readings

**Locations:** [measure_command_handler.go](../runtime/internal/handlers/measure_command_handler.go),
lines 480, 699, 848, 865, 936, and 997;
[client.go](../runtime/internal/serverinterpreter/client.go), line 541.

The handler substitutes cached state when some getter scripts return no
instrument results, acknowledges setters with the requested value, and
substitutes the requested voltage for an empty `measure_leakage` result.
`get_trigger_leader` shares the `triggerLevels` cache with trigger level.
Setting a trigger leader overwrites the cached trigger level with `1.0`.

ISS's result protocol supplies instrument command results. It does not contain
a separate field for an arbitrary Lua `main` return. Controller scripts such as
`get_sample_rate.tl`, `get_number_of_samples.tl`, and `measure_leakage.tl` only
return arguments, so the hub's fallback supplies their apparent results.

**Consequence:** Passing integration tests can demonstrate hub state echoing
without demonstrating instrument readback. Scripts that forget a measurement
call can still produce a plausible response.

**Suggested correction:** Separate command acknowledgments, explicitly
hub-owned configuration, and measured data. Require an instrument result for
operations advertised as readings; use a separate Lua-result protocol if Lua
computed results must be returned. Separate trigger leader and level state.
Test an empty successful dispatch after priming the cache and ensure it cannot
masquerade as a fresh reading.

### 3. P1: Unknown units silently become dimensionless

**Location:** [port_request_handler.go](../runtime/internal/handlers/port_request_handler.go),
lines 298 and 354.

`symbolUnitFromString` maps both an empty unit and every unrecognized unit to
`NewDimensionless()`. A typo or a valid unsupported symbol, such as `kHz`,
therefore loses its dimensions without an error. Both port publication and
measurement response construction use this helper.

**Suggested correction:** Keep an intentional empty-unit convention, but reject
unknown nonempty units or use a supported falcon-core unit parser. Test known
units, empty units, unsupported symbols, and typographical errors.

### 4. P1: Production waveform decoding can manufacture a zero-valued waveform

**Locations:** [falcon_core.go](../runtime/internal/serverinterpreter/falcon_core.go),
line 469; [waveform_json_utils.go](../runtime/internal/serverinterpreter/waveform_json_utils.go),
line 43.

The CGO implementation navigates fixed cereal `valueN` paths and assumes the
first stored array and first domain describe a single voltage axis. Missing
paths or insufficient array data return `stubWaveformData()` with a nil error:
one zero sample and bounds `0..0.001`. The decoder also assumes that the final
array entry is excluded and returns an empty `AxisDomains` list.

**Consequence:** Unsupported serialization or waveform layouts can be treated
as valid zero-valued measurement commands. The current path is tailored to
controller identity-waveform fixtures, without tests establishing a general
waveform contract.

**Suggested correction:** Reject missing or unsupported layouts. Prefer typed
waveform APIs where available. Test actual falcon-core serialized single-point,
multi-point, multi-axis, and transformed requests, including endpoint handling.
The endpoint assumption requires verification; this review does not establish
that it is wrong for the current identity-waveform representation.

### 5. P1: API loading failures can be discarded during hub construction

**Locations:** [manager.go](../runtime/internal/handlers/manager.go), lines 59 and 71;
[instrument/handler.go](../runtime/internal/handlers/instrument/handler.go), line 40.

If `instrument.NewHandler` fails, `NewManager` logs the error and replaces it
with an empty handler. The next metadata load overwrites `err`; only that later
error is retained as `metadataError`. With unannotated scripts, metadata loading
does not parse the APIs again, so startup can proceed after a broken API load.
Wiremap connection errors are also logged while partial connections are retained.

**Suggested correction:** Return initialization errors or preserve all startup
errors and refuse readiness when required configuration fails. Test invalid API
files with both annotated and unannotated script directories.

### 6. P1: Results lose channel identity and are grouped by position or concatenation

**Locations:** [client.go](../runtime/internal/serverinterpreter/client.go), line 541;
[types.go](../runtime/internal/serverinterpreter/types.go), line 92;
[measure_command_handler.go](../runtime/internal/handlers/measure_command_handler.go),
lines 476, 557, 954, and 1232;
[measure_command_handler_response.go](../runtime/internal/handlers/measure_command_handler_response.go), line 113.

The protobuf `CommandResult` includes channel and group, but `ISSCallResult`
does not retain them. Multi-get responses assign results to getters by index;
current measurements filter a specific verb and still assign by order. The
generic sweep path resolves only the first getter, concatenates numeric results
from all calls, and creates one flat array labelled with that getter's metadata.

**Consequence:** Reordered calls, setup calls returning numbers, or scripts
reading multiple channels can associate data with the wrong physical target.
The current buffered fixtures mainly check one flattened stream's length.

**Suggested correction:** Preserve instrument/channel/group and output identity;
select measurement outputs explicitly and build one labelled result per output.
Specify the shape contract for 2D results. Test two getters, reversed result
order, and an extra numeric setup result.

### 7. P2: Runtime wiremap parsing bypasses the standalone validator's checks

**Locations:** [loader.go](../runtime/internal/config/loader.go), line 115;
[connections.go](../runtime/internal/ports/connections.go), line 58;
[api.go](../runtime/internal/ports/api.go), line 61.

The runtime YAML loader accepts unknown fields, collapses duplicate physical
endpoints into a map, and does not reject duplicate logical names. The reverse
lookup can then pick an arbitrary endpoint for a duplicated logical connection.
`ConnectWireMap` reports malformed keys but accepts zero/negative indices and
silently produces no ports for an unknown instrument or channel group. It does
not enforce the API channel parameter's `min`/`max` bounds.

**Suggested correction:** Apply equivalent validation during startup, before
converting entries to a map. Reject unmatched endpoints and out-of-range
channels. Add Go runtime tests using the existing invalid wiremap fixtures,
plus unknown identifiers and API range violations.

### 8. P2: Physical connection selection is ambiguous and capability names remain visible

**Locations:** [library.go](../runtime/internal/ports/library.go), line 34;
[definitions.go](../runtime/internal/handlers/instrument/definitions.go), line 79;
[connections.go](../runtime/internal/ports/connections.go), line 83;
controller [data-retrieval.cpp](../../instrument-controller/tests/instrument-control/data-retrieval.cpp), line 476.

Published port names still include IO capability suffixes. The meter API exposes
both `voltage` and `stream` inputs on each connection, and both are published.
The controller's `PublishedPort` returns the first matching physical connection.
Connection construction traverses maps, so that selection is not deterministic.

**Consequence:** The caller obtains an arbitrary representative capability's
port metadata. Annotation-based request routing often compensates, but initial
request units and identity still depend on which port appeared first. Publication
also does not yet provide capability-free names as previously requested.

**Suggested correction:** Define a deterministic physical-port publication and
selection contract. Decide how multiple signals or units on one endpoint are
represented before collapsing them; sorting alone does not resolve semantic
ambiguity. Add a discovery test with two inputs on one physical connection and
different units. Keep backend capability resolution internal.

### 9. P2: Batch port serialization does not release owned C handles

**Location:** [port_request_handler.go](../runtime/internal/handlers/port_request_handler.go),
line 173.

`serializePortsToCerealJSON` does not close its allocated connections, units,
instrument ports, or aggregate `Ports` handle, including the empty case. The
single-port serializer immediately below it does close its handles. The pinned
falcon-core Go binding's allocation helper does not register automatic cleanup.

**Consequence:** Repeated port discovery retains native allocations.

**Suggested correction:** Release each owned handle on success and error paths,
following binding ownership rules. Also move response-array cleanup so arrays
created before an error in a later target are released.

### 10. P2: Embedded NATS's port availability check does not bind a port

**Location:** [networking/nats.go](../runtime/internal/networking/nats.go), line 136.

`isPortAvailable` creates a `server.NewServer` but never starts it. Construction
does not establish whether the TCP port can be bound, so the advertised port
search generally picks `4222` even when another server occupies it. Production
JetStream storage also uses the shared OS temporary directory, whereas tests
correctly isolate it with `t.TempDir()`.

**Suggested correction:** Let NATS bind an OS-assigned port, or actually test the
bind if the default-port policy must be preserved. Give each runtime an explicit
storage directory. Test startup while port `4222` is occupied.

### 11. P2: Live ISS tests still probe the retired HTTP protocol

**Locations:** [live_iss_test.go](../runtime/internal/serverinterpreter/live_iss_test.go),
lines 1, 45, 112, and 144;
[client.go](../runtime/internal/serverinterpreter/client.go), line 1.

The live test waits for `POST /rpc` with a JSON `list` command, but the current
client and ISS service use gRPC. Its binary/plugin paths also assume a sibling
repository's old build layout. The setup and teardown run a system daemon stop
command, which can affect an unrelated running ISS instance.

**Suggested correction:** Probe `ListInstrumentsWithTimeout` or daemon status
through gRPC; accept explicit binary/plugin paths or controller build outputs.
Use process/PID isolation where ISS supports it, or require an explicitly
dedicated integration environment. Test transport and measurement execution,
not just instrument listing.

### 12. P2: Hub startup tests skip on a misspelled fixture path and omit current config fields

**Locations:** [hub_startup_test.go](../runtime/internal/serverinterpreter/hub_startup_test.go),
lines 112, 135, 247, and 329.

The helper looks for `2-dot-1-chargesensor.yaml`; the checked-in fixture is
`2-dot-1-chargesensor.yml`. It calls `t.Skipf` rather than failing, so both startup
tests can disappear from a successful full test run. Its configuration writer
omits `inst-plugins`, `instrument-apis`, and `measurement-metadata`, all written
by the current controller test. The Lua directory is empty. Assertions check
status/config presence or ISS reachability rather than discovery and measurement.
The binary lookup can also fall back to an installed hub from a different commit.

**Suggested correction:** Fail on missing checked-in fixtures, update the config
writer to the current contract, require the binary under review, and supply real
annotated compiled scripts. Assert port discovery and at least one measurement
with resolved metadata. Missing optional external binaries may still justify an
explicitly reported skip.

### 13. P2: Hub fixtures and examples are behind the controller integration contract

**Locations:** all four files under [test_data/instrument-apis](../test_data/instrument-apis/);
[test-config.yaml](../test_data/test-config.yaml), lines 1, 8, 9, and 15;
[runtime/scripts](../runtime/scripts/).

The API templates and generated fixtures lack required
`instrument.instrument_type`; the hub's synthetic tests supply it manually.
The hub meter fixture lacks the newer slope and trigger-level capabilities.
The sample config contains a developer's absolute paths, uses `inst-apis`
instead of the CLI loader's `instrument-apis` list, names a singular
`instrument-plugin` directory for one plugin, and points to an absent `Scripts`
directory. The current CMake file does not build those sample plugins/APIs.

The bundled Lua examples use `main(ctx, params)` and older instrument-call
construction, while the current handler sends positional target/value arguments
matching the controller's Teal scripts. For example, `get_voltage.lua` expects
`params.instrument`, but receives an `InstrumentTarget` with `id` and `channel`.
These scripts also lack the new metadata headers.

**Suggested correction:** Maintain validated, reproducible example assets aligned
with the controller flow, or label/archive the old examples. Add a fixture-loading
test instead of relying exclusively on synthetic API structs.

### 14. P2: Configuration has three inconsistent representations

**Locations:** [config.schema.json](../config.schema.json), line 1;
[hub_config.go](../runtime/internal/serverinterpreter/hub_config.go), lines 19, 56, and 129;
[cmd/main.go](../runtime/cmd/main.go), line 367.

The CLI's anonymous config struct accepts `instrument-apis`, `inst-plugins`, and
`measurement-metadata`. `HubConfig` instead uses semicolon-delimited `inst-apis`
and defaults its getter to port `5555`, while the client/dispatcher default to
`8555`. The schema disallows additional properties but does not define the new
API/plugin/metadata fields. No schema validation occurs in `applyHubConfig`.
The separate `HubConfig` offers config-relative path resolution, but the CLI
loader does not use it; changing the working directory can invalidate relative
paths that passed earlier existence checks.

**Suggested correction:** Consolidate around one typed configuration and schema,
resolve resource paths before changing directories, and share default values.
Test the exact YAML generated by controller integration, CLI overrides, and
relative paths with a different working directory.

### 15. P2: Accepted metadata fields are not used, and the legacy file is less strict

**Locations:** [measurement_metadata.go](../runtime/internal/handlers/measurement_metadata.go),
lines 84 and 106;
[annotations.go](../runtime/internal/scriptmetadata/annotations.go), lines 19 and 123;
[measurement-metadata.yml](../config_templates/measurement-metadata.yml), line 1.

The legacy file accepts `responses[].metadata_from`, `connection_from`, and
`request_source`, but dispatch and response routing never consult them. The
annotation validator restricts `request_source` to matching the target name,
while argument extraction still depends on script-specific Go branches. For
example, `set_sample_rate`'s script target is named `getter` but its physical
connection is extracted from the first waveform setter.

Unlike annotation parsing, the legacy metadata loader uses permissive
`yaml.Unmarshal` and does not validate all loaded/default target definitions
against APIs. An incomplete target can fall through to wiremap-only routing.

**Suggested correction:** Remove or clearly mark inert fields, or implement
their documented semantics. Apply structural and API validation to every enabled
metadata source. Add tests proving that unsupported fields or definitions cannot
silently change the routing path. Annotations currently validate target metadata,
not arbitrary script signatures or command-to-capability agreement.

### 16. P2: Several acquisition parameters are fixed in the hub

**Location:** [measure_command_handler.go](../runtime/internal/handlers/measure_command_handler.go),
lines 525, 534, 1079, 1099, 1143, 1175, and 1186.

The handler supplies `sampleRate = 1000`, `illuminationTime = 0.1`, and buffered
`numPoints = 1` in several paths. Those values are not derived from the request
or metadata; acquisition paths do not consistently use the cached sample rate
set through the setting scripts.

**Suggested correction:** Define where each script parameter comes from: request,
declared configurable default, or script-owned constant. Test nondefault values
and verify the dispatched arguments. Fixed fixture values are acceptable, but
should not implicitly establish the production acquisition policy.

### 17. P2: Dispatcher configuration advertises options it ignores

**Locations:** [script_dispatcher.go](../runtime/internal/serverinterpreter/script_dispatcher.go),
lines 19 and 30; [client.go](../runtime/internal/serverinterpreter/client.go), line 237.

`ServerURL` is never read despite comments describing precedence over host/port.
`RequestTimeout` is stored as `pollTimeout`, which is never used. The client
instead uses its own fixed five-minute job deadline and separate call timeouts.

**Suggested correction:** Remove unsupported options or propagate a caller
context/deadline through submission, polling, and buffer reads. Test a short
configured timeout against a job that never completes.

### 18. P2: Early startup failures can leave services running

**Location:** [cmd/main.go](../runtime/cmd/main.go), lines 111 and 195.

ISS starts before `setupCoreServices`, but its cleanup is registered only after
that function succeeds. If NATS, the measurement database, or logger setup then
fails, the newly started ISS daemon is not stopped. `setupCoreServices` likewise
does not release earlier resources when a later construction step fails.

**Suggested correction:** Register cleanup as each resource is acquired and
transfer ownership only after successful initialization. Test a failure after
daemon startup and after NATS connection creation in a dedicated environment.

## Deprecated and Unused Code Candidates

No broad removal is recommended without checking compatibility requirements.
"Unused" below means no current in-repository production caller was found.

| Location | Evidence and suggested treatment |
| --- | --- |
| [capability_lookup_handler.go](../runtime/internal/handlers/capability_lookup_handler.go), line 20; [manager.go](../runtime/internal/handlers/manager.go), line 92 | Still actively subscribed. Current controller tests no longer use `LookupCapabilityPort`, and no executable client use was found in controller, falcon-comms, or std-lib. A retirement candidate, not already dead code. Confirm external clients before removing the endpoint, envelope types, and tests. Keep internal `ResolveConnectedPort`, which measurement routing uses. |
| [measure_command_handler.go](../runtime/internal/handlers/measure_command_handler.go), line 44 | `inferSetterOnlyScriptName` still guesses measurement names from port names, descriptions, and JSON. It is an active compatibility fallback. With explicit `measurement_name`, prefer validation; retain only if an older client requires it. |
| [instrument/definitions.go](../runtime/internal/handlers/instrument/definitions.go), lines 12, 20, 30, and 96; [handler.go](../runtime/internal/handlers/instrument/handler.go), line 74 | Old setup subjects, process/chunk IDs, instrument state registry, `GetActiveInstruments`, and `BuildConfigurations` have no current production consumers. `Instruments` is initialized but never populated; a developer could mistake it for live ISS state. Remove unused state or explicitly synchronize it with ISS. |
| [instrument/utils.go](../runtime/internal/handlers/instrument/utils.go), line 10 | Reflection-based `unmarshalAndValidate` has no callers. Remove with the retired setup lifecycle. |
| [port_request_handler.go](../runtime/internal/handlers/port_request_handler.go), line 362; its test at line 179 | `isOhmicConnection` is used only by its unit test and inspects an old handcrafted JSON shape. Current connection classification uses device configuration and typed constructors. Remove the helper and replace its test with actual connection-type checks. |
| [bridge.go](../runtime/internal/serverinterpreter/bridge.go), line 12; [bridge_test.go](../runtime/internal/serverinterpreter/bridge_test.go), line 13 | `Bridge` has no execution methods or production callers. The HTTP mock and URL parser have no test callers; the only test checks default config values. Remove or implement a supported abstraction rather than leaving a misleading bridge API. |
| [types.go](../runtime/internal/serverinterpreter/types.go), lines 54 and 101 | HTTP `RPCRequest`/`RPCResponse` and old sweep/script-data structs are disconnected from current dispatch. Keep used `ISSCallResult`/`ISSReturnValue`; review older wrappers individually, since some appear only in serialization tests. |
| [script_dispatcher.go](../runtime/internal/serverinterpreter/script_dispatcher.go), line 92 | `ScriptRegistry` and hardcoded builtin script descriptors have no callers and describe outdated signatures. Remove or connect one authoritative registry to actual script metadata. |
| [config/loader_new.go](../runtime/internal/config/loader_new.go), line 20 | Large commented-out wiremap implementation uses an older key format. Delete the commented code; `LoadConfigCGO` itself is active and should remain. |
| [api/api.go](../runtime/internal/api/api.go), lines 17, 35, and 69 | Generated old internal NATS messages have no current handlers. This may be a shared schema artifact rather than a hub bug. Review the generation source before pruning; do not hand-edit generated declarations. |
| [measure_command_handler.go](../runtime/internal/handlers/measure_command_handler.go), line 252; [measurements/manager.go](../runtime/internal/measurements/manager.go), line 103 | Measurement storage manager is constructed and tested, but the handler only stores its pointer; dispatch never allocates or completes an HDF5 record. Clarify whether storage is now another component's responsibility before removing or connecting this subsystem. |

## Developer Comprehension and Repository Hygiene

These supplement the behavior findings above.

| Files | Issue and recommended action |
| --- | --- |
| [serverinterpreter/doc.go](../runtime/internal/serverinterpreter/doc.go), line 1; [docs/server-interpreter.md](server-interpreter.md); [docs/nats-protocol.md](nats-protocol.md) | Describe removed interpreter daemons, instruction generators, HTTP transport, and internal lifecycle channels. Package examples refer to nonexistent methods/fields. Rewrite around the current measurement handler, gRPC jobs, annotations, and CLI buffer adapter. |
| [REFACTOR_STATUS.md](../REFACTOR_STATUS.md), line 1; [PORT_REFACTOR.md](../PORT_REFACTOR.md), line 1 | Historical plans describe already-resolved or superseded implementations as current. Label them historical with their applicable commit, or update them. They are evidence of design history, not authoritative current behavior. |
| [cmd/main.go](../runtime/cmd/main.go), lines 62, 315, and 329 | `--packages` promises Python instrument templates but is only logged; Python initialization comments and old server naming remain. Remove the inactive flag or explicitly document compatibility status. Startup comments about initialization messages also describe a retired lifecycle. |
| [logging/logger.go](../runtime/internal/logging/logger.go), lines 48 and 132 | Unconditional diagnostic output every 500 ms makes even idle tests noisy. Gate diagnostics behind a log/debug setting. The fixed `pageSize` is a buffer tuning value, not actually obtained from the OS; rename it or use `os.Getpagesize()` if OS page semantics matter. |
| [docs/data-viewer.md](data-viewer.md); [dataviewer/main.go](../runtime/cmd/dataviewer/main.go), line 270; [demo_measurements](../test_data/demo_measurements/) | Viewer examples use old averaged/raw JSON storage, while active measurement dispatch publishes cereal data through JetStream. Demo index files contain `/home/zach/...` paths, and averaged-file loading has no equivalent relocation fallback. Explain the supported storage source and resolve demo paths relative to the dataset. No viewer tests were found. |
| `runtime/main`, `runtime/cmd/main`, `runtime/dataviewer`, `runtime/log/*`, `runtime/datacache/*.db-shm`, `runtime/datacache/*.db-wal`, `nohup.out` | These generated binaries/logs/SQLite sidecars are tracked by Git. They obscure which implementation is under review and can preserve stale local state. Remove tracked artifacts in a dedicated cleanup and update ignore rules for nested runtime output. |
| [Makefile](../Makefile), line 100 | Retired Python test target names now all run the same Go suite. This compatibility mapping is explicit, but names such as `test-3D-buffered` imply coverage that does not exist. Retire aliases or make that limitation visible to callers. |

## Test File Inventory and Hardcoded Assumptions

| Test file | Assessment | Recommended alignment |
| --- | --- | --- |
| [handlers/measure_command_handler_test.go](../runtime/internal/handlers/measure_command_handler_test.go), lines 46, 99, and 149 | Highest-priority gap. Dispatcher ignores script name, globals, and manifest. Handler tests cover malformed/empty messages and lifecycle, not a valid measurement round trip. Manually injected connections and builtin metadata test the old defaults; an unknown-script test explicitly endorses wiremap-only fallback. | Record dispatch arguments; load APIs/wiremap/annotations; submit real cereal requests built from discovered ports; parse responses and check values, units, type, target, and correlation. Add novel script names and routing failures. |
| [handlers/port_request_handler_test.go](../runtime/internal/handlers/port_request_handler_test.go), lines 22, 49, and 314 | Directly assigns synthetic connections with names/identifiers that need not agree and empty device configuration. The E2E test only looks for brackets in strings. It would pass for empty or incorrect serialized port lists. | Keep direct-input serializer unit tests, but add a configured discovery flow and deserialize payloads to assert exact connections, classes, units, types, and settings exclusion. |
| Same file, line 324 | Useful typed cereal round-trip test for explicit type and units. Its hardcoded expected metadata is appropriate for that isolated fixture. | Retain it; add connection-kind and unsupported-unit cases. It does not replace API-loading integration coverage. |
| [handlers/device_config_handler_test.go](../runtime/internal/handlers/device_config_handler_test.go), lines 51 and 122 | Creates a Go struct without `DeviceConfigCerealJSON`, then parses the response back into the same Go struct. This exercises the fallback format, not the format consumed by controller `Config::from_json_string`. | Load with the production CGO loader and deserialize through the falcon-core binding; retain a separately labelled fallback test if that path remains supported. |
| [handlers/capability_lookup_handler_test.go](../runtime/internal/handlers/capability_lookup_handler_test.go), line 18 | Tests the earlier client-supplied capability endpoint with injected metadata. Valid endpoint tests, but do not demonstrate the new discovery-plus-measurement workflow. | Retain only while the endpoint is supported; add migration coverage on `PORT_REQUEST` and measurement annotations rather than copying this lookup into controller helpers. |
| [handlers/measurement_annotations_test.go](../runtime/internal/handlers/measurement_annotations_test.go), line 10 | Positive coverage of annotation precedence, missing APIs, and capability/role mismatch, but its deliberately minimal synthetic API bypasses full ISS schema validation. | Retain unit cases and add one schema-valid controller-style fixture that resolves through a real wiremap to response metadata. |
| [handlers/instrument/port_resolver_test.go](../runtime/internal/handlers/instrument/port_resolver_test.go), lines 86 and 139 | Good focused matching/no-match/ambiguity cases. The helper name suggests a fixture loader, but constructs its own API and wiremap structs. | Preserve explicit expected answers; add file-based handler initialization using current controller assets. Rename the helper to make its synthetic nature clear. |
| [ports/ports_test.go](../runtime/internal/ports/ports_test.go), lines 14 and 147 | Reasonable deterministic unit fixtures and explicit type-validation checks. They do not load the actual hub fixtures, so missing required type fields in those files escape detection. | Add real API fixture parsing and wiremap tests for unknown groups, duplicate definitions, and invalid channel bounds. |
| [serverinterpreter/client_test.go](../runtime/internal/serverinterpreter/client_test.go), lines 9 and 85 | Current protobuf conversion tests are useful; `1000`, `Meter1`, and `buffer-123` are normal fixture values. They do not invoke a gRPC service, job polling, or the CLI buffer reader. | Add an in-process gRPC fake with standard errors, failed/cancelled jobs, multiple outputs, channel identity, and a timeout. Test CLI buffer JSON/errors with a controlled executable. |
| [serverinterpreter/bridge_test.go](../runtime/internal/serverinterpreter/bridge_test.go), line 13 | Unused HTTP server mock; only default configuration is tested. It supplies no coverage of current ISS transport. | Remove dead fixtures and put transport tests around the actual gRPC client. |
| [serverinterpreter/live_iss_test.go](../runtime/internal/serverinterpreter/live_iss_test.go), lines 45 and 144 | Old sibling-build paths, obsolete HTTP readiness check, and global daemon lifecycle. Only list/start scenarios remain. | Update to dedicated gRPC integration with configurable assets and compiled measurement scripts. |
| [serverinterpreter/hub_startup_test.go](../runtime/internal/serverinterpreter/hub_startup_test.go), lines 112 and 135 | Skipped checked-in fixture, outdated config writer, empty scripts, optional installed binary fallback, weak readiness assertions. | Fix mandatory assets; mirror the controller config contract and assert discovery and measurement execution. |
| [serverinterpreter/types_test.go](../runtime/internal/serverinterpreter/types_test.go), line 11 | Ordinary serialization tests with fixture constants; no intrinsic hardcoding defect. Some tested wrappers are not in the current runtime path. | Keep only supported contracts; do not treat these tests as end-to-end measurement evidence. |
| [scriptmetadata/annotations_test.go](../runtime/internal/scriptmetadata/annotations_test.go), line 12 | Good isolated parser coverage: malformed headers, unknown/duplicate fields, companion conflicts, and ignoring code-body text. Hardcoded header contents are appropriate. | Retain; add schema/Go-validator parity coverage when the annotation schema changes. |
| [config/loader_new_test.go](../runtime/internal/config/loader_new_test.go), lines 12 and 187 | Explicit device/wiremap YAML is a normal loader fixture. It correctly calls `LoadConfigCGO`, but does not assert cereal JSON capture or round-trip compatibility. It lacks a CGO/falcon-core build guard despite calling a tagged function. | Retain its loader checks, assert cereal JSON round trips, and tag the test consistently if both build variants are supported. |
| [measurements/manager_test.go](../runtime/internal/measurements/manager_test.go); [database_test.go](../runtime/internal/measurements/database_test.go) | Sequential IDs/dates and small files are normal storage fixtures. Dummy `.h5` contents test file existence/indexing, not valid HDF5. Several setup errors are ignored, and this subsystem is disconnected from active measurement dispatch. | Clarify coverage as storage metadata, check setup failures, and test real HDF5 only if this component owns HDF5 creation. |
| [handlers/log_handler_test.go](../runtime/internal/handlers/log_handler_test.go); [status_handler_test.go](../runtime/internal/handlers/status_handler_test.go); [test_helpers_test.go](../runtime/internal/handlers/test_helpers_test.go) | Embedded NATS and fixed example messages are legitimate isolated testing. Several tests use sleeps/timing tolerances; malformed-message tests mostly assert no crash. | Prefer observable completion where possible. Reuse isolated server helpers and assert absence of unintended replies/dispatch. These tests need not boot the controller. |
| [tests/wiremap](../tests/wiremap/) and [CMakeLists.txt](../CMakeLists.txt), line 83 | Good standalone valid/invalid validator coverage, including duplicates and unknown fields. | Reuse the invalid files to assert runtime rejection; passing the external validator does not prove startup invokes it. |

### Controller Reference Tests Also Need Qualification

These are comparison findings, not requested code changes in another repository.

| Controller files | Limitation |
| --- | --- |
| [data-retrieval.cpp](../../instrument-controller/tests/instrument-control/data-retrieval.cpp), lines 467 and 476 | Correctly consumes published cereal ports rather than constructing capability-specific request ports, but takes the first physical match. Add deterministic ambiguity handling as described above. |
| Same file, lines 623 and 643 | Generic echo assertion helpers ignore the supplied target port and check connection/value/count, without checking resolved units/type. The dedicated metadata test at line 1004 is useful, but setting responses need checks for `Hz`, dimensionless bins, and API-derived types too. |
| [get_sample_rate.tl](../../instrument-controller/tests/instrument-control/measurement-scripts/get_sample_rate.tl), line 23; [get_number_of_samples.tl](../../instrument-controller/tests/instrument-control/measurement-scripts/get_number_of_samples.tl), line 23; [measure_leakage.tl](../../instrument-controller/tests/instrument-control/measurement-scripts/measure_leakage.tl), line 19 | Return hub-provided values rather than querying plugins. Classify as configuration/echo smoke tests; add real queries/API commands if instrument readback is intended. |
| [measure_1D_buffered.tl](../../instrument-controller/tests/instrument-control/measurement-scripts/measure_1D_buffered.tl), line 36; [measure_2D_buffered.tl](../../instrument-controller/tests/instrument-control/measurement-scripts/measure_2D_buffered.tl), line 36 | Set endpoint voltages rather than executing each requested sweep step. The 2D script repeats the same maximum Y value for every iteration. These exercise stream plumbing but do not establish physical sweep correctness. Verify the instrument command sequence and sampled axes in a separate measurement-semantic test. |
| [data-retrieval.cpp](../../instrument-controller/tests/instrument-control/data-retrieval.cpp), lines 1131, 1202, and 1285 | Assertions expect volts but their failure messages still say millivolts. Update messages/comments to the current API fixtures. Fixed `VOLTMETER`/volt expectations are appropriate for those explicitly declared mock fixtures, but add another instrument/unit fixture to prove metadata is not a universal default. |

## Recommended Work Order

1. Restore trustworthy integration coverage: current fixtures, gRPC readiness,
   mandatory checked-in assets, real discovery, and recorded dispatch arguments.
2. Prevent silent substitution: reject unknown units, malformed waveforms,
   invalid APIs/wiremaps, and missing required measurement outputs.
3. Define extensible dispatch, physical-port publication, result identity, and
   parameter ownership contracts; annotations alone currently cover only targets.
4. Consolidate configuration and remove inert metadata/options and confirmed
   unused implementation remnants.
5. Retire capability lookup after checking external clients; update examples,
   documentation, and tracked artifact hygiene.

## Verification and Limits

- Passed: `go test -tags cgo,falcon_core -short -timeout 90s ./...`, using the
  existing local vcpkg headers/libraries. All seven packages with tests passed.
- The first sandboxed Go run could not start embedded NATS servers. The rerun
  outside the sandbox passed; those failures were environmental.
- Passed: `ctest --test-dir build/wiremap-validator --output-on-failure`, all six
  tests, against the existing validator build. The C++ validator was not rebuilt.
- Full live ISS/startup tests and controller integration tests were not executed
  for this review. Their current setup can stop an unrelated daemon, and the
  source issues above prevent treating them as reliable integration evidence.
- No race detector, native allocation profiling, hardware tests, or fresh
  dependency/bootstrap build was performed. Native ownership, transport, and
  configuration findings are grounded in inspected code.
- This change adds the review document only; no runtime or test fixes are applied.
