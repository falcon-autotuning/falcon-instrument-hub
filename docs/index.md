# Falcon Instrument Hub

The hub bridges Falcon's falcon-core measurement requests to user-provided Lua
scripts executed by instrument-script-server (ISS). NATS carries discovery and
measurement envelopes; gRPC carries ISS jobs, and an ISS CLI adapter reads buffers.

## Runtime Responsibilities

- Load instrument APIs, device configuration, and the physical wiremap.
- Publish connected knobs/meters for Falcon port discovery.
- Read Teal/Lua headers for script target capability/role requirements.
- Resolve request connections to instrument IDs/channels and dispatch the named
  script with its current handler-defined arguments.
- Build cereal measurement responses and publish them through NATS/JetStream.

The hub does not generate Lua, infer scripts from port names, or automatically
archive live results to the viewer's JSON dataset format. Annotations describe
targets, not a generic extensible argument binding system. See
[Hub Runtime](server-interpreter.md) for the implementation and limits.

## Build and Test

From the repository root:

```bash
make build-go
make test-go-short
make test-schema
```

The binaries are `runtime/bin/instrument-hub` and `runtime/bin/dataviewer`.
`make install` uses `INSTALL_PREFIX` (default `/opt/falcon`); instrument-controller
is the `/opt/instrument-controller` bundle. See the controller's packaging for
its bundled hub/ISS paths rather than treating those prefixes as interchangeable.

`make test-go` / `make test` include live ISS tests. Some current integration
helpers stop the system ISS daemon: run them only in a dedicated environment.
Retired Python measurement-specific target names fail with a migration notice;
they do not represent separate buffered/2D/3D test coverage.

## Start

Create the work directory and provide existing resources using absolute paths:

```bash
runtime/bin/instrument-hub start \
  --hub-config /absolute/path/instrument_hub_config.yaml \
  --working-dir /absolute/path/work \
  --iss-lib-path /opt/falcon/lib
```

The CLI reads `quantum-dot-config`, `wiremap`, `nats-url`, `inst-config`,
`inst-plugins`, `instrument-server-port`, `local-database`, `user-measurement-luas`,
`instrument-apis` (a YAML list), and optional `measurement-metadata` from the hub
config. Defaults and overrides are shown by `instrument-hub start --help`.
`--packages` was removed; instruments use ISS configuration files and plugins,
not Python instrument templates.

By default, the hub starts embedded NATS when no URL is provided and manages the
ISS daemon/instruments. `--no-iss` uses an externally managed ISS; it does not
disable measurement dispatch. ISS's default gRPC port is `8555`, and its default
binary is `/opt/instrument-controller/bin/instrument-script-server`.
`--log-diagnostics` opts into internal writer diagnostics; logs remain in `log/`.

## Documentation

- [Hub Runtime](server-interpreter.md): routing, annotations, jobs, and storage limits.
- [NATS Protocol](nats-protocol.md): implemented subjects and cereal envelopes.
- [Configuration Validation](CONFIG_VALIDATION.md): wiremap format and validator.
- [Data Viewer](data-viewer.md): legacy/exported datasets, not live hub archival.
- [Codebase Review](CODEBASE_REVIEW.md): original review snapshot, intentionally unchanged.
- [Deprecated Code Cleanup](DEPRECATED_CODE_CLEANUP.md): first cleanup's changes and follow-ups.
- [Developer Comprehension and Hygiene Cleanup](DEVELOPER_HYGIENE_CLEANUP.md): this cleanup's tracker.

Historical refactor plans are available in Git history; they are not current
runtime documentation. Remaining behavioral/integration findings in the review
should not be mistaken for already implemented features.
