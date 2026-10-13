# 1D measurement handlers: build fixes and hardware observations

## Scope of this update

This note records the review of the sequential 1D prototype and the developing
QDAC and mioDAQ ISAs. It distinguishes compilation fixes from hardware support.
It supersedes implementation-status claims in
`ONE_DIMENSIONAL_VOLTAGE_SWEEP_DATABUFFER_DESIGN.md`; that document also contains
earlier proposals, including a buffer-consumption API that is no longer present.

The current update restores the existing sequential prototype. It does not
implement QDAC/mioDAQ command mapping or hardware-triggered acquisition, change
the client/dispatcher/buffer manager, or change the Lua execution sequence.

## Three cases, two execution modes

| Case | Execution | Returned data |
| --- | --- | --- |
| Sequential scalar | Set each waveform point and get one numeric value | One float64/int64 per point |
| Sequential buffer | Set each waveform point and get one single-sample buffer | One DataBuffer of length 1 per point |
| Single-get buffered sweep | Configure source and acquisition, arm, trigger, wait, then get the trace | One DataBuffer containing the sweep |

The first two cases can share a sequential handler and differ in return-value
decoding. The third needs a separate execution sequence selected before any
instrument commands run. Buffer length cannot determine which commands should
already have been executed.

`measure_1d_single_get_buffer_handler.go` reserves the third workflow. By explicit
agreement it remains unregistered, its `CanHandle` returns false, and its `Handle`
returns a not-implemented error. No hardware commands or fake results are emitted.

## Minimum build repairs implemented

1. Share waveform validation between scalar and 1D parsing. The shared parser
   accepts a nonempty, one-dimensional, identity-transformed voltage waveform,
   converts every finite setpoint to volts, and retains the existing voltage
   setter/getter restrictions. The scalar wrapper requires exactly one point;
   the 1D wrapper requires at least two. Owned Falcon handles are closed when a
   wrapper rejects a request.
2. Replace the nonexistent `ConsumeMeasurementBuffer` with
   `ConsumeMeasurementSamples`. The existing dispatcher/buffer manager checks
   measurement ownership, copies numeric samples, and releases the buffer. The
   old buffer-metadata check is removed because the current API returns only
   samples; command identity is still checked separately. Leftover registered
   buffers are released on error paths.
3. Share an array response builder with shape `[len(values)]`; the scalar
   response wraps its value in an array of length one.
4. Add `StateUpdater` to the sequential handler and record the final applied
   setpoint, in volts, after successful execution and result validation.
5. Restore the `cgo` build constraint and register the sequential handler before
   the scalar handler, whose `CanHandle` remains a broad structural match.

The sequential result decoder accepts float64, int64, or a DataBuffer containing
exactly one sample. It checks finite values and compatible units. It rejects
multi-element buffers instead of averaging distinct sweep points. Single-sample
buffers do not require the shared sample-rate/bin presence check.

### Limits of the restored prototype

The existing `measure_1D_buffered.lua` is unchanged: it calls `SET_SAMPLE_RATE`,
`SET_BINS(1)`, and then `SET_VOLTAGE` / `MEASURE_STREAM` for every setpoint. The
Go handler still requires `.voltage` / `.stream` ports, the `stream` output, and
that command sequence. Numeric return decoding does not itself add a
`GET_DATAPOINT` execution path; selecting a scalar getter command is follow-up
work. The sample-rate estimate from the request time domain does not make this
software loop a hardware-timed sweep.

This build repair does not add HDF5/SQL persistence to the sequential prototype;
the existing scalar handler's raw-data persistence and averaging are unchanged.
State updates on partial execution failure also remain follow-up work: a failed
sweep may already have changed the physical output.

## Instrument ISA observations

Reviewed sources, relative to this document:

- [QDAC ISA](../../qdac-instrument/isa/qdac.yml.tmpl)
- [mioDAQ ISA](../../mioDAQ6451/isa/MIODAQ6451.yml.tmpl)
- [mioDAQ plugin](../../mioDAQ6451/plugin/plugin.c)

