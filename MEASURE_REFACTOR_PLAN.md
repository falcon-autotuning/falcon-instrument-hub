# Plan: Hub Cleanup + Data-Retrieval Test Path

## TL;DR
Three parallel streams: (A) remove dead server code + fix tests, (B) align ExtractedInstrumentInfo, (C) rewrite handleMessage to call ISS. Plus fix build system. Data-retrieval test passes only after all streams complete.

---

## Phase A — Dead code removal + test cleanup

### A1 — Remove dead production code (no dependencies)
Files: `client.go`, `types.go`, `script_dispatcher.go`, `bridge.go`, `measure_command_handler.go`

**`client.go`**: remove `SubmitMeasure`, `SubmitMeasureWithGlobals`, `JobStatus`, `JobResult`, `WaitForJob`  
**`types.go`**: remove `SubmitMeasureParams`, `JobStatusParams`, `JobResultParams`, `ParsedMeasurementRequest`  
**`script_dispatcher.go`**: remove `ExecuteScript`  
**`bridge.go`**: remove `ExecuteSetVoltage`, `ExecuteGetVoltage`, `ExecuteParsedRequest`, `ExecuteMeasurementRequestWithFalconCore`, `ExecuteMeasurementRequestJSON`; remove `ParseMeasurementRequestJSON` (defined somewhere in the package — grep to find exact location)  
**`measure_command_handler.go`**: remove `PendingMeasurement` type, `uploadSubscription` field, `pendingMeasurements` field, `handleUploadData` method, `sendProcessRequest` method; remove `UploadDataSubject`, `ProcessRequestSubject`, `UploadDataName`, `ProcessRequestName` constants; update `Subscribe`/`Unsubscribe` to not touch `uploadSubscription`; leave `handleMessage` stubbed returning an error (will be rewritten in Phase C)

### A2 — Extract MeasurementDispatcher interface (depends on A1)
File: `measure_command_handler.go` (new interface) + `script_dispatcher.go`

Add interface:
```go
type MeasurementDispatcher interface {
    RunMeasurement(scriptName string, globals map[string]interface{}) ([]ResolvedCallResult, error)
}
```
Change `dispatcher` field on `MeasureCommandHandler` from `*serverinterpreter.ScriptDispatcher` to `MeasurementDispatcher`. Update `NewMeasureCommandHandler` signature accordingly.

### A3 — Test cleanup (depends on A1, A2)
**`bridge_test.go`**:
- Remove `TestScriptServerClient_SubmitMeasure`, and any test functions for `JobStatus`, `JobResult`, `WaitForJob`
- Update `MockInstrumentScriptServer.handleRequest` switch cases: remove `submit_measure`/`job_status`/`job_result` cases; add `measure` (returns `{"ok":true,"results":[]}`) and `read_buffer` (returns `{"ok":true,"data":[],"buffer_id":"x","element_count":0}`) cases
- Keep `list`, `start`, `stop` cases

**`types_test.go`**: Remove `TestParseMeasurementRequestJSON`; keep `TestInstrumentTarget_Serialize`

**`measure_command_handler_test.go`**: Rewrite all tests:
- Introduce `mockDispatcher` struct implementing `MeasurementDispatcher`
- Fix NATS subjects: send on `INSTRUMENTHUB.MEASURE_COMMAND`, receive on `FALCON.MEASURE_RESPONSE`
- `successful_measure_command`: inject mockDispatcher that returns stub `ResolvedCallResult`; verify `FALCON.MEASURE_RESPONSE` published with correct `Hash`
- Remove `TestMeasureCommandHandler_HandleMessage/invalid_subject_format` (handler is flat subject, not wildcard — send on wrong subject and expect no crash is a valid test but fix subject first)
- Keep `invalid_json` and `empty_request` subtests, fixing subjects to `INSTRUMENTHUB.MEASURE_COMMAND`
- Remove `TestMeasureCommandHandler_WithInstruments` entirely (tested PROCESS_REQUEST Configurations — concept gone)

**Verify**: `go build -tags cgo,falcon_core ./...` + `go test -tags cgo,falcon_core ./...`

---

## Phase B — ExtractedInstrumentInfo alignment (parallel with A)

