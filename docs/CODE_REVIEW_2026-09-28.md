# Hub refactor review — 2026-09-28

Reviewed the current worktree at `f1b2e7a` (`origin/main` was `9a5b13b`). Existing local documentation, log files, coverage changes, and deletions were treated as context rather than reviewed changes. The refactor has useful boundaries now: the NATS adapter, router, dispatcher, ISS client, and buffer manager are separated and individually testable.

## Findings

### 1. P0 — Production accepts measurement requests but cannot route any

[NewRouter](../runtime/internal/interpreter/router.go#L88) constructs an empty handler list. Every valid request therefore returns `no measurement handler matched request`. The NATS adapter only logs that error and publishes no correlated failure response, so callers see a timeout.

Register one validated measurement handler before enabling the subscription, or fail startup when the registry is empty. Publish a terminal error containing the request timestamp/hash for every parse, route, execution, and serialization failure.

### 2. P0 — ISS startup configuration leaves the measurement path without a client

[NewRuntime](../runtime/cmd/main.go#L293) starts and connects to ISS only when `AutoStart` is false. The default is true, while `--no-iss` sets it to false. With the default configuration, `services.issClient` remains nil and is passed into the measurement stack. Production also passes an empty instrument API path list, leaving no ports for future handlers to resolve.

Define separate modes for “start an owned ISS,” “attach to an existing ISS,” and “disable ISS.” Construct a ready client in both enabled modes, make `--no-iss` disable it, and validate API/port configuration before subscribing handlers.

### 3. P1 — The new DataBuffer path loses scalar buffers and has no completed lifetime

[measureJobResultToCallResults](../runtime/internal/instrumentserver/client.go#L575) ignores each return parameter's declared type. A scalar DataBuffer arrives in the protobuf string field and becomes a plain Go `string`; [the dispatcher](../runtime/internal/interpreter/dispatcher.go#L67) registers only the named `DataBuffer` type, so the reference is skipped.

For buffers that are registered, production never calls `ReadBuffer` or `ReleaseRequestor`; those methods are currently test-only. Partial registration failure also leaves earlier buffers claimed. Decode values using both protobuf payload and declared type, validate native metadata, then give each request an explicit read/copy/release lifecycle with rollback.

### 4. P1 — A job-status RPC error can panic the client

[checkJobStatus](../runtime/internal/instrumentserver/client.go#L254) reads `resp.Job.Status` before returning `err`. A normal gRPC failure can return a nil response, causing a nil-pointer panic instead of an error. Check the RPC error and response fields before dereferencing, and add a regression test with `(nil, error)`.

### 5. P1 — The test baseline was not migrated with the package split

The targeted test run failed to compile `internal/interpreter`: `measurement_annotations_test.go` still refers to `Manager`, and `measurement_name_test.go` still refers to the old measure handler and mocks. `cmd` also contains expectations for the removed measurement manager; one test panics after constructing incomplete dependencies. This prevents the package suite from distinguishing expected unfinished behavior from new regressions.

Move or rewrite those tests at the same boundary as their production types. Remove retired measurement-manager assertions. Keep `go mod tidy` as an explicit maintenance target rather than a prerequisite of every `build` and `test-*` target.

### 6. P2 — Port serialization leaks native handles

[serializePortsToCerealJSON](../runtime/internal/handlers/port_request_handler.go#L152) creates an empty/container `Ports` handle and one `InstrumentPort` handle per connection without closing them. Repeated port requests can accumulate native allocations, including on intermediate errors. Close each handle immediately after its last use and add a repeated-call leak check.

## Suggested next gate

Implement one scalar getter end to end: resolve one port, pass a typed CallStack to Lua, execute ISS, select one attributed result, publish success or a correlated failure, and leave no native resources owned afterward. That narrow path will exercise the new architecture before buffered and multidimensional measurements are migrated.

## Verification

Executed:

```text
go test -count=1 -timeout=120s -tags cgo,falcon_core \
  ./internal/interpreter ./internal/handlers/measure ./internal/databuffer \
  ./internal/instrumentserver ./internal/handlers ./internal/ports ./cmd
```

Passing packages: `handlers/measure`, `databuffer`, `instrumentserver`, `handlers`, and `ports`.

Failing packages: `interpreter` (test compilation) and `cmd` (two stale assertions followed by a panic in a retired measurement-manager test). No live ISS, NATS-to-controller workflow, hardware, race detector, or multiprocess shared-memory lifecycle was exercised.
