# Complete the `set_voltage` prerequisites

Finish these tasks in order before implementing
[`setVoltageHandler`](SET_VOLTAGE_HANDLER_GUIDE.md). Each section ends with a
small completion check.

## 1. Make CallStack serialization safe

Work in the sibling `instrument-call-stack` repository first.

The current serializer allocates a `char *`, but the public API has no matching
free function. The current `sscanf` parser also rejects an empty group and
accepts text after the fourth field.

1. Add this exported function to
   `include/instrument-call-stack/instrument-call-stack.h`:

   ```c
   INSTRUMENT_CALL_STACK_EXPORT void
   instrument_call_stack_serialized_free(char *buffer);
   ```

2. Implement it with `free(buffer)` in `src/instrument-call-stack.c`.
3. Replace every test-side `free(blob)` with the new public function.
4. Replace the `sscanf` deserializer with parsing that requires exactly four
   pipe-delimited fields while preserving an empty group:

   ```text
   instrument|group|channel|command
   ```

5. Reject values containing `|`, strings longer than 63 bytes, invalid channel
   text, and extra fields. Do not silently truncate input in the public
   constructor.
6. Add tests for a normal round trip, an empty group, boundary-length strings,
   overlong strings, embedded delimiters, invalid channel text, and a trailing
   fifth field.

Run the native suite from `instrument-call-stack`:

```sh
make test PRESET=linux-clang-release
```

**Complete when:** every accepted stack round-trips without changing any field,
every rejected form returns `NULL`, and callers release serialized buffers only
through the library API.

## 2. Release and consume the same native version

The hub builds the released archive named by
`ports/instrument-call-stack/portfile.cmake`; it does not build the adjacent
clone. After the native change is reviewed:

1. Bump the library version, for example from `1.0.6` to `1.0.7`.
2. Tag and publish that version in `instrument-call-stack`.
3. Update both the hub and ISS copies of the `instrument-call-stack` vcpkg port:
   - set the new version in `vcpkg.json`;
   - update `REF`/the archive SHA512 in `portfile.cmake`;
   - raise the ISS dependency minimum from `1.0.6` to the new version.
4. Rebuild ISS and the hub from clean vcpkg build trees so both processes load
   the same serialization implementation.

During local development, a temporary overlay port may point `SOURCE_PATH` at
the clone. Do not commit an absolute developer-machine path.

**Complete when:** the header under
`vcpkg_installed/x64-linux-dynamic/include/instrument-call-stack/` contains the
new free function, and the rebuilt ISS links the matching library.

## 3. Add one Go/native adapter

Create `runtime/internal/callstack/callstack.go` with build tag
`cgo && falcon_core`. Keep all C ownership inside this package.

Use the installed pkg-config module:

```go
/*
#cgo pkg-config: instrument-call-stack
#include <stdlib.h>
#include <instrument-call-stack/instrument-call-stack.h>
*/
import "C"
```

Expose a small Go value and one operation:

```go
type Descriptor struct {
    Instrument string
    Group      string
    Channel    int
    Command    string
}

func (d Descriptor) Serialize() (string, error)
```

Inside `Serialize`:

1. Validate required fields, the 63-byte limit, delimiters, and the C `int`
   range before calling C.
2. Allocate each input with `C.CString`; import `unsafe` and release it with
   `defer C.free(unsafe.Pointer(value))`.
3. Create the native stack and `defer C.instrument_call_stack_free` it.
4. Serialize, copy the result with `C.GoString`, and release it with
   `C.instrument_call_stack_serialized_free`.
5. Return an error for every `NULL` result.

Add `callstack_test.go` with a round trip covering all four fields and table
tests for invalid input. The repository `Makefile` already supplies
`PKG_CONFIG_PATH`, linker search paths, and the runtime rpath.

**Complete when:** the adapter owns every C allocation and no handler imports
`C` directly.

## 4. Encode a scalar CallStack for ISS

In `runtime/internal/instrumentserver/client.go`, add scalar handling to
`toGrpcVariableValue`:

```go
cccase CallStack:
    return &daemonv1.VariableValue{
        Value: &daemonv1.VariableValue_S{S: string(value)},
    }, nil
```

The protobuf payload is a string. Its separate manifest entry must remain
`LUA_TYPES_CALL_STACK`; `VariableValue.LuaType()` already provides that type.

Add tests that prove:

- `CallStack("serialized-stack")` becomes `VariableValue_S`;
- `buildMeasureJobRequest` writes the same string to `Globals`;
- the matching manifest parameter is `LUA_TYPES_CALL_STACK`;
- an ordinary Go `string` still becomes `LUA_TYPES_STRING`.

Use canonical values in handler inputs: `float64` for voltage and `CallStack`
for the stack. This avoids the existing mismatch where the protobuf converter
accepts some numeric types that `LuaType()` does not.

**Complete when:** a client request contains the serialized string and the
CallStack manifest type together.

## 5. Give production code a live ISS client

Fix the lifecycle in `runtime/cmd/main.go` before registering a measurement
handler. Currently `NewRuntime` starts ISS only when `AutoStart` is false and
leaves `services.issClient` nil on the default path.

Keep the existing two modes, but make their behavior consistent:

| Settings | Startup behavior | Shutdown behavior |
| --- | --- | --- |
| `autostart: true` | Start daemon, wait for readiness, save client, start configured instruments | Stop owned instruments/daemon, close client |
| `autostart: false` or `--no-iss` | Connect to the existing daemon, wait for readiness, save client | Close client only |

Treat `--no-iss` as “do not manage the ISS process,” as the existing docs state;
measurement dispatch still needs a client. Require `ISSBinary` only in autostart
mode. In both modes, create the handler manager only after assigning a non-nil
client.

Update command tests for both paths. Assert that the dispatcher received by
`newHandlerManager` is non-nil and that an externally managed daemon is never
stopped by `Runtime.Close`.

**Complete when:** the default runtime starts and connects to ISS, external mode
attaches without claiming ownership, and neither mode can pass a nil measurement
client.

## 6. Supply instrument API paths

Add the missing field to `HubConfig`:

```go
InstrumentAPIPaths []string `yaml:"instrument-apis"`
```

Use a YAML list in the hub config:

```yaml
instrument-apis:
  - /absolute/path/generated-source-api.yml
  - /absolute/path/generated-multimeter-api.yml
```

Then:

1. Validate that the list is non-empty and every path is a regular file.
2. Optionally add a repeatable `--instrument-api` CLI override. When present,
   replace the configured list rather than appending to it.
3. Pass `cfg.InstrumentAPIPaths` to `deps.newHandlerManager` in place of the
   hard-coded `[]string{}`.
4. Extend `TestLoadConfig_FullConfig`, validation tests, CLI override tests, and
   `TestNewRuntime_HappyPath` to check the exact paths passed to the handler
   manager.

Do not read `inst-apis` from `test_data/test-config.yaml` as if it were already
part of `HubConfig`; that fixture uses an older schema and semicolon-separated
values. Convert active fixtures to `instrument-apis` with a YAML list.

**Complete when:** `ports.NewConnectedPorts` receives real API files and can
resolve one voltage output from the wiremap.

## Final prerequisite check

Run the narrow suites first:

```sh
make test-instrumentserver
make test-cmd
make test-ports
```

Then run the full build once the known refactor compilation failures are fixed:

```sh
make test
```

The prerequisites are finished when a production-created handler manager gets
a non-nil ISS client, valid instrument API paths, and a scalar CallStack that ISS
can deserialize into the same four fields.
