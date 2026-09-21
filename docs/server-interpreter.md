# Hub Runtime and Script Dispatch

The hub accepts falcon-core measurement requests over NATS, resolves their
physical connections against instrument APIs and a wiremap, and executes
user-provided Lua scripts in instrument-script-server (ISS). It does not
generate measurement scripts or use HTTP for ISS measurement dispatch.

## Runtime Flow

```text
Falcon -> NATS MEASURE_COMMAND -> MeasureCommandHandler
                                 -> wiremap + script target metadata
                                 -> ScriptDispatcher -> ISS gRPC job
                                 <- instrument call results + CLI buffer data
Falcon <- NATS MEASURE_RESPONSE <- cereal MeasurementResponse in JetStream
```

1. The CLI loads device configuration, wiremap, instrument API paths, and the
   measurement script directory.
2. `instrument.NewHandler` builds connected ports from API channel capabilities
   and the wiremap. ISS, not this registry, owns instrument lifecycle/state.
3. Annotation loading combines builtin target defaults, optional legacy metadata
   YAML, and script headers, in that precedence order. Headers are validated
   structurally and against loaded API capabilities/roles.
4. `MeasureCommandHandler` deserializes the request through falcon-core. A
   readable, nonblank `measurement_name` is required; port names do not choose
   the script. The handler extracts connections, values, and waveform parameters.
5. Target resolution combines the request's physical connection with the
   script's capability and role. API metadata supplies units and the explicitly
   declared `instrument.instrument_type`; no type heuristic is used.
6. `ScriptDispatcher` selects `<measurement_name>.lua` in the configured script
   directory and forwards globals and the positional argument type manifest.
7. `ScriptServerClient` submits `MeasureJob`, polls `JobStatus`, and retrieves
   `MeasureJobResult` through gRPC. ISS injects `ctx`; the client omits it from
   the transmitted parameter manifest.
8. The dispatcher resolves returned buffer IDs through the configured ISS CLI.
   The handler builds a cereal response, publishes data to JetStream, then
   publishes the correlated response envelope to Falcon.

See [NATS Protocol](nats-protocol.md) for exact subjects and envelope fields.

## Script Annotations

Place a versioned YAML block in leading Teal/Lua line comments. For example,
`get_voltage.tl` / `get_voltage.lua` for a source with a measured-voltage input:

```lua
-- @falcon.metadata
-- schema_version: 1
-- measurement: get_voltage
-- targets:
--   getter:
--     capability: measured_voltage
--     role: input
-- @falcon.end
```

The measurement must match the script basename. Targets are `getter` and/or
`setter`; roles are `input`, `output`, or `setting`. Capabilities are API
`io_types[].name` values. Optional `request_source` must match the target name.
Matching Teal/Lua companions are accepted; conflicting declarations fail
startup. Header parsing does not execute the script.

The schema is [measurement-annotations.schema.json](../schemas/measurement-annotations.schema.json);
the runtime parser is [annotations.go](../runtime/internal/scriptmetadata/annotations.go).

Annotations declare target requirements, **not arbitrary argument signatures**.
Current argument extraction still has measurement-specific branches in
[measure_command_handler.go](../runtime/internal/handlers/measure_command_handler.go).
Adding a novel script name/header alone does not establish a supported new
request shape. Unannotated scripts retain builtin/legacy target behavior;
unknown targets may fall back to wiremap-only routing.

## Configuration and Startup

```bash
runtime/bin/instrument-hub start \
  --hub-config /absolute/path/instrument_hub_config.yaml \
  --working-dir /absolute/path/work \
  --iss-lib-path /opt/falcon/lib
```

Create the working directory beforehand. Use absolute resource paths to avoid
the current working-directory/path-resolution limitation. The CLI accepts
`instrument-apis` as a YAML list and `inst-config` / `inst-plugins` as positional
semicolon-separated lists; `user-measurement-luas` selects the script directory.
See the CLI's `start --help` for overrides. The top-level config schema and
standalone `HubConfig` helper are not yet fully aligned with this CLI contract.

Defaults: ISS host `127.0.0.1`, gRPC port `8555`, and CLI binary
`/opt/instrument-controller/bin/instrument-script-server`. Operational handlers
start before ISS instruments; `STATUS.instrument-server` publishing starts after
instrument startup. `--no-iss` skips ISS lifecycle management, not dispatch.
Automatic daemon startup/shutdown is global; use a dedicated environment to
avoid interfering with another ISS instance.

Log entries are written to `log/` under the working directory. Internal writer
diagnostics are off by default; `--log-diagnostics` enables them on stderr without
changing the file log level. Real write failures and queue-loss warnings remain
visible regardless of this flag.

## Storage and Limits

Successful measurement data is published as cereal JSON through JetStream,
not automatically archived as the viewer's raw/averaged JSON datasets. Startup
still initializes the retained SQLite storage-metadata subsystem; active
measurement dispatch does not allocate or complete its archival records.
The [Data Viewer](data-viewer.md) reads legacy/exported datasets only.

The ISS client uses existing per-call deadlines and a five-minute polling
deadline; there is no configurable dispatcher execution timeout. Many handler
failures log and return without an error envelope, so callers also need a timeout.

The old `InterpreterDaemon`, HTTP `Bridge`, instruction generator, independent
script registry, and internal instrument NATS lifecycle are not supported
runtime APIs. See [Deprecated Code Cleanup](DEPRECATED_CODE_CLEANUP.md).
