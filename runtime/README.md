# Falcon Instrument Hub - Go Runtime

This directory contains the Go-based runtime for the Falcon Instrument Hub, which replaces the previous Python daemon architecture with integration to the [instrument-script-server](https://github.com/falcon-autotuning/instrument-script-server).

## Architecture Overview

The new Go runtime provides:

1. **Instrument Management**: Start, stop, and control instruments through the instrument-script-server
2. **Measurement Compilation**: Convert measurement requests into Lua scripts for execution
3. **HTTP/RPC Integration**: Communicate with instrument-script-server via its HTTP RPC API

## Directory Structure

```
runtime/
├── cmd/
│   └── main.go                          # Entry point for the runtime
├── internal/
│   ├── config/
│   │   └── config.go                    # Configuration management
│   ├── client/
│   │   └── instrument_client.go         # HTTP client for instrument-script-server
│   ├── compiler/
│   │   └── compiler.go                  # Measurement request compiler
│   └── handlers/
│       ├── instrument/
│       │   └── handler.go               # Instrument lifecycle management
│       └── measure/
│           └── handler.go               # Measurement execution
```

## Key Components

### Configuration (`config/`)

Manages runtime configuration including:
- instrument-script-server RPC endpoint (default: `http://localhost:8555`)
- instrument-script-server binary location

Environment variables:
- `INSTRUMENT_SCRIPT_SERVER_RPC_PORT`: Override RPC port (default: 8555)
- `INSTRUMENT_SERVER_HOST`: Override server host (default: localhost)
- `INSTRUMENT_SERVER_BINARY`: Path to instrument-server binary (default: instrument-server)

### Client (`client/`)

HTTP client for communicating with instrument-script-server RPC API:
- Start/stop instruments
- List instruments and their status
- Execute measurements
- Send commands to instruments

### Compiler (`compiler/`)

Compiles measurement requests into Lua scripts for instrument-script-server execution.

**TODO**: Integrate [falcon-core Go bindings](https://github.com/falcon-autotuning/falcon-core-libs/tree/main/go/falcon-core) for:
- Advanced measurement pattern generation
- Parameter validation
- Sweep and quantum measurement support
- Measurement script template selection

### Handlers

#### Instrument Handler (`handlers/instrument/`)

Manages instrument lifecycle:
- Start/stop instrument-script-server daemon at hub startup
- Start/stop individual instruments
- List active instruments
- Send commands to instruments

**Note**: API endpoints are marked with comments as they will be refactored in future versions.

#### Measure Handler (`handlers/measure/`)

Executes measurements:
- Compile measurement requests into Lua scripts
- Execute measurements via instrument-script-server
- Support for pre-written Lua scripts

**Missing Feature** (per problem statement): Measurement script selection logic. This requires:
1. Falcon-core Go bindings integration
2. Script template repository
3. Request-to-script matching algorithm

## Building

```bash
cd runtime
go mod tidy
go build -o ../bin/falcon-instrument-hub ./cmd
```

## Running

```bash
# Ensure instrument-script-server is installed and in PATH
# Or set INSTRUMENT_SERVER_BINARY environment variable

./bin/falcon-instrument-hub
```

## Integration Points

### With instrument-script-server

The runtime starts the instrument-script-server daemon at startup and communicates via HTTP RPC:

1. **Daemon Start**: `instrument-server daemon start`
2. **Start Instrument**: `POST /api/instruments/start` with config file
3. **Execute Measurement**: `POST /api/measure` with Lua script
4. **Daemon Stop**: `instrument-server daemon stop`

### With falcon-core (TODO)

Future integration with falcon-core Go bindings will:
- Replace placeholder measurement compilation
- Provide measurement script selection (currently missing)
- Support all measurement patterns from Python falcon-core
- Enable native Go measurement request processing

## Migration from Python

This Go runtime replaces:

1. **Python Daemon** (`bin/launch_instrument_daemon.py`):
   - Old: Python daemon with NATS messaging
   - New: Go runtime with HTTP RPC to instrument-script-server

2. **Server Daemons** (formerly `/src/server_daemons`):
   - Old: Python measurement compilation
   - New: Go compiler with falcon-core bindings (in progress)

The Python code in `src/instrument_server/` is retained for backward compatibility during migration.

## Future Enhancements

1. **HTTP/gRPC API Server**: Expose handlers as REST/gRPC endpoints
2. **Measurement Script Templates**: Load from falcon-core repository
3. **Script Selection**: Implement missing script selection logic
4. **Caching**: Cache compiled scripts and instrument connections
5. **Batch Operations**: Support batch measurement execution
6. **Real-time Streaming**: Stream measurement results
7. **Job Scheduling**: Integrate with instrument-script-server job scheduling

## API Endpoints (Planned)

These endpoints are commented in the code as they will be implemented in future versions:

- `POST /api/instruments/start` - Start an instrument
- `POST /api/instruments/:name/stop` - Stop an instrument
- `GET /api/instruments/list` - List all instruments
- `POST /api/measurements/execute` - Execute a measurement from request
- `POST /api/measurements/from-script` - Execute a pre-written script
- `POST /api/instruments/:name/command` - Send command to instrument

## Testing

```bash
cd runtime
go test ./...
```

## Dependencies

- Go 1.19+
- instrument-script-server (running and accessible)
- falcon-core Go bindings (TODO: add to go.mod)

## License

MIT
