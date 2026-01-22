# Usage Examples

This document provides examples of how to use the Falcon Instrument Hub Go runtime.

## Basic Setup

### Starting the Runtime

The runtime automatically starts the instrument-script-server daemon on startup:

```bash
# Build and run
make run

# Or run the binary directly
./bin/falcon-instrument-hub
```

### Environment Configuration

Configure the runtime using environment variables:

```bash
# Set custom RPC port
export INSTRUMENT_SCRIPT_SERVER_RPC_PORT=9000

# Set custom server host
export INSTRUMENT_SERVER_HOST=instrumentserver.local

# Set custom binary path
export INSTRUMENT_SERVER_BINARY=/usr/local/bin/instrument-server

./bin/falcon-instrument-hub
```

## Using the Handlers

### Example: Instrument Management

```go
package main

import (
    "context"
    "log"
    
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/config"
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/handlers/instrument"
)

func main() {
    // Load configuration
    cfg, err := config.LoadConfig()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    
    // Create instrument handler
    handler := instrument.NewHandler(cfg)
    
    ctx := context.Background()
    
    // Start the daemon
    if err := handler.StartDaemon(ctx); err != nil {
        log.Fatalf("Failed to start daemon: %v", err)
    }
    defer handler.StopDaemon(ctx)
    
    // Start an instrument
    configFile := "/path/to/instrument/config.yaml"
    if err := handler.StartInstrument(ctx, configFile); err != nil {
        log.Fatalf("Failed to start instrument: %v", err)
    }
    
    // List all instruments
    instruments, err := handler.ListInstruments(ctx)
    if err != nil {
        log.Fatalf("Failed to list instruments: %v", err)
    }
    
    for _, inst := range instruments {
        log.Printf("Instrument: %s, Status: %s, PID: %d", 
            inst.Name, inst.Status, inst.PID)
    }
    
    // Send a command to an instrument
    result, err := handler.SendCommand(ctx, "DMM", "MEASURE", map[string]interface{}{
        "range": "10V",
    })
    if err != nil {
        log.Fatalf("Failed to send command: %v", err)
    }
    log.Printf("Command result: %v", result)
    
    // Stop an instrument
    if err := handler.StopInstrument(ctx, "DMM"); err != nil {
        log.Fatalf("Failed to stop instrument: %v", err)
    }
}
```

### Example: Measurement Execution

```go
package main

import (
    "context"
    "log"
    
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/client"
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/compiler"
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/config"
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/handlers/measure"
)

func main() {
    // Load configuration
    cfg, err := config.LoadConfig()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    
    // Create client and handler
    instrumentClient := client.NewInstrumentServerClient(cfg.GetRPCBaseURL())
    measureHandler := measure.NewHandler(instrumentClient)
    
    ctx := context.Background()
    
    // Create a measurement request
    req := &compiler.MeasurementRequest{
        InstrumentName: "DMM",
        Command:        "MEASURE",
        Parameters: map[string]interface{}{
            "range": "10V",
            "rate":  1000,
        },
        Globals: map[string]interface{}{
            "voltage": 5.0,
            "samples": 100,
        },
    }
    
    // Execute the measurement
    result, err := measureHandler.ExecuteMeasurement(ctx, req)
    if err != nil {
        log.Fatalf("Measurement failed: %v", err)
    }
    
    if result.Success {
        log.Printf("Measurement successful!")
        log.Printf("Results: %+v", result.Results)
    } else {
        log.Printf("Measurement failed: %s", result.Error)
    }
}
```

### Example: Execute Pre-written Lua Script

```go
package main

import (
    "context"
    "log"
    
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/client"
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/config"
    "github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/handlers/measure"
)

func main() {
    cfg, err := config.LoadConfig()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }
    
    instrumentClient := client.NewInstrumentServerClient(cfg.GetRPCBaseURL())
    measureHandler := measure.NewHandler(instrumentClient)
    
    ctx := context.Background()
    
    // Execute a pre-written Lua script
    scriptPath := "/path/to/measurement_script.lua"
    globals := map[string]interface{}{
        "voltage":    5.0,
        "sampleRate": 1000,
    }
    
    result, err := measureHandler.ExecuteMeasurementFromScript(ctx, scriptPath, globals)
    if err != nil {
        log.Fatalf("Script execution failed: %v", err)
    }
    
    log.Printf("Script result: %+v", result)
}
```

