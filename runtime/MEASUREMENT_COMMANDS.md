# Measurement Commands and NATS Integration

This document describes the measurement command system that uses NATS messaging and the falcon-measurement-lib types.

## Overview

The measurement command system allows the compiler (or any other component) to issue measurement commands that are executed by the instrument-script-server. This system uses:

1. **Generated Types**: Go structs from falcon-measurement-lib v1.0.0
2. **NATS Messaging**: For communication between components
3. **Lua Scripts**: User measurement scripts executed by instrument-script-server
4. **Data Collection**: Optional storage in HDF5 database

## Architecture

```
┌─────────────┐
│  Compiler   │ (Future implementation)
│  or Client  │
└──────┬──────┘
       │ Publishes MeasurementCommand
       │ to "measurement.command"
       ▼
┌─────────────────────┐
│  NATS Message Bus   │
└──────┬──────────────┘
       │
       ▼
┌────────────────────────┐
│  CommandHandler        │
│  - Receives commands   │
│  - Selects Lua script  │
│  - Calls instrument-   │
│    script-server       │
└──────┬─────────────────┘
       │
       ▼
┌──────────────────────────┐
│  instrument-script-server│
│  - Executes Lua script   │
│  - Returns results       │
└──────┬───────────────────┘
       │
       ▼
┌────────────────────────┐
│  Response published to │
│  measurement.response. │
│  {request_id}          │
└────────────────────────┘
       │
       ▼
┌────────────────────────┐
│  DataCollector (opt)   │
│  - Stores in HDF5      │
└────────────────────────┘
```

## Configuration

The system requires a configuration file (JSON format) matching the schema in `runtime/config-schema.json`:

```json
{
  "wiremap": "/path/to/wiremap.json",
  "quantum-dot-config": "/path/to/quantum_dot_config.json",
  "inst-config": "/path/to/instrument_configs/",
  "teal-apis": "/path/to/teal_apis/",
  "lua-library-types": "/path/to/lua_libraries/",
  "user-measurement-luas": "/path/to/measurement_scripts/",
  "local-database": "/path/to/hdf5_database/",
  "nats-url": "nats://localhost:4222",
  "instrument-server-port": 8555
}
```

Load configuration:

```go
cfg, err := config.LoadConfigFromFile("config.json")
```

## Measurement Types

All measurement types are defined in `runtime/internal/measurements/types.go`, generated from falcon-measurement-lib v1.0.0.

Available measurements:
- `set_voltage` / `get_voltage`
- `set_many_voltages` / `get_many_voltages` / `get_all_voltages`
- `measure_1D_buffered` / `measure_2D_buffered`
- `measure_current` / `measure_get_set` / `measure_illumination` / `measure_leakage`
- `ramp`
- `set_slope` / `get_slope`
- `set_sample_rate` / `get_sample_rate`
- `set_number_of_samples` / `get_number_of_samples`
- `set_trigger_leader` / `get_trigger_leader`

Each measurement type has:
- **Request struct**: Input parameters (e.g., `SetVoltageRequest`)
- **Response struct**: Output results (e.g., `SetVoltageResponse`)
- **Spec struct**: Pairs request and response (e.g., `SetVoltageSpec`)

## NATS Message Format

### Command Message

Published to: `measurement.command`

```json
{
  "type": "set_voltage",
  "input": {
    "setter": {
      "id": "DAC1",
      "channel": 1
    },
    "setVoltage": 2.5
  },
  "request_id": "unique-request-id"
}
```

### Response Message

Published to: `measurement.response.{request_id}`

Success:
```json
{
  "request_id": "unique-request-id",
  "success": true,
  "output": {
    // Measurement-specific output
  }
}
```

Error:
```json
{
  "request_id": "unique-request-id",
  "success": false,
  "error": "Error message"
}
```

## Usage Examples

### Starting the Command Handler

```go
import (
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/measurements"
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/client"
)

// Create instrument client
instrumentClient := client.NewInstrumentServerClient("http://localhost:8555")

// Create command handler
handler, err := measurements.NewCommandHandler(
    "nats://localhost:4222",
    instrumentClient,
    "/path/to/user-measurement-luas",
)
if err != nil {
    log.Fatal(err)
}
defer handler.Close()

// Start listening for commands
ctx := context.Background()
go handler.StartListening(ctx)
```

### Sending a Measurement Command (via NATS)

```go
import (
    "github.com/nats-io/nats.go"
    "encoding/json"
)

nc, _ := nats.Connect("nats://localhost:4222")

// Create a set_voltage command
cmd := measurements.MeasurementCommand{
    Type:      "set_voltage",
    RequestID: "req-123",
    Input:     json.RawMessage(`{"setter":{"id":"DAC1","channel":1},"setVoltage":2.5}`),
}

cmdData, _ := json.Marshal(cmd)
nc.Publish("measurement.command", cmdData)

// Subscribe to response
sub, _ := nc.SubscribeSync("measurement.response.req-123")
msg, _ := sub.NextMsg(5 * time.Second)

var response measurements.MeasurementResponse
json.Unmarshal(msg.Data, &response)

if response.Success {
    fmt.Println("Measurement succeeded:", string(response.Output))
} else {
    fmt.Println("Measurement failed:", response.Error)
}
```

### Using Type-Safe Helpers

