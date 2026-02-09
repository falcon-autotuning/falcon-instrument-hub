# NATS Protocol Reference

This document describes the NATS messaging protocol used by the Server Interpreter component of falcon-instrument-hub. Message schemas are aligned with `falcon-api/embedded/commands/v1/` specifications.

## Overview

The Server Interpreter uses NATS for:
- Receiving measurement requests from falcon
- Coordinating with instrument daemons
- Uploading measurement results

JetStream is used for large data transfers to ensure reliability and persistence.

## Channel Naming Convention

Channels follow the pattern: `{CHANNEL_NAME}.{suffix}`

Examples:
- `LOG.interpreter` - Log messages from interpreter
- `PROCESS_REQUEST` - Measurement request channel
- `measurement.data.{process_id}` - JetStream data channel

## Message Schemas

### LOG

Logging messages from components.

```yaml
# falcon-api/embedded/commands/v1/log.yaml
channel: LOG
parameters:
  hash: int (optional)     # Process identifier
  message: string          # Log message text
  timestamp: int           # Unix timestamp
```

**Go Type:**
```go
type LogMessage struct {
    Hash      int64  `json:"hash,omitempty"`
    Message   string `json:"message"`
    Timestamp int64  `json:"timestamp"`
}
```

### PROCESS_REQUEST

Request to process a measurement.

```yaml
# falcon-api/embedded/commands/v1/process_request.yaml
channel: PROCESS_REQUEST
parameters:
  process_id: int          # Unique process identifier
  request: jsonable        # MeasurementRequest object
  configurations: json     # Instrument configurations
  data_path: string        # Path to store data
```

**Go Type:**
```go
type ProcessRequestMessage struct {
    ProcessID      int64       `json:"process_id"`
    Request        interface{} `json:"request"`
    Configurations interface{} `json:"configurations"`
    DataPath       string      `json:"data_path"`
}
```

### MEASUREMENT_READY

Signal that measurement is ready for instrument daemon.

```yaml
# falcon-api/embedded/commands/v1/measurement_ready.yaml
channel: MEASUREMENT_READY
parameters:
  timestamp: int
  getters: list[string]    # InstrumentPort JSONs
  setters: list[string]    # InstrumentPort JSONs
  requirements: list[string]
  has_set: boolean
  has_trigger: boolean
  is_buffered: boolean
  process_id: int
  chunk_id: int
```

**Go Type:**
```go
type MeasurementReadyMessage struct {
    Timestamp    int64    `json:"timestamp"`
    Getters      []string `json:"getters"`
    Setters      []string `json:"setters"`
    Requirements []string `json:"requirements"`
    HasSet       bool     `json:"has_set"`
    HasTrigger   bool     `json:"has_trigger"`
    IsBuffered   bool     `json:"is_buffered"`
    ProcessID    int64    `json:"process_id"`
    ChunkID      int64    `json:"chunk_id"`
}
```

### PROCESS_DATA

Data collected from instruments.

```yaml
# falcon-api/embedded/commands/v1/process_data.yaml
channel: PROCESS_DATA
parameters:
  chunk_id: int
  timestamp: int
  data: string             # JSON-serialized measurement data
  process_id: int
```

**Go Type:**
```go
type ProcessDataMessage struct {
    ChunkID   int64  `json:"chunk_id"`
    Timestamp int64  `json:"timestamp"`
    Data      string `json:"data"`
    ProcessID int64  `json:"process_id"`
}
```

### UPLOAD_DATA

Notification of uploaded measurement results.

```yaml
# falcon-api/embedded/commands/v1/upload_data.yaml
channel: UPLOAD_DATA
parameters:
  timestamp: int
  process_id: int
  unit_hash: int           # Algorithmic unit hash
  channel: string          # NATS channel for data retrieval
  stream: string           # JetStream stream name
```

**Go Type:**
```go
type UploadDataMessage struct {
    Timestamp int64  `json:"timestamp"`
    ProcessID int64  `json:"process_id"`
    UnitHash  int64  `json:"unit_hash"`
    Channel   string `json:"channel"`
    Stream    string `json:"stream"`
}
```

### UPDATE_DAEMON_PROPERTY

Update instrument daemon property.

```yaml
# falcon-api/embedded/commands/v1/update_daemon_property.yaml
channel: UPDATE_DAEMON_PROPERTY
parameters:
  timestamp: int
  property: string         # Property name
  name: string             # InstrumentPort JSON
  value: any               # Value to set
```

**Go Type:**
```go
type UpdateDaemonPropertyMessage struct {
    Timestamp int64       `json:"timestamp"`
    Property  string      `json:"property"`
    Name      string      `json:"name"`
    Value     interface{} `json:"value"`
}
```

### STATUS

Daemon status heartbeat.

```yaml
# falcon-api/embedded/commands/v1/status.yaml
channel: STATUS
parameters:
  timestamp: int
  status: boolean          # Active status
```

**Go Type:**
```go
type StatusMessage struct {
    Timestamp int64 `json:"timestamp"`
    Status    bool  `json:"status"`
}
```

## Instrument Coordination Channels

### SET

Execute a set instruction on an instrument.

```yaml
# falcon-api/embedded/commands/v1/set.yaml
channel: SET
parameters:
  timestamp: int
  process_id: int
  chunk_id: int
  property: string
  index: int
  value: any
```

