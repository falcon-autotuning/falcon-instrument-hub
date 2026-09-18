# Deprecated and Unused Code Cleanup

This change log addresses **Deprecated and Unused Code Candidates** in
[CODEBASE_REVIEW.md](CODEBASE_REVIEW.md). The original review is unchanged.
Changes are limited to the hub runtime and supporting tests/documentation;
controller, ISS, comms, and std-lib implementations are not modified.

## Change Tracker

| Review item | Status | Change |
| --- | --- | --- |
| Client capability lookup | Removed | Deleted `capability_lookup_handler.go` and its tests; removed manager subscription to `INSTRUMENTHUB.CAPABILITY_REQUEST`. Removed its private single-port serialization helper. |
| Script-name inference | Removed | Deleted `inferSetterOnlyScriptName`. Requests must supply a readable, nonblank `measurement_name`; surrounding whitespace is still trimmed. |
| Legacy instrument lifecycle | Removed | Deleted setup constants, process/chunk IDs, `InstrumentProcess`, empty `Instruments` state, `GetActiveInstruments`, and `BuildConfigurations`. Removed no-op subscriptions and unused NATS/logger state from the port registry. |
| Reflection validator | Removed | Deleted `instrument/utils.go`, whose `unmarshalAndValidate` had no callers. |
| Handcrafted connection classification | Replaced | Deleted `isOhmicConnection`; tests now check actual falcon-core typed connections built from device configuration. |
| Unused HTTP bridge | Removed | Deleted `Bridge`, its config/defaults, and unused HTTP test mock/parser. |
| Old RPC and script-data wrappers | Removed | Removed HTTP envelopes, old request/response wrappers, `InstrumentTarget`, and obsolete sweep/script-data structs. Deleted tests that only exercised those retired wrappers. Kept `ISSCallResult` and `ISSReturnValue`. |
| Independent script registry | Removed | Deleted `ScriptInfo`, `ScriptRegistry`, and outdated builtin descriptors. Annotation metadata remains authoritative for target requirements. |
| Commented wiremap loader | Removed | Deleted the commented duplicate implementation in `config/loader_new.go`; the active CGO loader and shared wiremap parser remain unchanged. |
| Generated legacy NATS declarations | Retained; follow-up required | `runtime/internal/api/api.go` is generated from an external schema/tool. No local authoritative generation source was found; declarations and timestamp extensions are not manually pruned. Their presence does not re-enable retired subscriptions. |
| Unused measurement-storage injection | Removed; subsystem retained | Removed `measurementManager` from measurement-handler state, constructors, manager wiring, and handler test fixtures. Kept startup SQLite initialization, the `measurements` package, its tests, and storage configuration until persistence ownership is decided. |

## Related Adjustments

- Removed the now-unreferenced `RouteInfo` type and `ConnectedPort.RouteInfo`.
- Simplified `instrument.NewHandler` to accept only logger and configuration;
  `handlers.NewManager` no longer accepts NATS URL or a storage manager.
- Removed unused manager configuration/NATS URL/mutex fields.
- Kept instrument-construction errors separately from annotation errors and
  reject operational startup if instrument API loading failed. This prevents
  the old empty-registry fallback from hiding a failed construction.
- Removed ignored dispatcher `ServerURL`, `RequestTimeout`, and `pollTimeout`
  fields. Dispatch still uses gRPC host/port and the client's existing deadlines;
  this cleanup does not implement configurable execution timeouts.
- Replaced obsolete package documentation referencing the deleted bridge with
  the actual gRPC dispatch and CLI buffer-resolution flow.
- Marked capability lookup protocol entries as retired and the old
  `server-interpreter.md` architecture description as historical.

## Compatibility and Preserved Behavior

Current executable controller, falcon-comms, and std-lib sources contain no
client usage of the retired capability endpoint. External clients were not
audited: any client still sending `CAPABILITY_REQUEST` must migrate before using
this hub version. Use `PORT_REQUEST` / `PORT_PAYLOAD` to discover wired physical
ports, then submit an explicitly named measurement.

Requests without `measurement_name` are logged and rejected before extraction
or dispatch; no error-response protocol is added. Clients relying on port-name
guessing must supply the script name explicitly.

Internal capabilities remain necessary and are **not removed**:
`instrument.Handler.ResolveConnectedPort`, API-derived port metadata, annotation
validation, setting capability routing, and response metadata are preserved.
Settings remain excluded from published knob/meter lists. Existing physical
port publication behavior is otherwise unchanged.

The SQLite subsystem remains a storage-metadata component, not an active
measurement archival path. No existing databases or measurement files are
deleted. Decide which component owns archival before either removing startup
storage initialization or implementing record allocation/completion.

## Verification

- Added regression tests for required measurement names using real falcon-core
  request serialization and a dispatcher spy, not fabricated cereal JSON.
- Added tests for actual device-config connection classes, active manager
  operations, and rejection of instrument API loading failures.
- Existing gRPC client and internal capability resolver tests are retained.
- Passed: `go test -tags cgo,falcon_core -short -timeout 90s ./...` from
  `runtime`, using the existing local vcpkg headers/libraries. All seven tested
  packages passed, including the new regression tests. Embedded NATS tests ran
  outside the filesystem sandbox so they could bind isolated local ports.
- Passed: `go build -tags cgo,falcon_core -o /tmp/falcon-instrument-hub-cleanup ./cmd`
  with the same local library environment.
- Optional binary `--help` smoke check could not run: executing the `/tmp`
  output returned `Permission denied`, including outside the sandbox.
- Passed: `gofmt`, `git diff --check`, and a search confirming no retired
  implementation symbols remain in handwritten runtime Go code.
- Original review SHA-256 remains
  `8f5050cc6cbf8f69fe36dfd2af7e8976296a8c7d687ca98630d4f5284079a3f4`.
- Full live ISS/controller integration tests were not run. The short suite
  deliberately skips tests that could stop an existing system ISS daemon;
  installed hub binaries were not replaced.

## Remaining Follow-ups

1. Retire unused envelopes in the authoritative falcon-api schema, regenerate
   bindings, and coordinate any clients still using the old channels.
2. Decide persistence ownership, then remove or connect the retained SQLite
   startup subsystem and its configuration deliberately.
3. Broader behavior, fixture, integration-test, and documentation findings in
   the original review are outside this cleanup's scope.

User deletions of `PORT_REFACTOR.md` and `REFACTOR_STATUS.md`, already present
before this cleanup, were not changed or restored.
