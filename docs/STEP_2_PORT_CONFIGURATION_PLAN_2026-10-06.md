# Step 2: Complete port configuration and identity

Reviewed: 2026-10-06, hub revision `cf1c33b`.

This implements [Step 2 of the completion guide](MEASUREMENT_ROUTER_COMPLETION_GUIDE_2026-10-04.md#step-2-complete-configuration-and-port-identity-around-the-existing-builder) against the newly pulled code.

## Confirmed starting point

Yes, this is the configuration layer discussed: `b047296` introduced it and `560b122` updated its tests and bindings. [config.go](../runtime/internal/config/config.go) now owns `HubConfig`, instrument configuration, and characteristic validation. [connections.go](../runtime/internal/config/connections.go) owns port construction and resolution; startup calls `config.NewConnectedPorts`. Channel suffixes, top-level IO, and local/global characteristic entries already exist. Extend this implementation; the old `internal/ports` paths and builder instructions are outdated.

## Dependencies and scope

Proceed while routine and std-lib are being updated. Before finalizing behavior, document the port contract: role direction, canonical identity, global settings without a gate, and the source of units/access/scope. These are the relevant Step 1 decisions; measurement route keys and producer implementation can remain pending.

Recommended identity: preserve the existing fully qualified `PortName` as Falcon `default_name`, keep `DeviceName` as the logical gate label, and retain the actual gate in `pseudo_name`. Share serialized examples with the colleague updating consumers. Setting request execution and measurement routing remain separate work.

## Implementation order

1. **Complete configuration validation — `config.go`, `api.go`.** Validate characteristics by slice index so parsed enum values survive; the current range loop validates copies. Check referenced commands, parameters, bounds, and channel groups against the API. Validate both read and write mappings and reject conflicting locations; unknown parameters must not silently become global. Accept schema-defined string model identifiers, with an explicit policy for existing numeric fixtures.

2. **Preserve port attributes — `connections.go`.** Set scope explicitly for channel and global IO; `addIOPort` currently leaves it at the zero value. Retain the IO capability/characteristic identity and resolve units and descriptions for characteristic ports, which currently omit them. Apply the agreed role/access mapping centrally; reject unsupported or ambiguous definitions rather than substituting defaults.

3. **Finish catalog assembly — `ConnectWireMap`.** Join gate-bound ports to their physical endpoints and retain global settings independently, once per instrument/property, with no fabricated gate. Define treatment of local settings on unwired channels. Reject unmatched wire-map endpoints and duplicate/conflicting mappings; preserve multiple distinct capabilities on one endpoint. Keep stable output ordering for reproducible payloads.

4. **Align identity and resolution — `ResolveConnectedPort`.** Match `default_name` to `PortName`, replacing the current comparison with `DeviceName`. Verify instrument/type, role, scope, access, characteristic, units, and connection where applicable. Reject missing, mismatched, or ambiguous identities. Global settings must resolve without dereferencing a gate handle.

5. **Complete the serialization boundary — `port_request_handler.go` (Step 3 overlap).** Add an explicit setting branch and preserve all catalog attributes; currently every non-knob becomes a meter. Use the general port constructor where convenience constructors would reset access/scope. Verify nullable connections through the selected C/Go bindings for global settings; record any binding limitation rather than creating a fake connection. This small overlap is required to prove advertised ports resolve correctly.

6. **Add focused regression coverage — configuration and port-handler tests.** Test loaded YAML through validation and catalog construction, including characteristic enum retention, local/global scope, units, invalid references, conflicting mappings, and multiple capabilities. Serialize and deserialize a knob, meter, local setting, and gate-less global setting; each must resolve uniquely to its original catalog entry. Include altered-attribute rejection and a global setting with an empty wire map.

## Completion check

With the configured native libraries available, run from `runtime/` using the Makefile's Go/native environment:

```sh
go test -tags cgo,falcon_core ./internal/config ./internal/handlers
```

Use the current `internal/config` package; `make test-ports` still targets the removed `internal/ports` directory. Report unrelated build blockers separately from these checks.

Step 2 is complete when validated configuration produces the intended catalog, invalid mappings fail clearly, and advertised ports round-trip through Falcon serialization and resolve without losing identity or attributes. This can be demonstrated with local fixtures and real core bindings before routine/std-lib integration is ready.

This document is an implementation plan based on source inspection; no implementation changes or test runs were performed for it.
