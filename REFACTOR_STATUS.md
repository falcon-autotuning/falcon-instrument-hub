# Refactor Status: Hub → ISS HTTP RPC Flow

This document records issues found during investigation of the `measure_command_handler.go`
rewrite that require alignment decisions before proceeding. Items are grouped by the file or
subsystem they affect. The hub must align to falcon-core, falcon-comms, and ISS — not the
other way around.

---

## 1. `serverinterpreter/client.go` — Two ISS API generations coexist

`client.go` contains two incompatible generations of the ISS HTTP API:

**Old async API (dead, will fail at runtime):**
- `SubmitMeasureWithGlobals` → sends `"submit_measure"` command
- `JobStatus` → sends `"job_status"` command
- `JobResult` → sends `"job_result"` command
- `WaitForJob` — polling loop over the above

**New sync API (correct):**
- `Measure` → sends `"measure"` command to `/rpc`
- `ReadBuffer` → sends `"read_buffer"` command to `/rpc`

The ISS C++ only handles `"measure"`, `"read_buffer"`, `"start"`, `"stop"`, `"list"`. It has
no `"submit_measure"`, `"job_status"`, or `"job_result"` handlers. Calls to those will receive
an error response.

`script_dispatcher.go`'s `ExecuteScript` method uses the old polling API. It will fail at
runtime if called. `RunMeasurement` uses the correct new API and should be the only path used.

**Action needed:** Remove `SubmitMeasure`, `SubmitMeasureWithGlobals`, `JobStatus`, `JobResult`,
`WaitForJob` from `client.go`. Remove `ExecuteScript` from `script_dispatcher.go`.

---

## 2. `serverinterpreter/types.go` — Dead types for the old API

`types.go` still contains:
- `SubmitMeasureParams` — parameters for the removed `submit_measure` command
- `JobStatusParams` / `JobResultParams` — same
- `ParsedMeasurementRequest` — a simplified struct with top-level `"setters"`/`"getters"` fields
  that don't match the actual falcon-core cereal JSON structure. This struct is not used anywhere.

**Action needed:** Remove all three dead types.

---

## 3. `serverinterpreter/hub_config.go` — `WireMapConfig`/`LoadWireMapConfig` is broken and redundant

`hub_config.go` defines `WireMapConfig` with a `Mappings map[string]WireMapping` field and
expects a YAML schema with a `mappings:` top-level key. The actual wiremap format used by the
test and the controller is a flat `InstrumentName.Channel: GateName` mapping, for example:

```yaml
Source1.4: P1
Meter1.1: O2
```

This format is **already handled correctly** by `internal/config.loadWireMap`, which parses the
wiremap into `WireMap = map[InstrumentConnection]InstrumentConnection` (i.e. `"Source1.4" → "P1"`).
That map is stored in `config.Config.WireMap` and is passed to `handlers.Manager` via `NewManager`.

`LoadWireMapConfig()` on `HubConfig` would silently produce an empty `WireMapConfig.Mappings`
map when called on the real wiremap file, with no error, because the top-level key `"mappings"`
doesn't exist. It does not return an error — it just gives you nothing.

**Action needed:** Remove `WireMapConfig`, `WireMapping`, and `LoadWireMapConfig` from
`hub_config.go`. The correct wiremap data is already in `config.Config.WireMap`.

---

## 4. `handlers/measure_command_handler.go` — Wiremap is available but not reachable

`Manager.config.WireMap` contains the parsed flat wiremap (`"Source1.4" → "P1"`). To build the
Lua globals in `handleMessage`, the handler needs the reverse lookup: given a gate name from the
`MeasurementRequest` setter port (e.g. `"P1"`), find `{id: "Source1", channel: 4}`.

Currently `MeasureCommandHandler` has no reference to the wiremap. `Manager.config` is stored
in `Manager` but is not passed to `NewMeasureCommandHandler`.

**Action needed:** Pass `config.WireMap` (or the whole `*config.Config`) into
`NewMeasureCommandHandler` and store it on the struct.

---

## 5. `serverinterpreter/falcon_core.go` — Waveform/domain extraction is hardcoded, not functional