These are developing interfaces, not evidence that a hardware sequence has been
validated experimentally.

### Sequential scalar measurements

QDAC exposes `SET_VOLTAGE_LOW/HIGH`, scalar `GET_CURRENT_LOW/HIGH`, and voltage
readback commands. These do not match the prototype's mock command names or
fixed IO suffixes. Voltage readback should not be confused with an independent
measurement of the device under test.

QDAC current-sensing IO is currently marked `role: output`. The hub maps that to
a writable knob, not a meter. Current getter support requires reviewing that
declaration and the handler's voltage-only getter restriction.

mioDAQ exposes a buffer-valued `READ_TRACE`, not a direct scalar measurement
getter.

### Sequential single-sample buffers

mioDAQ provides these building blocks:

1. `CONFIGURE_MEASUREMENT(sample_rate, num_points)` with `num_points = 1`.
2. Apply the source voltage and allow any required settling.
3. `FIRE_TRIGGER`.
4. Poll `CHECK_BUFFERED_TRACES_EXIST` with a bounded timeout.
5. `READ_TRACE` on the selected `ai` channel, returning the `voltage` buffer.

The plugin runs acquisition asynchronously. Reading immediately after triggering
can fail with acquisition incomplete.

Buffer lifetime needs attention before putting repeated acquisitions into one
Lua call: the plugin releases previous buffers at the next acquisition, while
the hub registers returned buffer IDs after the Lua measurement returns. This
requires verification that ISS independently retains the earlier buffers. A
conservative initial implementation would return to the hub and consume each
point before starting the next acquisition. This behavior is not added by the
compilation repair.

### Single-get hardware-triggered sweep

The QDAC ISA has no literal start/stop/step command. It provides:

- `SET_STAIR`: step duration, number of steps, repetitions, and trigger number.
- `SET_WAVEFORM_LOW/HIGH`: generator attachment, amplitude, and offset.
- `SET_SYNC` and `TRIGGER`: synchronization and execution controls.

A proposed sequence is:

```text
Configure QDAC staircase and output
  -> Configure mioDAQ sample rate and sample count
  -> Arm mioDAQ and confirm readiness
  -> Trigger QDAC with a physical synchronization signal to mioDAQ
  -> Wait for acquisition completion
  -> Read, copy, and release the trace buffer
```

This needs verified amplitude/offset scaling, endpoint conventions, trigger
wiring, settling time, and sample-to-setpoint alignment. Only suitable uniformly
spaced waveforms can be represented as start/stop/step sweeps. Arbitrary waveform
upload is not currently exposed: QDAC's proposed `SET_AWG_DATA` is commented out.

The mioDAQ plugin's `ARM_EXT_TRIGGER` queues a worker and returns before that
worker calls `DAQmxStartTask`. Command return is therefore not an armed-ready
acknowledgement. A readiness guarantee is needed before firing the source's
trigger. Completion polling is needed afterward as well.

## Other compatibility gaps

- The shared measurement validator requires channel-local `sample_rate` and
  `bins`. mioDAQ defines global `sample_rate` and `num_points`, and no `bins`.
  The hub's connected-port view is channel-scoped, so merely allowing another
  setting name will not solve global-setting discovery. Validation must reflect
  the real instrument's settings and their scope. `num_points` is not automatically
  an averaging-bin definition.
- Both ISA templates use textual `instrument.model` values, while the hub's
  `APIInstrument.Model` is currently an `int`. Direct loading needs a schema
  compatibility decision.
- A full length-N sweep trace must not be averaged to one value. If multiple
  samples represent each setter point, grouping and settling exclusions must be
  explicit. Preserve raw samples in HDF5; return the per-point averages only.
- A single-get handler needs an explicit selection rule and source/acquisition
  configuration. Do not infer these from the generic waveform or buffer type.

No tests were run as part of this update.
