# Use DataBuffers between ISS and the hub

Use `instrument-data` for shared sample memory and gRPC for buffer IDs, metadata, and release requests. This guide describes the changes needed to finish the existing integration.

**Deployment requirement:** the hub, ISS, and producing instrument process must be able to access the same shared-memory namespace. A remote daemon's buffer ID is not enough to read its data across a network.

For the first implementation, copy samples into Go-owned memory before returning them to measurement code. Add borrowed-memory leases later only if needed.

## Files to work in

| File | Change |
| --- | --- |
| [instrumentserver/client.go](../runtime/internal/instrumentserver/client.go) | Decode typed buffer references and preserve native metadata. |
| [interpreter/dispatcher.go](../runtime/internal/interpreter/dispatcher.go) | Register returned buffers and clean up partial failures. |
| [databuffer/buffer.go](../runtime/internal/databuffer/buffer.go) | Wrap native access and validate bounds before creating slices. |
| [databuffer/buffer_management.go](../runtime/internal/databuffer/buffer_management.go) | Manage request ownership, copies, and release. |
| [ISS DataBufferManager.cpp](../../instrument-script-server/src/daemon/DataBufferManager.cpp) | Retain ISS ownership until the hub has attached; release it on request. |

## 1. Establish native ownership first

Before relying on the handoff, fix and test the lifetime issues in [instrument-data](../../instrument-data/src/manager.c):

- Balance references acquired by metadata reads and zero-copy creation.
- Keep a process registered as an owner while it still has local users.
- Remove freed buffers from the manager's lookup map.
- Unlink shared-memory objects when the final owner releases them.

The current implementation does not consistently satisfy these rules. A Go lock alone cannot fix them.

Rebuild and update the installed dependencies for all participating processes. The hub currently pins `instrument-data v1.1.8`; editing the sibling clone alone does not change that build.

## 2. Preserve buffer types in the ISS response

ISS sends a scalar buffer as a string ID with declared type `LUA_TYPES_DATA_BUFFER`. The current Go decoder turns it into a plain string, so the dispatcher misses it.

Change `measureJobResultToCallResults` to decode each value using its declared parameter type:

- A declared DataBuffer with a string payload becomes `instrumentserver.DataBuffer(id)`.
- An ordinary string stays a string.
- Buffer arrays remain `DataBufferArray`.
- An incompatible payload/type combination returns an error.

Propagate decoding errors through `Measure`. Preserve native element type, element count, byte size, and available provenance.

Keep two types distinct:

| Type | Meaning |
| --- | --- |
| Lua `DATA_BUFFER` | This return value references a buffer. |
| Native array type | The samples are float32, float64, int32, int64, uint32, uint64, or uint8. |

Replace the misleading `DataBufferMetadata.Type LuaType`. Use a separate native-type representation and validate its conversion at the buffer boundary. Do not import `databuffer` into `instrumentserver`: the existing dependency runs in the other direction. Preserve count/size without narrowing or silently overflowing.

## 3. Implement the ownership handoff

Use this order for each returned buffer:

1. The producer creates and fills it. ISS acquires ownership before the producer releases its ownership.
2. ISS returns the ID and metadata while retaining its claim.
3. The hub attaches with `data_manager_get_buffer`.
4. The hub reads and validates native metadata against the ISS result.
5. Once the hub has a valid local claim, it calls ISS `ReleaseBuffer(id)`.
6. ISS releases its claim. The hub retains its claim until its readers finish.
7. The hub copies the samples, then releases its native claim when no local consumer needs it.

Published buffers should stay unchanged during this sequence. If Lua applies offset/gain operations, finish those operations before exposing the result.

There are two different release operations:

| Operation | Ownership released |
| --- | --- |
| `ScriptServerClient.ReleaseBuffer(id)` | ISS's claim, through gRPC. |
| `DataBufferManager.ReleaseBuffer(id)` | The hub's native claim and tracking entry. |

Never ask ISS to release first and then attempt to attach.

## 4. Complete registration and validation

Extend registration to receive the expected ISS metadata, updating the dispatcher interface and tests.

Before constructing a native slice, validate:

- The buffer exists and its data pointer is valid for the declared count.
- The element type is supported and agrees with ISS metadata.
- Count × element width equals byte size, with overflow checks.
- Count fits Go's slice-length range.
- The ID and available provenance agree with the expected result.

For example, 100 float64 samples require 800 bytes. An inconsistency is an error, not permission to guess the layout.

Make duplicate registration explicit. A simple first policy is: the same requestor registering the same ID is a no-op; a different requestor is rejected. Coordinate this check with attachment so concurrent registration cannot overwrite ownership. Add shared-requestor claims later if required.

Route every cleanup through the injected `bufferLib`. Current registration error paths and `ReleaseRequestor` bypass it.

## 5. Return owned sample copies

Change `ReadBuffer` to copy the appropriate typed native slice while holding the manager's read lock and a valid native claim. Return the copy and a metadata snapshot.

For example, for a validated float64 view:

```go
// Inside ReadBuffer, while ownership and the read lock are held:
samples := append([]float64(nil), buffer.float64Slice()...)
```

Apply the same approach to every supported element type. The current implementation returns views into C memory; those must not escape if release can invalidate them.

The measurement handler then uses the owned samples to build its response. Keep physical units, dimensions, and axis labels in the measurement/result contract; the native buffer supplies a flat typed array. Do not silently convert all integer arrays to float64.

Make the copy available through the measurement execution boundary: the current dispatcher only exposes buffer registration. Add the read/release access the handler needs, and have the shared executor own cleanup.

## 6. Handle completion and failures

Release request-owned claims after copying, and on errors, cancellation, and shutdown. Install cleanup as soon as a claim is acquired.

If a later buffer fails registration, unwind earlier registrations. Also account for returned IDs that ISS still owns but the hub will not consume; release those through the agreed ISS cleanup path.

A release RPC timeout has an uncertain outcome. Define idempotent release/retry behavior and retain enough state to reconcile it. Do not treat a timeout as proof that ISS released its claim.

Once samples are copied, publication can use the Go-owned copy after native release. Shared-memory ownership is separate from durable storage.

## Completion checks

- [ ] A real scalar protobuf buffer result reaches dispatcher registration.
- [ ] Ordinary strings are not mistaken for buffers.
- [ ] Supported element types retain their values and type.
- [ ] Invalid type/count/byte-size combinations fail before slice construction.
- [ ] Copies remain valid after hub release.
- [ ] Duplicate registration and partial failure leave no untracked claims.
- [ ] Separate producer, ISS, and hub processes complete the handoff.
- [ ] Final release removes native mappings and shared-memory objects.

For supporting findings, see the [refactor review](REFACTOR_REVIEW_2026-09-25.md). For Lua arguments, follow the [CallStack guide](LUA_CALLSTACK_IMPLEMENTATION_GUIDE.md).
