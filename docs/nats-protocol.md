# NATS Protocol

This describes subjects implemented by the current hub handlers. Payload
envelopes are JSON; falcon-core objects inside string fields use C++ cereal JSON,
not handwritten object dictionaries or falcon-measurement-lib script schemas.
Generated declarations in `runtime/internal/api/api.go` include legacy messages
that are not supported merely because a Go type exists.

## Active Subjects

| Subject | Hub direction | Payload |
| --- | --- | --- |
| `LOG.>` | Subscribe | Structured log envelope or plain text |
| `INSTRUMENTHUB.DEVICE_CONFIG_REQUEST` | Subscribe | Device configuration request |
| `FALCON.DEVICE_CONFIG_RESPONSE` | Publish | Device configuration envelope |
| `INSTRUMENTHUB.PORT_REQUEST` | Subscribe | Port discovery request |
| `FALCON.PORT_PAYLOAD` | Publish | Knob/meter discovery envelope |
| `INSTRUMENTHUB.MEASURE_COMMAND` | Subscribe | Measurement request envelope |
| `FALCON.MEASURE_DATA.<timestamp>` | JetStream publish | Cereal `MeasurementResponse` JSON |
| `FALCON.MEASURE_RESPONSE.<timestamp>` | Publish | Correlated completion envelope |
| `STATUS.instrument-server` | Publish | Hub readiness heartbeat |

Subscribe to response subjects **before** sending requests. Device-config and
port handlers publish to fixed subjects; they do not use the NATS request's
reply inbox. Do not assume `nc.Request` works for these endpoints.

## Port Discovery

Publish to `INSTRUMENTHUB.PORT_REQUEST`:

```json
{"timestamp": 123456}
```

The response on `FALCON.PORT_PAYLOAD` contains:

| Field | Type | Meaning |
| --- | --- | --- |
| `timestamp` | integer | Echo of the request timestamp |
| `knobs` | string | Cereal `Ports` JSON for output capabilities |
| `meters` | string | Cereal `Ports` JSON for input capabilities |

Deserialize both lists with falcon-core, select the desired physical connection,
and use the returned port when building a measurement request. Settings such as
`sample_rate` are not published as measurement ports. Their scripts resolve
setting capabilities internally using target metadata and the wiremap.

The current implementation publishes fully qualified capability-specific port
names and does not deduplicate physical connections. A connection can match
multiple returned ports; do not assume the first match is uniquely correct.

## Measurement Execution

Publish to `INSTRUMENTHUB.MEASURE_COMMAND`:

| Field | Type | Meaning |
| --- | --- | --- |
| `timestamp` | integer | Caller-selected response correlation/suffix |
| `hash` | integer | Caller correlation value, echoed in the envelope |
| `request` | string | Cereal `MeasurementRequest` JSON, including `measurement_name` |

The name selects a Lua script and its target metadata. The hub resolves its
targets and dispatches to ISS over gRPC; capability lookup is internal, not a
separate client request. Missing/blank names are logged and rejected.

On success, the handler publishes cereal `MeasurementResponse` data to
`FALCON.MEASURE_DATA.<timestamp>`, then sends an envelope to
`FALCON.MEASURE_RESPONSE.<timestamp>`:

| Field | Type | Meaning |
| --- | --- | --- |
| `timestamp` | integer | Echo of the command timestamp |
| `hash` | integer | Echo of the command hash |
| `stream` | string | Data subject `FALCON.MEASURE_DATA.<timestamp>` |
| `response` | string | The same cereal measurement response JSON |
| `channel` | string | Currently left empty by the measurement handler |

Despite its field name, `stream` identifies a **subject**, not the JetStream
stream name. The handler creates stream `FALCON_MEASURE`, covering
`FALCON.MEASURE_DATA.*`, with a 60-second maximum age. This is short-lived transport,
not durable measurement archival.

Failures often log without publishing a completion/error envelope. Use a caller
timeout; there is currently no defined NATS measurement error-response contract.

## Device Configuration

Publish `{"timestamp": 123456}` to `INSTRUMENTHUB.DEVICE_CONFIG_REQUEST`.
The response on `FALCON.DEVICE_CONFIG_RESPONSE` has `timestamp` (hub-generated
Unix microseconds) and `response` (serialized configuration string).
Its timestamp is **not** the request's correlation timestamp.

With the production CGO loader, `response` is cereal `Config` JSON captured
at load time. The fallback Go-marshalled format is not equivalent and should
not be treated as a cereal-compatible controller integration response.

## Logs and Status

Structured `LOG.>` messages have `timestamp` (Unix microseconds), `hash`, and
`message`; plain-text messages are also accepted. Log levels are derived from
subjects such as `LOG.INFO` and `LOG.ERROR`. Structured messages requesting a
reply receive a plain-text acknowledgement; this is not a measurement protocol.

`STATUS.instrument-server` carries `timestamp` (Unix microseconds) and
`status: true`, initially and every four seconds after status publishing starts.
It reports the hub publishing loop's state, not a fresh ISS health probe. No
shutdown `status: false` publication is implemented.

## Retired Subjects

`CAPABILITY_REQUEST` / `CAPABILITY_PAYLOAD` no longer have a hub handler. Use port
discovery plus an explicit measurement name instead. Old setup, process/chunk,
instruction, data-collector, and upload lifecycle messages in generated bindings
likewise do not represent current handler subscriptions.

See [Hub Runtime](server-interpreter.md) for routing, script annotations, ISS
transport, and current limitations. These documents describe existing behavior;
they do not introduce a new wire protocol.
