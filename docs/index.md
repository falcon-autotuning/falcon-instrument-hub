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