### GET

Execute a get instruction on an instrument.

```yaml
# falcon-api/embedded/commands/v1/get.yaml
channel: GET
parameters:
  timestamp: int
  process_id: int
  chunk_id: int
  property: string
  index: int
```

### TRIGGER

Trigger buffered instruments.

```yaml
# falcon-api/embedded/commands/v1/trigger.yaml
channel: TRIGGER
parameters:
  timestamp: int
  process_id: int
  chunk_id: int
  is_setter: boolean
```

### ARMED

Instrument armed and ready notification.

```yaml
# falcon-api/embedded/commands/v1/armed.yaml
channel: ARMED
parameters:
  timestamp: int
  process_id: int
  chunk_id: int
```

### EXECUTING

Instrument currently executing notification.

```yaml
# falcon-api/embedded/commands/v1/executing.yaml
channel: EXECUTING
parameters:
  timestamp: int
  process_id: int
  chunk_id: int
```

### RETURN_DATA

Measurement data response from instrument.

```yaml
# falcon-api/embedded/commands/v1/return_data.yaml
channel: RETURN_DATA
parameters:
  timestamp: int
  process_id: int
  chunk_id: int
  data: any
```

### RETURN_GET

Get operation response from instrument.

```yaml
# falcon-api/embedded/commands/v1/return_get.yaml
channel: RETURN_GET
parameters:
  timestamp: int
  process_id: int
  chunk_id: int
  value: any
```

## JetStream Configuration

The Server Interpreter uses JetStream for reliable data transfer:

```go
streamConfig := &nats.StreamConfig{
    Name:      "FALCON_MEASUREMENTS",
    Subjects:  []string{"measurement.result.>", "measurement.data.>"},
    Retention: nats.LimitsPolicy,
    MaxAge:    24 * time.Hour,
    MaxMsgs:   10000,
    MaxBytes:  1024 * 1024 * 1024, // 1GB
    Storage:   nats.FileStorage,
}
```

### Measurement Completion Notification

When the hub completes an averaged measurement, it publishes a notification to JetStream:

**Subject:** `measurement.result.{measurement_id}`

```json
{
  "type": "measurement_complete",
  "measurement_id": "jetstream-test-001",
  "process_id": 42,
  "status": "success",
  "data_location": {
    "stream": "FALCON_MEASUREMENTS",
    "subject": "measurement.result.jetstream-test-001",
    "file_path": "/data/measurements/sweep_jetstream-test-001.json",
    "num_points": 101,
    "num_sweeps": 10
  },
  "timestamp": "2026-02-09T10:30:00Z"
}
```

**Go Type:**
```go
type FalconMeasurementNotification struct {
    Type          string             `json:"type"`
    MeasurementID string             `json:"measurement_id"`
    ProcessID     int64              `json:"process_id"`
    Status        string             `json:"status"`
    DataLocation  FalconDataLocation `json:"data_location"`
    Timestamp     time.Time          `json:"timestamp"`
}

type FalconDataLocation struct {
    Stream    string `json:"stream"`
    Subject   string `json:"subject"`
    FilePath  string `json:"file_path"`
    NumPoints int    `json:"num_points"`
    NumSweeps int    `json:"num_sweeps"`
}
```

### Falcon Subscription Example

Falcon can subscribe to measurement completions:

```python
import nats

async def handle_measurement_complete(msg):
    data = json.loads(msg.data)
    if data["status"] == "success":
        file_path = data["data_location"]["file_path"]
        # Load and process measurement data
        
js = nc.jetstream()
await js.subscribe("measurement.result.>", cb=handle_measurement_complete)
```

## Message Flow Example

```
Falcon                 Server Interpreter           Instrument Daemon
  │                           │                           │
  │──PROCESS_REQUEST────────>│                           │
  │                           │                           │
  │                           │──UPDATE_DAEMON_PROPERTY─>│
  │                           │──UPDATE_DAEMON_PROPERTY─>│
  │                           │──MEASUREMENT_READY──────>│
  │                           │                           │
  │                           │<──────────ARMED──────────│
  │                           │<────────EXECUTING────────│
  │                           │<──────RETURN_DATA────────│
  │                           │                           │
  │                           │<──────PROCESS_DATA───────│
  │                           │                           │
  │<────────UPLOAD_DATA───────│                           │
  │                           │                           │
```

## Error Handling

Errors are communicated through:
1. LOG channel messages
2. Empty/error responses on request channels
3. NATS subscription errors

## Channel Constants

In Go code, use `RuntimeChannels`:

```go
serverinterpreter.RuntimeChannels.ProcessRequest  // "PROCESS_REQUEST"
serverinterpreter.RuntimeChannels.ProcessData     // "PROCESS_DATA"
serverinterpreter.RuntimeChannels.MeasurementReady // "MEASUREMENT_READY"
serverinterpreter.RuntimeChannels.UploadData      // "UPLOAD_DATA"
serverinterpreter.RuntimeChannels.Log             // "LOG"
serverinterpreter.RuntimeChannels.Status          // "STATUS"
```

## See Also

- [falcon-api](https://github.com/falcon-autotuning/falcon-api) - Canonical API specifications
- [Server Interpreter](server-interpreter.md) - Implementation details