Despite being in the real CGO build (`//go:build cgo && falcon_core`), several extraction
functions return hardcoded defaults and never read from the CGO handle:

- `extractDomainBounds` returns `DomainBounds{Min: 0, Max: 0.001}` unconditionally, ignoring
  the passed handle.
- `extractFirstValidWaveform` returns `Shape: []int{1}` and `RawTimeTrace: [][]float64{{0.0}}`,
  ignoring the actual waveform data.
- `extractAxisDomainsFromTransforms` returns default bounds `{Min: -1.0, Max: 1.0}`.

The stub (`falcon_core_stub.go`) also returns defaults from `ExtractWaveformDataFromRequest`.
So in both build variants, `numPoints` and the voltage domain bounds cannot be extracted from
the `MeasurementRequest`. These values must come from somewhere else (e.g. parsed directly from
the waveform structure's JSON).

**Action needed:** Either implement these functions properly using the available CGO API (the
falcon-core-libs Go bindings expose `waveform.Handle`, `domain.Handle`, `discretespace.Handle`,
etc.) or parse `numPoints` and domain bounds directly from the MeasurementRequest JSON in
`handleMessage`.

---

## 6. `serverinterpreter/falcon_core.go` and `falcon_core_stub.go` — `ExtractedInstrumentInfo` structs diverge

These two files define `ExtractedInstrumentInfo` independently (only one is compiled at a time).
They have drifted:

- `falcon_core_stub.go` has a `PortJSON string` field (the raw port JSON).
- `falcon_core.go` does **not** have `PortJSON`, nor any `ConnectionJSON` or `UnitsJSON` field.

To build the `MeasurementResponse`, the handler needs the setter port's **connection** (e.g.
`PlungerGate("P1")`) and the getter port's **instrument type** and **units** (e.g. `VOLTMETER`,
`MilliVolt`). Neither build variant of `ExtractedInstrumentInfo` currently carries these values.

In the real CGO build, `instrumentport.Handle` exposes `PsuedoName()` (returns
`*connection.Handle`) and `Units()` (returns `*symbolunit.Handle`), but `extractInfoFromInstrumentPort`
in `falcon_core.go` does not call either.

**Action needed:** Add `ConnectionJSON string` and `UnitsJSON string` to `ExtractedInstrumentInfo`
in **both** files, and populate them. In `falcon_core.go`, use `portHandle.PsuedoName().ToJSON()`
and `portHandle.Units().ToJSON()`. In `falcon_core_stub.go`, parse from the existing `PortJSON`
field.

---

## 7. Building `MeasurementResponse` — the `AcquisitionContext` label is a cross-port combination

The C++ test (`DataRetrievalTest.Gaussian1D`) expects the returned `LabelledMeasuredArray` to have:
- `connection = PlungerGate("P1")` — from the **setter** port
- `instrument_type = VOLTMETER` — from the **getter** port
- `units = MilliVolt` — from the **getter** port

This combination (setter's connection + getter's type/units) forms the `AcquisitionContext`
label for the response array. It is not documented anywhere in the hub codebase. No existing
helper builds this cross-port combination, and it is not obvious from either port's struct alone.

The `acquisitioncontext.New(connection, instrument_type, units)` CGO constructor takes all three
explicitly, so the correct call would be:
```
acquisitioncontext.New(
    connection.NewPlungerGate("P1"),   // from setter port PsuedoName
    "VOLTMETER",                        // from getter port InstrumentType
    symbolunit.NewMillivolt(),          // from getter port Units
)
```

This requires `-tags falcon_core` to build. There is no pure-Go path.

**Action needed:** Add `-tags falcon_core` to `make build-go`. Add a method on
`FalconMeasurementRequest` (in `falcon_core.go` only) that builds and serializes the
`MeasurementResponse` given `[]float64` buffer data, using the above combination. In the stub,
this method should return an error clearly stating it requires the `falcon_core` build tag.

---

## 8. `Makefile` / `vcpkg_installed` — `.pc` file prefix points to the wrong location

