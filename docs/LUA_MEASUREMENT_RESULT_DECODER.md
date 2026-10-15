# Lua Measurement Result Decoder

## Purpose

Lua measurement scripts executed by instrument-script-server (ISS) produce a
list of instrument calls. A measurement value may be returned directly or by a
shared-memory `DataBuffer`. The measurement handlers currently contain their
own code for finding those returns, consuming buffers, checking units, and
constructing arrays.

The decoder in `runtime/internal/measurementdecode` provides one small layer
for those shared operations. It does not parse Falcon requests, decide which
Lua script to run, update instrument state, or construct the final Falcon
response. Those remain handler responsibilities.

## Return layouts

The apparent return variants are combinations of transport and logical shape:

| ISS representation | Normalized values | Declared shape |
| --- | --- | --- |
| One scalar | One sample | `[1]` |
| N one-sample buffers | N samples | `[N]` |
| One N-sample buffer | N samples | `[N]` |
| One N×K-sample buffer | N×K samples | `[N, K]` |
| N buffers with K samples each | N×K samples | `[N, K]` |

ISS has three relevant transport types: a numeric scalar, `DataBuffer`, and
`DataBufferArray`. The number and size of chunks do not define their logical
meaning. For example, 20 samples could represent 20 sweep positions, one
position sampled 20 times, or 10 positions sampled twice.

The handler must therefore provide the expected shape. The decoder concatenates
chunks in ISS execution order and verifies that the number of decoded samples
equals the product of that shape.

## Decoder contract

Each `SeriesSpec` supplies:

- The instrument, group, and channel identifying the getter.
- The measurement verb, such as `GET_DATAPOINT` or `MEASURE_STREAM`.
- The named return containing the measurement.
- The unit required by the Falcon getter.
- The logical result shape.

The decoder:

1. Finds matching calls without depending on fixed call-stack indexes.
2. Reads the named return from each matching call.
3. Turns scalars and buffers into numeric chunks.
4. Converts each chunk to the requested unit.
5. Concatenates chunks in call order.
6. Rejects missing calls, duplicate named returns, invalid values, incompatible
   units, and incorrect sample counts.

Decoding a buffer consumes and releases that buffer. A handler should continue
to defer `ReleaseMeasurementBuffers` for the measurement so that buffers not
yet consumed when an error occurs are also released.

## Intended handler use

A fixed set/get measurement taking one sample per getter declares:

```go
measurementdecode.SeriesSpec{
    Target:     getterTarget,
    Verb:       "GET_DATAPOINT",
    ReturnName: "voltage",
    Unit:       getterUnit,
    Shape:      []int{1},
}
```

If set/get acquires `numPoints` samples in one buffer, its shape is
`[]int{numPoints}`.

A 1D sweep containing `numSteps` settings and `numPoints` samples at each
setting declares `[]int{numSteps, numPoints}`. A later 2D implementation can
declare `[]int{numYSteps, numXSteps, numPoints}`. Whether singleton sample axes
are retained or removed should be decided consistently when the handler builds
the Falcon response.

The first integration should replace the duplicated value-decoding portions of
`measure_set_get_handler.go` and `measure_1d_buffered_handler.go`. Request
parsing, Lua argument construction, source state updates, database recording,
and Falcon response construction should stay in those handlers.

## Measurement schema relationship

The schemas in `falcon-measurement-lib` describe script inputs and logical
returns, but they do not yet describe how ISS call results are chunked or laid
out. The current `MeasurementResponse<number>` annotation cannot distinguish a
scalar from a buffer reference or specify dimensions.

A future schema extension could generate `SeriesSpec` values:

```json
"x-resultLayout": {
  "groupBy": "getter",
  "verbs": ["GET_DATAPOINT", "MEASURE_STREAM"],
  "returnName": "voltage",
  "shape": ["numSteps", "numPoints"],
  "order": "step-major",
  "encodings": ["number", "buffer", "buffer-array"]
}
```

This is a custom schema annotation rather than ordinary JSON Schema behavior.
The measurement-lib generator could later translate it into generated Go
decoder specifications.

## Current schema and runtime differences

The newly updated schemas and the current hub examples are not yet aligned:

- `measure_get_set` no longer declares `sampleRate`, while the current example
  Lua function still accepts it.
- Its `setVoltages` property is an instrument-target map, while the current hub
  sends an ordered numeric array.
- `measure_1D_buffered` declares `setters`, `getters`, `setVoltageDomains`,
  `numPoints`, and `numSteps`, while the current handler still uses singular
  targets, an explicit voltage array, and `sampleRate`.
- Measurement return descriptions say nA even though the scripts may produce
  voltage readings.
- The schemas use `descriptions` instead of `description` for `numPoints`.

These differences should be resolved before decoder specifications are
generated directly from the schemas. The initial decoder deliberately accepts
an explicit `SeriesSpec`, allowing handler cleanup to proceed independently of
that schema work.