```go
import "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/measurements"

// Set voltage with type safety
req := &measurements.SetVoltageRequest{
    Setter: measurements.InstrumentTarget{
        Id:      "DAC1",
        Channel: 1,
    },
    SetVoltage: 2.5,
}

response, err := measurements.ExecuteSetVoltage(ctx, handler, req, "req-123")
if err != nil {
    log.Fatal(err)
}

// Measure 1D buffered
measureReq := &measurements.Measure1DBufferedRequest{
    SampleRate: 10000,
    NumSteps:   100,
    NumPoints:  1000,
    BufferedGetters: []measurements.InstrumentTarget{
        {Id: "DMM1", Channel: 0},
    },
    BufferedSetters: []measurements.InstrumentTarget{
        {Id: "DAC1", Channel: 1},
    },
    SetVoltageDomains: map[string]measurements.Domain{
        "DAC1:1": {Min: 0.0, Max: 5.0},
    },
}

measureResponse, err := measurements.ExecuteMeasure1DBuffered(ctx, handler, measureReq, "req-124")
if err != nil {
    log.Fatal(err)
}

// Process buffered results
for _, result := range *measureResponse {
    fmt.Printf("Instrument: %s, Value: %f\n", result.Instrument, result.Value)
}
```

## Lua Script Selection

The `CommandHandler` maintains a mapping from measurement type to Lua script filename:

```go
scriptNameMapping: map[string]string{
    "set_voltage": "set_voltage.lua",
    "measure_1D_buffered": "measure_1D_buffered.lua",
    // ... etc
}
```

When a command is received:
1. Handler looks up the script filename
2. Constructs full path: `{user-measurement-luas}/{script}.lua`
3. Passes input as globals to instrument-script-server
4. Executes the script
5. Returns the output

## Data Collection

Measurements can optionally be stored in an HDF5 database.

### Interface

```go
type DataCollector interface {
    StoreMeasurement(measurement *MeasurementData) error
    Close() error
}
```

### Implementation Options

1. **HDF5Collector** (stub): Direct HDF5 storage
   ```go
   collector, _ := database.NewHDF5Collector("/path/to/database")
   ```

2. **JSONCollector** (stub): Simple JSON file storage
   ```go
   collector, _ := database.NewJSONCollector("/path/to/database")
   ```

3. **Standalone Service** (future): Separate microservice with REST/gRPC API

### Storage Integration

To store measurements automatically:

```go
// In CommandHandler.handleCommand, after successful measurement:
if h.dataCollector != nil {
    measurementData := &database.MeasurementData{
        Timestamp:       time.Now(),
        MeasurementType: cmd.Type,
        RequestID:       cmd.RequestID,
        Input:           cmd.Input,
        Output:          result,
        Success:         true,
    }
    
    if err := h.dataCollector.StoreMeasurement(measurementData); err != nil {
        log.Printf("Failed to store measurement: %v", err)
    }
}
```

## Compiler Integration (Future)

The compiler will:

1. **Analyze** measurement requirements from higher-level description
2. **Select** appropriate measurement type from falcon-measurement-lib
3. **Fill** the corresponding Go struct with parameters
4. **Map** named connections to physical instruments using wiremap
5. **Validate** parameters against quantum-dot-config
6. **Publish** MeasurementCommand to NATS

Example compiler workflow:

```go
// Compiler receives high-level request
type CompilerRequest struct {
    Operation string // e.g., "sweep_voltage"
    Target    string // e.g., "plunger_gate"
    // ... other parameters
}

// Compiler maps to measurement type
func (c *Compiler) Compile(req *CompilerRequest) (*measurements.MeasurementCommand, error) {
    // 1. Determine measurement type
    measurementType := c.selectMeasurementType(req.Operation)
    
    // 2. Map named targets to physical instruments
    physicalTarget := c.wiremap.Resolve(req.Target)
    
    // 3. Create appropriate request struct
    var input interface{}
    switch measurementType {
    case "set_voltage":
        input = &measurements.SetVoltageRequest{
            Setter:     physicalTarget,
            SetVoltage: req.Voltage,
        }
    // ... other cases
    }
    
    // 4. Marshal to JSON
    inputJSON, _ := json.Marshal(input)
    
    // 5. Create command
    return &measurements.MeasurementCommand{
        Type:      measurementType,
        Input:     inputJSON,
        RequestID: generateRequestID(),
    }, nil
}
```

## Extending with New Measurements

To add a new measurement type:

1. **Update falcon-measurement-lib**:
   - Add JSON schema in `schemas/scripts/`
   - Generate new release
   
2. **Update instrument hub**:
   - Download new `go-types-{version}.tar.gz`
   - Replace `runtime/internal/measurements/types.go`
   - Add script mapping in `CommandHandler.scriptNameMapping`
   - Add typed helper function (optional)

3. **Add Lua script**:
   - Place in `user-measurement-luas` directory
   - Ensure script accepts globals matching the schema

## Testing

Run unit tests:
```bash
cd runtime
go test ./internal/measurements
go test ./internal/database
```

Integration test with NATS:
```bash
# Start NATS server
docker run -d -p 4222:4222 nats:latest

# Run handler
go run cmd/main.go -config config.json

# Send test command (in another terminal)
go run test/send_measurement_command.go
```

## Future Enhancements

1. **HDF5 Implementation**: Complete HDF5 storage with Go bindings
2. **Query API**: Retrieve stored measurements
3. **Streaming**: Real-time measurement data streaming
4. **Batch Operations**: Execute multiple measurements atomically
5. **Priority Queue**: Prioritize urgent measurements
6. **Result Caching**: Cache recent measurement results
7. **Monitoring**: Metrics and dashboards for measurement throughput
8. **Authentication**: Secure NATS connections with TLS/JWT

## References

- [falcon-measurement-lib](https://github.com/falcon-autotuning/falcon-measurement-lib)
- [NATS Documentation](https://docs.nats.io/)
- [instrument-script-server](https://github.com/falcon-autotuning/instrument-script-server)
- Configuration schema: `runtime/config-schema.json`
- Generated types: `runtime/internal/measurements/types.go`
