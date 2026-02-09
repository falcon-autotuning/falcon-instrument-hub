# Server Interpreter

The **Server Interpreter** is a Go package that bridges falcon-core measurement requests to instrument commands through NATS messaging. It is the core component responsible for processing measurement requests, coordinating with instruments, and returning results.

## Overview

The server interpreter (`serverinterpreter` package) handles:

1. **Receiving measurement requests** via `PROCESS_REQUEST` NATS channel
2. **Breaking down requests** into chunked instructions for instruments
3. **Coordinating with instruments** via `MEASUREMENT_READY` and other signals
4. **Collecting measurement data** from `PROCESS_DATA` responses
5. **Uploading results** back to falcon via `UPLOAD_DATA`

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Server Interpreter                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐      │
│  │ PROCESS_REQUEST  │───>│ WaveformProcessor │───>│ DataCollector    │      │
│  │ (subscribe)      │    │                   │    │                   │      │
│  └──────────────────┘    └──────────────────┘    └──────────────────┘      │
│           │                       │                       │                 │
│           v                       v                       v                 │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐      │
│  │ Instructions     │───>│ MEASUREMENT_READY │───>│ UPLOAD_DATA      │      │
│  │ (chunking)       │    │ (publish)        │    │ (publish)        │      │
│  └──────────────────┘    └──────────────────┘    └──────────────────┘      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Operation Modes

### 1. Daemon Mode (Recommended)

The `InterpreterDaemon` provides full measurement processing via NATS:

```go
import "falcon-instrument-hub/runtime/internal/serverinterpreter"

func main() {
    config := serverinterpreter.DefaultInterpreterConfig()
    config.NATSUrl = "nats://localhost:4222"
    config.Debug = true
    
    daemon := serverinterpreter.NewInterpreterDaemon(config)
    
    if err := daemon.Start(); err != nil {
        log.Fatal(err)
    }
    defer daemon.Stop()
    
    // Daemon runs until stopped...
    daemon.Run() // Blocks until context cancelled
}
```

### 2. Direct Mode (HTTP RPC)

The `Bridge` communicates directly with instrument-script-server:

```go
config := serverinterpreter.BridgeConfig{
    ScriptServerHost: "127.0.0.1",
    ScriptServerPort: 8555,
    ScriptOutputDir:  "/tmp/falcon-scripts",
}

bridge, err := serverinterpreter.NewBridge(config)
if err != nil {
    log.Fatal(err)
}

result, err := bridge.ExecuteSetVoltage("DAC1", 0, 1.5)
```

## NATS Channel Protocol

Message schemas are defined in `falcon-api/embedded/commands/v1/`.

### Channels Owned by Server Interpreter

| Channel | Direction | Description |
|---------|-----------|-------------|
| `LOG` | Publish | Logging messages |
| `PROCESS_REQUEST` | Subscribe | Receive measurement requests |
| `PROCESS_DATA` | Subscribe | Receive collected data chunks |
| `MEASUREMENT_READY` | Publish | Signal measurement is ready |
| `UPDATE_DAEMON_PROPERTY` | Publish | Update instrument properties |
| `UPLOAD_DATA` | Publish | Send results back to falcon |
| `STATUS` | Publish | Daemon status updates |

### Instrument Coordination Channels

| Channel | Direction | Description |
|---------|-----------|-------------|
| `SET` | Publish | Execute set instruction |
| `GET` | Publish | Execute get instruction |
| `TRIGGER` | Publish | Trigger buffered instruments |
| `ARMED` | Subscribe | Instrument armed notification |
| `EXECUTING` | Subscribe | Instrument executing notification |
| `RETURN_DATA` | Subscribe | Measurement data response |
| `RETURN_GET` | Subscribe | Get operation response |

## Message Types

### ProcessRequestMessage

```go
type ProcessRequestMessage struct {
    ProcessID      int64       `json:"process_id"`
    Request        interface{} `json:"request"`        // MeasurementRequest
    Configurations interface{} `json:"configurations"` // Instrument configs
    DataPath       string      `json:"data_path"`
}
```

### MeasurementReadyMessage