## Generated Lua Scripts

The compiler generates Lua scripts compatible with instrument-script-server's new format:

### Example Generated Script

```lua
-- Auto-generated measurement script
-- TODO: Generated by falcon-core Go bindings

function main(ctx)
    ctx:log("Starting measurement")

    local params = {
        range = "10V",
        rate = 1000,
    }

    local response = ctx:call("DMM.MEASURE", params)
    local result = response:value()
    ctx:log("Measurement complete")

    return result
end
```

## Instrument Configuration

Instruments are configured using YAML files for instrument-script-server:

### Example: `dmm_config.yaml`

```yaml
name: DMM
driver: visa
connection:
  resource: "TCPIP0::192.168.1.100::INSTR"
  timeout: 5000
commands:
  MEASURE:
    format: "MEAS:VOLT:DC?"
    response: float
  SET_RANGE:
    format: "VOLT:DC:RANG {range}"
    params:
      range: string
```

## Future Integration with falcon-core

Once falcon-core Go bindings are integrated, the compiler will support advanced features:

```go
// TODO: Future API with falcon-core integration

import "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core"

// Select appropriate measurement script based on request
scriptPath, err := compiler.SelectMeasurementScript(&compiler.MeasurementRequest{
    InstrumentName: "QDAC",
    Command:        "SWEEP_VOLTAGE",
    Parameters: map[string]interface{}{
        "start":  0.0,
        "stop":   5.0,
        "steps":  100,
        "delay":  0.01,
    },
})

// Use falcon-core to generate optimized measurement patterns
pattern, err := falconcore.GenerateSweepPattern(sweepConfig)
script := compiler.CompilePattern(pattern)
```

## HTTP API Server (Planned)

Future versions will include an HTTP/gRPC API server:

```bash
# Start the API server
./bin/falcon-instrument-hub --api-port 8080

# API Endpoints
curl -X POST http://localhost:8080/api/instruments/start \
  -H "Content-Type: application/json" \
  -d '{"config_file": "/path/to/config.yaml"}'

curl -X GET http://localhost:8080/api/instruments/list

curl -X POST http://localhost:8080/api/measurements/execute \
  -H "Content-Type: application/json" \
  -d '{
    "instrument_name": "DMM",
    "command": "MEASURE",
    "parameters": {"range": "10V"}
  }'
```

## Testing

Run tests for all components:

```bash
# Run all tests
make test

# Run specific package tests
cd runtime
go test -v ./internal/config
go test -v ./internal/compiler

# Run with coverage
go test -cover ./...
```

## Debugging

Enable verbose logging:

```go
import "log"

// In main.go
log.SetFlags(log.LstdFlags | log.Lshortfile)
log.SetOutput(os.Stdout)
```

Check instrument-script-server logs:

```bash
# View daemon logs (location depends on installation)
tail -f /var/log/instrument-script-server/daemon.log

# Or use instrument-server CLI
instrument-server daemon status
```

## Common Patterns

### Error Handling

```go
result, err := handler.ExecuteMeasurement(ctx, req)
if err != nil {
    // Check for specific error types
    if strings.Contains(err.Error(), "daemon not started") {
        // Start daemon and retry
    } else if strings.Contains(err.Error(), "connection refused") {
        // Check instrument-script-server installation
    }
    return err
}
```

### Context Timeout

```go
import "time"

// Set timeout for measurement
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

result, err := measureHandler.ExecuteMeasurement(ctx, req)
```

### Graceful Shutdown

```go
// Handle interrupt signals
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

go func() {
    <-sigChan
    log.Println("Shutting down...")
    
    // Stop instruments
    handler.StopInstrument(ctx, "DMM")
    
    // Stop daemon
    handler.StopDaemon(ctx)
    
    os.Exit(0)
}()
```