The `.pc` files in `falcon-instrument-hub/vcpkg_installed/x64-linux-dynamic/lib/pkgconfig/`
have their `prefix` hardcoded to:
```
prefix=/home/.../instrument-controller/vcpkg/packages/falcon-core_x64-linux-dynamic
```
This is the path where they were generated, not where they now live. When `PKG_CONFIG_PATH`
points to the hub's pkgconfig directory, `pkg-config --cflags falcon-core-c-api` returns
`-I/...instrument-controller/vcpkg/packages/.../include` which may not exist on the build
machine.

The standard fix for relocatable `.pc` files is to use `${pcfiledir}/../..` as the prefix:
```
prefix=${pcfiledir}/../..
```
This resolves correctly regardless of where `vcpkg_installed` is placed.

Additionally, `make build-go` does not pass `-tags falcon_core`, so the CGO path is never
compiled by default. This must change for the hub to produce a valid `MeasurementResponse`.

**Action needed:**
1. Update the `prefix` line in both `falcon-core-c-api.pc` and `falcon-core.pc` in the hub's
   `vcpkg_installed` to use `${pcfiledir}/../..`.
2. Update `Makefile` to add `-tags falcon_core` to the `build-go` target and use the hub's own
   `vcpkg_installed` for `PKG_CONFIG_PATH` and `LD_LIBRARY_PATH`.

---

## 9. Three instrument naming systems with no documented relationship

The test involves three distinct names for the same physical instrument:

| Context | Name for the source instrument | Name for the multimeter |
|---|---|---|
| Instrument config YAML | `MockInstrument2` | `MockInstrument1` |
| Wiremap logical ID | `Source1` | `Meter1` |
| Lua global name | `Mock1Source1` | `Mock5Meter1` |

The Lua script checks `setter.id ~= "Source1"` — this refers to the **wiremap** logical ID, not
the instrument config name. The Lua global `Mock1Source1` is a **generated name** produced by
`teal-api-gen-cli` from the instrument API YAML. The relationship between these three naming
systems is entirely implicit and undocumented in the hub codebase.

The hub currently has no code that maps wiremap logical IDs (e.g. `"Source1"`) to ISS-registered
instrument names (e.g. `"Mock1Source1"`). Whether the Lua script relies on the wiremap ID or
the generated name matters for how `setter.id` should be populated in globals.

**Action needed:** Document and verify which identifier `setter.id` in the Lua globals must
correspond to. Confirm this with the `measure_get_set.tl` script logic (`setter.id ~= "Source1"`
checks the wiremap ID, suggesting the wiremap's `"Source1"` prefix is the correct value).

---

## 10. `measure_command_handler.go` — Old NATS flow is still fully active

The old `handleMessage` still:
1. Calls `sendProcessRequest` → publishes `PROCESS_REQUEST` to NATS (ISS never subscribed to this)
2. Stores a `PendingMeasurement` waiting for `UPLOAD_DATA`
3. `handleUploadData` is subscribed to `UPLOAD_DATA` (ISS never publishes this)
4. The `dispatcher` field is wired in but **never called**

The test times out at 10 seconds waiting for `UPLOAD_DATA` that will never arrive.

**Action needed:** Rewrite `handleMessage` to call `h.dispatcher.RunMeasurement(...)` and remove
`handleUploadData`, `sendProcessRequest`, `uploadSubscription`, `PendingMeasurement`,
`pendingMeasurements`, and the `UPLOAD_DATA`/`PROCESS_REQUEST` constants.

---

## Summary of Prerequisite Work Before `handleMessage` Rewrite

In rough dependency order:

1. Remove old async API from `client.go` and dead types from `types.go`
2. Remove `WireMapConfig`/`LoadWireMapConfig` from `hub_config.go`
3. Fix `.pc` file prefixes; add `-tags falcon_core` to `make build-go`
4. Make `ExtractedInstrumentInfo` struct identical between both build variants (add `ConnectionJSON`, `UnitsJSON`)
5. Implement waveform/domain extraction in `falcon_core.go` (or parse directly from JSON in handler)
6. Add `BuildMeasurementResponseJSON(bufferData []float64) (string, error)` to `FalconMeasurementRequest` in `falcon_core.go`
7. Pass `config.WireMap` into `MeasureCommandHandler`
8. Rewrite `handleMessage`