```go
type MeasurementReadyMessage struct {
    Timestamp    int64    `json:"timestamp"`
    Getters      []string `json:"getters"`       // InstrumentPort JSONs
    Setters      []string `json:"setters"`       // InstrumentPort JSONs
    Requirements []string `json:"requirements"`
    HasSet       bool     `json:"has_set"`
    HasTrigger   bool     `json:"has_trigger"`
    IsBuffered   bool     `json:"is_buffered"`
    ProcessID    int64    `json:"process_id"`
    ChunkID      int64    `json:"chunk_id"`
}
```

### UploadDataMessage

```go
type UploadDataMessage struct {
    Timestamp int64  `json:"timestamp"`
    ProcessID int64  `json:"process_id"`
    UnitHash  int64  `json:"unit_hash"`
    Channel   string `json:"channel"`  // NATS channel for data
    Stream    string `json:"stream"`   // JetStream stream name
}
```

## Processing Pipeline

1. **Receive Request**: `PROCESS_REQUEST` message arrives with MeasurementRequest
2. **Parse Configurations**: Extract instrument configurations
3. **Process Waveform**: Break down into chunked instructions via `WaveformProcessor`
4. **Register Measurement**: Track expected data chunks in `DataCollector`
5. **Deploy Instructions**: Send `UPDATE_DAEMON_PROPERTY` and `MEASUREMENT_READY` for each chunk
6. **Collect Data**: Receive `PROCESS_DATA` messages and accumulate
7. **Complete**: When all chunks received, process and upload via `UPLOAD_DATA`

## Waveform Processing

The `WaveformProcessor` breaks complex waveforms into executable instructions:

- **Chunking**: Splits raw time traces at staircase boundaries
- **Buffered vs Unbuffered**: Decides based on instrument capabilities
- **Ramp Interjection**: Adds ramp instructions between buffered measurements
- **Property Generation**: Creates `voltage_state`, `staircase`, `timeout`, etc.

## Data Collection

The `DataCollector` handles async data using Go channels:

```go
collector := serverinterpreter.NewDataCollector(config)
collector.Start()

// Register measurement
collector.RegisterMeasurement(processID, expectedChunks, dataPath, shape, requestJSON)

// Queue incoming data
collector.QueueData(&DataEntry{
    MeasurementID: processID,
    ChunkID:       chunkID,
    Data:          parsedData,
})

// Completion callback fires when all chunks received
```

## Configuration

### InterpreterConfig

```go
type InterpreterConfig struct {
    NATSUrl               string        // Default: "nats://localhost:4222"
    Debug                 bool          // Default: true
    StatusRefreshInterval time.Duration // Default: 500ms
}
```

### BridgeConfig

```go
type BridgeConfig struct {
    ScriptServerHost string // Default: "127.0.0.1"
    ScriptServerPort int    // Default: 8555
    ScriptOutputDir  string // Default: "/tmp/falcon-scripts"
}
```

## falcon-core Integration

The package supports two modes:

1. **With falcon-core** (build tag `falcon_core`): Uses C library for proper MeasurementRequest parsing
2. **Without falcon-core**: Uses pure-Go JSON parsing for testing

Build with falcon-core:
```bash
go build -tags falcon_core ./...
```

## File Structure

```
internal/serverinterpreter/
├── channels.go          # NATS channel definitions and message types
├── interpreter_daemon.go # Main daemon implementation
├── data_collector.go    # Async data collection
├── waveform_processor.go # Waveform chunking logic
├── instructions.go      # Instruction types
├── types.go             # Core data structures
├── bridge.go            # Direct mode orchestration
├── client.go            # HTTP RPC client
├── generator.go         # Lua script generation
├── nats_handler.go      # NATS handler utilities
├── set_voltage_handler.go # Set voltage command handling
├── falcon_core.go       # falcon-core CGO integration
├── falcon_core_stub.go  # Pure-Go fallback
└── doc.go               # Package documentation
```

## Error Handling

Errors are returned at key points:
- JSON parsing failures
- Script generation failures
- NATS communication failures
- Job execution failures

`ExecutionResult` includes status and error fields:

```go
type ExecutionResult struct {
    JobID     string `json:"job_id"`
    Status    string `json:"status"`    // "completed", "failed", "timeout"
    Error     string `json:"error,omitempty"`
    Results   []MeasurementResponse
    RawResult map[string]interface{}
}
```

## See Also

- [falcon-api](../../../falcon-api/README.md) - NATS channel specifications
- [falcon-measurement-lib](../../../falcon-measurement-lib/README.md) - Message schemas
- [instrument-script-server](../../../instrument-script-server/README.md) - Lua script execution
