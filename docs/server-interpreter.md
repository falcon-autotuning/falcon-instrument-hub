# Server Interpreter

The **Server Interpreter** is a Go package that bridges falcon-core measurement requests to instrument commands through NATS messaging. It is the core component responsible for processing measurement requests, coordinating with instruments, and returning results.

## Overview

The server interpreter (`serverinterpreter` package) handles:

1. **Receiving measurement requests** via `PROCESS_REQUEST` NATS channel
2. **Parsing and routing requests** via `MeasurementRouter`
3. **Orchestrating complex measurements** via `MeasurementOrchestrator`
4. **Dispatching Lua scripts** to instrument-script-server
5. **Buffering trace data** via `TraceBuffer`
6. **Storing results** to HDF5/JSON database
7. **Uploading results** back to falcon via JetStream

**Important**: The hub does NOT auto-generate Lua scripts. Experimenters create their own Lua measurement scripts that run on the instrument-script-server. The hub's role is to orchestrate calls to these user-provided scripts.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Server Interpreter                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐      │
│  │ FALCON REQUEST   │───>│ Measurement      │───>│ Measurement      │      │
│  │ (NATS subscribe) │    │ Router           │    │ Orchestrator     │      │
│  └──────────────────┘    └──────────────────┘    └──────────────────┘      │
│           │                       │                       │                 │
│           │                       │              ┌────────┴────────┐        │
│           │                       │              │                 │        │
│           │                       v              v                 v        │
│  ┌──────────────────┐    ┌──────────────────┐  ┌───────┐   ┌───────┐      │
│  │ Script           │<───│ User-Provided    │  │sweep_1d│   │ramp   │      │
│  │ Dispatcher       │    │ Lua Scripts      │  │.lua   │   │.lua   │      │
│  └──────────────────┘    └──────────────────┘  └───────┘   └───────┘      │
│           │                                                                 │
│           v                                                                 │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐      │
│  │ instrument-      │───>│ TraceBuffer      │───>│ HDF5 Database    │      │
│  │ script-server    │    │ (averaging)      │    │                   │      │
│  └──────────────────┘    └──────────────────┘    └──────────────────┘      │
│           │                       │                       │                 │
│           v                       v                       v                 │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐      │
│  │ Trace Reports    │───>│ JetStream        │───>│ Falcon           │      │
│  │ (NATS)           │    │ Notification     │    │                   │      │
│  └──────────────────┘    └──────────────────┘    └──────────────────┘      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Core Components

### MeasurementRouter

Routes incoming falcon requests to the appropriate orchestration method:

```go
router := serverinterpreter.NewMeasurementRouter(orchestrator)

envelope := FalconMeasurementEnvelope{
    MeasurementID:   "measurement-001",
    MeasurementType: "measure_2D_buffered",
    Request:         requestJSON,
}

result, err := router.Route(ctx, envelope)
```

Supported measurement types:
- `measure_2D_buffered` → Orchestrated 2D sweep
- `measure_1D_buffered` → Averaged 1D sweep
- `measure_get_set` → DC get/set

### MeasurementOrchestrator

Coordinates complex measurements by calling simpler Lua scripts:

```go
orchestrator := serverinterpreter.NewMeasurementOrchestrator(executor, hubConfig)

// Execute a 2D sweep (calls sweep_1d.lua 11 times)
result, err := orchestrator.Execute2DSweep(ctx, Sweep2DRequest{
    XGate:          "P1",
    XInstrument:    "QDAC1",
    XChannel:       1,
    XStartV:        -0.5,
    XStopV:         0.5,
    XNumPoints:     101,
    YGate:          "P2",
    YInstrument:    "QDAC1",
    YChannel:       2,
    YStartV:        -0.5,
    YStopV:         0.5,
    YNumPoints:     11,
    CurrentMeter:   "DMM1",
    CurrentChannel: 0,
    SettlingTimeMs: 5.0,
    RampSlopeVPerS: 0.1,
})
```

### TraceBuffer

Accumulates traces for N-averaged measurements:

```go
buffer := serverinterpreter.NewTraceBuffer(config)

// Register expected measurement
buffer.RegisterMeasurement(
    measurementID,
    "P1",           // sweep gate
    -1.0, 0.0,      // voltage range
    101,            // points per sweep
    10,             // number of averages
    []string{"DMM1_0"},
)

// Add traces as they arrive from instrument-script-server
complete, err := buffer.AddTrace(traceReport)
if complete {
    result, err := buffer.Complete(measurementID)
    // result contains raw traces AND averaged data
}
```

### ScriptDispatcher

Communicates with instrument-script-server over HTTP:

```go
dispatcher := serverinterpreter.NewScriptDispatcher(config)

result, err := dispatcher.ExecuteScript(ctx, "sweep_1d", map[string]interface{}{
    "sweepInstrument": "QDAC1",
    "sweepChannel":    1,
    "startVoltage":    -1.0,
    "stopVoltage":     0.0,
    "numPoints":       101,
    "currentMeter":    "DMM1",
    "currentChannel":  0,
})
```

## Lua Script Requirements

The hub expects these scripts in `runtime/scripts/`:

| Script | Purpose |
|--------|---------|
| `set_voltage.lua` | Set a single gate voltage |
| `get_voltage.lua` | Read a single voltage |
| `sweep_1d.lua` | 1D voltage sweep with current measurement |
| `ramp_voltage.lua` | Smooth voltage ramping at specified rate |
| `dc_get_set.lua` | Parallel set/get operations |
| `measure_current.lua` | Current measurement with averaging |

See [LUA_SCRIPT_AUTHORING.md](LUA_SCRIPT_AUTHORING.md) for detailed requirements.

## 2D Sweep Orchestration

For a 2D voltage sweep, the orchestrator:

1. Sets static gate voltages
2. For each Y value:
   - Sets Y gate to target voltage
   - Waits for settling
   - Calls `sweep_1d.lua` for X sweep
   - Collects current vs X voltage data
   - Calls `ramp_voltage.lua` to return X to start
3. Aggregates all lines into 2D result

```
Y=YStart ─────────────────────────────────────────────────────────────
         │                                                            
         │  ┌─[set_voltage Y]──[sweep_1d X]──[ramp X]─┐              
         │  │                                         │              
Y=Y1     │  └─────────────────────────────────────────┘              
         │                                                            
         │  ┌─[set_voltage Y]──[sweep_1d X]──[ramp X]─┐              
         │  │                                         │              
Y=Y2     │  └─────────────────────────────────────────┘              
         │                                                            
         │  ... repeats for all Y values ...                          
         │                                                            
Y=YStop  ─────────────────────────────────────────────────────────────
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