### B1 — Add ConnectionJSON + UnitsJSON fields to both build variants
**`falcon_core.go`**:
- Add `ConnectionJSON string` and `UnitsJSON string` to `ExtractedInstrumentInfo`
- In `extractInfoFromInstrumentPort`: after existing field extraction, add:
  ```go
  pseudoHandle, err := portHandle.PsuedoName()
  if err == nil {
      connJSON, _ := pseudoHandle.ToJSON()
      info.ConnectionJSON = connJSON
      pseudoHandle.Close()
  }
  unitsHandle, err := portHandle.Units()
  if err == nil {
      unitsJSON, _ := unitsHandle.ToJSON()
      info.UnitsJSON = unitsJSON
      unitsHandle.Close()
  }
  ```

**`falcon_core_stub.go`**:
- Add `ConnectionJSON string` and `UnitsJSON string` to `ExtractedInstrumentInfo` (remove `PortJSON` field or keep it — it's only in stub so keeping both is fine)
- In `extractInfoFromPortMap`: populate `ConnectionJSON` from the port map's `connection` or `pseudo_name` field if present; populate `UnitsJSON` from `units` field

**Verify**: build clean

---

## Phase C — handleMessage rewrite (depends on A1, A2, B1)

### C1 — Wiremap reverse lookup helper
File: `measure_command_handler.go`

Add `reverseWireMap(wm *config.WireMap) map[string]config.InstrumentConnection` — builds `gateName → "InstrumentId.channel.index"` map from `config.WireMap` (`map[InstrumentConnection]InstrumentConnection`). The wiremap stores `"Source1.analog.4" → "P1"` so the reverse is `"P1" → "Source1.analog.4"`.

Add `parseWireMapEntry(entry config.InstrumentConnection) (instrumentID string, channelIndex int, ok bool)` — splits `"Source1.analog.4"` into `instrumentID = "Source1"`, `channelIndex = 4`.

### C2 — Connection JSON → gate name helper
File: `measure_command_handler.go`

Add `gateNameFromConnectionJSON(connectionJSON string) (string, error)` — parses the cereal JSON for a `connection.Handle` to extract the gate name. The connection JSON (e.g. PlungerGate) has a `name` field in cereal format. The simplest approach: unmarshal with `json.Unmarshal` and look for the value field (cereal stores polymorphic types with `polymorphic_id` and a nested value; or check the actual JSON format by ToJSON on a known PlungerGate handle in a test).

### C3 — Rewrite handleMessage
File: `measure_command_handler.go`

New flow:
1. Unmarshal `api.MeasureCommand` from msg
2. `NewFalconMeasurementRequestFromJSON(measureCommand.Request)` → `falconReq`
3. `falconReq.ExtractSetters()` → `[]ExtractedInstrumentInfo`; take first setter's `ConnectionJSON`
4. `falconReq.ExtractGetters()` → `[]ExtractedInstrumentInfo`; take first getter's `InstrumentType` + `UnitsJSON`
5. Parse setter `ConnectionJSON` → gate name → reverse-lookup wiremap → `{instrumentID, channelIndex}`
6. Parse getter `ConnectionJSON` → gate name → reverse-lookup wiremap → `{instrumentID, channelIndex}`
7. `measureName, _ := falconReq.MeasurementName()` → derive script name (or use `"measure_get_set"` as default if name mapping is needed)
8. Build globals: `map[string]interface{}{"setter": map[string]interface{}{"id": setterInstrID, "channel": setterChIdx}, "getter": map[string]interface{}{"id": getterInstrID, "channel": getterChIdx}}`
9. `h.dispatcher.RunMeasurement(scriptName, globals)` → `[]ResolvedCallResult`
10. Find first result where `Return.Type == "buffer"` → `bufferData []float64`
11. Build `MeasurementResponse` JSON → publish to `FALCON.MEASURE_RESPONSE`

### C4 — MeasurementResponse construction (falcon_core build tag)
File: new file `measure_command_handler_response.go` (CGO build tag) + stub

**CGO path** (`//go:build cgo && falcon_core`):
- `buildMeasurementResponseJSON(bufferData []float64, setterConnJSON string, getterInstrType string, getterUnitsJSON string, hash int64) (string, error)`
- Deserialize setter connection: `connection.FromJSON(setterConnJSON)` (check if API exists; else reconstruct from gate name)
- Deserialize getter units: `symbolunit.FromJSON(getterUnitsJSON)` (check if API exists)
- `ctx, err := acquisitioncontext.New(connHandle, getterInstrType, unitsHandle)`
- `resp, err := measurementresponse.New(ctx, bufferData)` (check exact API signature)
- `respJSON, err := resp.ToJSON()`
- Wrap in `api.MeasureResponse{Response: respJSON, Hash: hash, Timestamp: ...}`

**Stub path** (`//go:build !cgo || !falcon_core`):
- Return `error("MeasurementResponse requires falcon_core build tag")`

---

## Phase D — Build system fixes (independent)

### D1 — Fix .pc file prefixes
Files: `vcpkg_installed/x64-linux-dynamic/lib/pkgconfig/falcon-core-c-api.pc` and `falcon-core.pc`  
Change `prefix=/home/.../instrument-controller/...` → `prefix=${pcfiledir}/../..`

### D2 — Fix Makefile build-go target
File: `falcon-instrument-hub/Makefile`
- Add `-tags cgo,falcon_core` to `build-go` and `build-release` targets
- Add `CGO_LDFLAGS="-L$(LOCAL_VCPKG_INSTALLED)/lib -Wl,-rpath,$(LOCAL_VCPKG_INSTALLED)/lib"` to these targets
- Add `CGO_ENABLED=1` to these targets

---

## Verification

1. `go build -tags cgo,falcon_core ./...` — zero errors (after Phase A1)
2. `go test -tags cgo,falcon_core ./internal/ports/...` — ports unit tests pass
3. `go test -tags cgo,falcon_core ./internal/serverinterpreter/...` — bridge/types tests pass
4. `go test -tags cgo,falcon_core ./internal/handlers/...` — measure + port handler tests pass
5. `go test -tags cgo,falcon_core ./...` — all tests pass
6. `make build-go` — produces binary with falcon_core CGO path compiled (verify with `nm bin/instrument-hub | grep falcon`)
7. C++ `DataRetrievalTest.Gaussian1D` — passes end-to-end

---

## Key files
- `runtime/internal/serverinterpreter/client.go`
- `runtime/internal/serverinterpreter/types.go`
- `runtime/internal/serverinterpreter/script_dispatcher.go`
- `runtime/internal/serverinterpreter/bridge.go`
- `runtime/internal/serverinterpreter/falcon_core.go`
- `runtime/internal/serverinterpreter/falcon_core_stub.go`
- `runtime/internal/handlers/measure_command_handler.go`
- `runtime/internal/serverinterpreter/bridge_test.go`
- `runtime/internal/serverinterpreter/types_test.go`
- `runtime/internal/handlers/measure_command_handler_test.go`
- `vcpkg_installed/x64-linux-dynamic/lib/pkgconfig/falcon-core-c-api.pc`
- `vcpkg_installed/x64-linux-dynamic/lib/pkgconfig/falcon-core.pc`
- `Makefile`

## Decisions
- `bridge.go` retained as empty shell (remove dead methods, keep `Bridge` struct + `NewBridge` for potential future use)
- `PortJSON` field kept in `falcon_core_stub.go` (harmless alongside new fields)
- Waveform domain extraction (REFACTOR_STATUS item 5) is NOT required for data-retrieval test — ISS handles sweep, hub only forwards buffer data
- `hub_startup_test.go` / `live_iss_test.go` are skip-guarded (binary absent = skip); no changes needed
- `ParseMeasurementRequestJSON` location: search for definition before deleting

## Further considerations
1. **Lua globals format**: REFACTOR_STATUS item 9 confirms `setter.id = "Source1"` (wiremap ID). Verify `measure_get_set.tl` in `instrument-controller/tests/data-retrieval-1D/measurement-scripts/` to confirm channel field name and format before implementing C3.
2. **`acquisitioncontext`/`measurementresponse` Go API**: Check actual function signatures in `falcon-core-libs/go/falcon-core/` before implementing C4 — `FromJSON` may not exist for `connection`/`symbolunit`; may need to rebuild from gate name using `NewPlungerGate` etc. (same as `port_request_handler.go` approach).
3. **Script name mapping**: `MeasurementName()` from falcon-core returns a string like `"measure_get_set"` that should match the `.lua` filename. If the hub's `--working-dir` Lua script directory uses different naming, a mapping table may be needed.
