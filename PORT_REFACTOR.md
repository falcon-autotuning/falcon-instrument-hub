# Port Library Refactor Plan

## Background

The current port system is broken and partially refactored. `JsonPort`, `PortObject`,
`PsuedoName`, and required methods on those types are **all undefined**, causing compile
errors across `internal/handlers/instrument/`. The old design had instruments push
Python-serialized port JSON over NATS during registration, which the `PortProcessor`
then augmented with wiremap and device-config data.

**New standard**: instrument API YAML + wiremap → static port library built at startup.
Port names follow the format `{vendor}.{identifier}.{channel_name}.{io_type_name}`, e.g.:


The port library is the **single source of truth** for connecting a falcon measurement
request to one or more registered ISS instrument ports. When falcon issues a measurement
command referencing a port name, the hub:

1. Looks up the port name in the library → `{InstrumentName, ChannelName, IoTypeName, role}`
2. Uses the wiremap connection to resolve the channel index for that device gate  
   (e.g. `P1 → Source1.analog index 4`)
3. Routes the command to the registered ISS instrument at `Source1.analog.4`

This replaces the current `BuildConfigurations()` / `PortOptions` cache lookup.

---

## Deprecated Code

### `internal/handlers/instrument/port_processor.go` — DELETE ENTIRE FILE

~420 lines: `PortProcessor` struct, `buildNameMapping`, `processPortProperty`,
`updatePortPsuedoName`, all cache methods.

### `internal/handlers/instrument/definitions.go` — remove/replace

**Types:** `JsonPort`, `PortObject`, `PsuedoName`, `units`, `defaultName`,
`connectionType`, `module`, `port`, `propertyIndexedPorts`, `instrumentIndexedPorts`, `PortConfiguration`

**Constants & vars:** `Knob`, `Meter`, `Port`, `ScreeningGate`, `BarrierGate`,
`ReservoirGate`, `PlungerGate`, `Ohmic`, all `*Module` consts, `connectionToModule`

**Fields on `InstrumentProcess`:** `Ports`, `Configuration`

**Field on `Handler`:** `portProcessor`

**Methods on `Handler`:** `CollectPortProperties`, `BuildConfigurations`,
`BuildPortConfigurations`, `GetMultiplePortOptions`, `InvalidatePortConfigCache`,
`AddInstrument`/`RemoveInstrument` (cache-invalidating), `UpdateInstrumentConfiguration`,
`FindPortByInstrumentPropertyIndex`

### `internal/handlers/port_request_handler.go`

`serializePortsToCerealJSON` — rewrite to accept `[]ConnectedPort` instead of `[]JsonPort`.

---

## New Code

### Phase 1 — `internal/ports/` (new package)

| File | Contents |
|---|---|
| `api.go` | `InstrumentAPI` struct: `.Instrument.{Vendor, Identifier}`, `.ChannelGroups[].{Name, IoTypes[].{Name, Role, Unit, Description}}` |
| `library.go` | `PortName string`; `PortEntry{…}`; `PortLibrary = map[PortName]PortEntry`; `BuildPortLibrary([]InstrumentAPI) PortLibrary` |
| `connections.go` | `ConnectedPort{PortName, DeviceName, InstrumentName, ChannelName, ChannelIndex, Role}`; `ConnectWireMap(wm, lib) ([]ConnectedPort, error)` |

### Phase 2 — Config wiring *(depends on Phase 1)*
- Add `InstrumentAPIPaths []string` to `HubConfig` and `Config`
- Add `ParseInstrumentAPIs(paths) ([]InstrumentAPI, error)` to `loader.go`

### Phase 3 — Handler simplification *(depends on Phase 1+2)*
- Replace `portProcessor` with `portConnections []ports.ConnectedPort` in `handler.go`
- New `CollectPortProperties() (knobs, meters []ports.ConnectedPort)`

### Phase 4 — Port request handler *(depends on Phase 3)*
- Rewrite `serializePortsToCerealJSON` to build port JSON from `ConnectedPort`

### Phase 5 — Measurement routing *(depends on Phase 3, parallel with Phase 4)*
- Replace `BuildConfigurations()` with `RoutePort(portName, deviceName) (*InstrumentRoute, error)`
- Update `measure_command_handler.go`

---

## Verification

1. `cd runtime && go build -tags cgo,falcon_core ./...` — zero errors
2. `go test -tags cgo,falcon_core ./internal/ports/...` — new unit tests using `test_data/`
3. `go test -tags cgo,falcon_core ./...` — all existing tests pass

---

## Open Questions

1. **API YAML config path**: New dedicated `instrument_apis` list field in `hub_config.yaml`, or alongside existing `inst_config`?
2. **Multiple API files**: One per instrument type (Source1, Meter1), or combined? *Recommendation: list of paths.*
3. **`serializePortsToCerealJSON` output format**: Does `instrumentport.FromJSON` in falcon-core still expect the old Python-cereal `PortObject` JSON shape, or is that interface also changing?