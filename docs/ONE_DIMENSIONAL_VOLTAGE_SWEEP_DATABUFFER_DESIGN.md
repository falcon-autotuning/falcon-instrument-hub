# DataBuffer design for a 1D voltage sweep handler

## Simple sequential implementation

The `measure_1D_buffered` handler now accepts one identity-transformed voltage
waveform with at least two setpoints and one voltage stream getter. Its Lua
script sets each voltage in waveform order and invokes `MEASURE_STREAM` with
one bin after each set. Each stream call hands one single-sample DataBuffer to
Go; the handler copies and releases each buffer and returns one ordered 1D
Falcon measurement array. This is a sequential sweep, not a hardware-timed
source ramp. The response's sample at index `i` corresponds to setter
setpoint `i`; the response does not separately carry the setter coordinates.

The installed Lua runtime cannot allocate a DataBuffer from a table of scalar
`GET_DATAPOINT` results. Producing one DataBuffer for the entire sweep would
require a source/meter command that performs the coordinated sweep and returns
that buffer, or a new Lua buffer-construction API.

## Scope

The scalar `measure_get_set` handler should execute one `SET_VOLTAGE` followed by one scalar `GET_DATAPOINT`. It does not need a DataBuffer.

A separate 1D voltage sweep handler may use `MEASURE_STREAM` when the instruments support a hardware-coordinated sweep or buffered acquisition. In that workflow, the stream command returns a DataBuffer identifier instead of returning all samples through the normal Lua result value.

## Proposed request shape

The sweep handler should claim requests containing:

- one voltage setter waveform with multiple finite setpoints;
- one voltage stream getter;
- transforms the handler explicitly supports; and
- enough timing information to derive the acquisition sample rate.

The handler should verify that the source and meter capabilities can perform the requested sweep. Setting only the first waveform value and using the waveform length as the meter bin count does not execute a voltage sweep.

## Lua execution

The Lua measurement script should perform one coherent instrument sequence:

1. Configure the source sweep or upload the voltage waveform.
2. Configure the meter sample rate and acquisition length.
3. Arm or start the instruments in the required order.
4. Execute `MEASURE_STREAM`.
5. Return the stream DataBuffer identifier.

If the source has no hardware sweep support, a Lua loop can perform repeated set/get operations, but that requires an explicit way to accumulate or create a DataBuffer. Returning many unrelated scalar call results should be treated as a different implementation.

## Go handler flow

The handler should:

1. Resolve and serialize the setter and getter `InstrumentTarget` values.
2. Dispatch the sweep script.
3. Validate every returned command identity and the `stream` output.
4. Confirm that the returned value is an `instrumentserver.DataBuffer`.
5. Verify that the buffer belongs to the current `MeasurementResult.ID`.
6. Read and copy the numeric samples.
7. Convert the instrument result unit to the requested getter unit.
8. Construct a one-dimensional `FalconMeasurementResponse` using the actual sample count.
9. Release the DataBuffer on every success and error path after registration.

## Dispatcher boundary

Avoid exposing unrestricted `ReadBuffer` and `ReleaseBuffer` methods on `MeasurementDispatcher`. Prefer one ownership-aware operation, for example:

```go
type ConsumedMeasurementBuffer struct {
    Metadata databuffer.BufferMetadata
    Values   []float64
}

ConsumeMeasurementBuffer(
    measurementID string,
    bufferID string,
) (*ConsumedMeasurementBuffer, error)
```

The implementation should verify that `bufferID` was registered for `measurementID`, copy the samples and metadata out of shared memory, and then release the tracked buffer. This keeps ownership and cleanup inside the dispatcher/data-buffer layer while the measurement handler receives ordinary Go-owned data.

## Response and validation

The resulting Falcon array should use the getter acquisition context and a shape derived from the buffer metadata or copied sample count. The handler should reject empty buffers, unsupported element types, non-finite samples, inconsistent metadata, incompatible units, and an unexpected number or order of instrument calls.
