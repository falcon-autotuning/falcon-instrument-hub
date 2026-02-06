# Server Interpreter Package

The `serverinterpreter` package provides the server interpreter daemon for falcon measurements.

## Overview

This package bridges falcon-core MeasurementRequest objects to instrument commands, coordinating through NATS messaging. Message types are aligned with `falcon-api/embedded/commands/v1/` specifications.

## Package Structure

| File | Description |
|------|-------------|
| `channels.go` | NATS channel definitions and message types |
| `interpreter_daemon.go` | Main daemon implementation |
| `data_collector.go` | Async data collection using Go channels |
| `waveform_processor.go` | Waveform chunking and instruction generation |
| `instructions.go` | Measurement instruction types |
| `types.go` | Core data structures |
| `bridge.go` | Direct mode orchestration with HTTP RPC |
| `client.go` | HTTP RPC client for instrument-script-server |
| `generator.go` | Lua script template generation |
| `nats_handler.go` | NATS handler utilities |
| `set_voltage_handler.go` | Set voltage command handling |
| `falcon_core.go` | falcon-core CGO integration (build tag: falcon_core) |
| `falcon_core_stub.go` | Pure-Go fallback for testing |
| `doc.go` | Package documentation |

## Quick Start

### Daemon Mode

```go
package main

import (
    "log"
    "falcon-instrument-hub/runtime/internal/serverinterpreter"
)

func main() {
    config := serverinterpreter.DefaultInterpreterConfig()
    daemon := serverinterpreter.NewInterpreterDaemon(config)
    
    if err := daemon.Run(); err != nil {
        log.Fatal(err)
    }
}
```

### Direct Mode

```go
config := serverinterpreter.BridgeConfig{
    ScriptServerHost: "127.0.0.1",
    ScriptServerPort: 8555,
}

bridge, _ := serverinterpreter.NewBridge(config)
result, _ := bridge.ExecuteSetVoltage("DAC1", 0, 1.5)
```

## Building

Standard build (pure-Go, for testing):
```bash
go build ./internal/serverinterpreter/...
```

With falcon-core integration:
```bash
go build -tags falcon_core ./internal/serverinterpreter/...
```

## Testing

```bash
go test ./internal/serverinterpreter/... -v
```

## NATS Channels

Channels owned by server-interpreter:
- `PROCESS_REQUEST` - Receive measurement requests
- `PROCESS_DATA` - Receive collected data
- `MEASUREMENT_READY` - Signal measurement ready
- `UPLOAD_DATA` - Upload results
- `UPDATE_DAEMON_PROPERTY` - Update instrument properties
- `LOG` - Logging
- `STATUS` - Status heartbeat

## Documentation

- [Server Interpreter Guide](../../docs/docs/server-interpreter.md)
- [NATS Protocol Reference](../../docs/docs/nats-protocol.md)
- [falcon-api specs](https://github.com/falcon-autotuning/falcon-api)
