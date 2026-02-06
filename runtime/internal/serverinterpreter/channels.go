// Package serverinterpreter provides NATS channel definitions for the server interpreter.
//
// These channels define the internal communication protocol between the interpreter daemon
// and the instrument daemon, as specified in falcon-api/embedded/commands/v1/.
//
// The server interpreter is responsible for:
//   - Processing measurement requests (PROCESS_REQUEST)
//   - Coordinating with instruments via MEASUREMENT_READY
//   - Collecting measurement data (PROCESS_DATA)
//   - Uploading results back to falcon (UPLOAD_DATA)
//
// Channel definitions are aligned with falcon-api YAML specifications.
package serverinterpreter

// RuntimeChannels contains all the NATS channel names used by the server interpreter.
// These match falcon-api/embedded/mapping.yaml for the server-interpreter role.
var RuntimeChannels = struct {
	// Core interpreter channels
	Log                  string
	MeasurementReady     string
	ProcessData          string
	ProcessRequest       string
	Status               string
	UpdateDaemonProperty string
	UploadData           string

	// Instrument coordination channels (from instrument-server role)
	Set         string
	Get         string
	Trigger     string
	Armed       string
	Executing   string
	ReturnData  string
	ReturnGet   string
}{
	// Core channels owned by server-interpreter
	Log:                  "LOG",
	MeasurementReady:     "MEASUREMENT_READY",
	ProcessData:          "PROCESS_DATA",
	ProcessRequest:       "PROCESS_REQUEST",
	Status:               "STATUS",
	UpdateDaemonProperty: "UPDATE_DAEMON_PROPERTY",
	UploadData:           "UPLOAD_DATA",

	// Instrument coordination channels
	Set:         "SET",
	Get:         "GET",
	Trigger:     "TRIGGER",
	Armed:       "ARMED",
	Executing:   "EXECUTING",
	ReturnData:  "RETURN_DATA",
	ReturnGet:   "RETURN_GET",
}

// LogMessage represents a log message sent to the NATS server.
// Matches falcon-api/embedded/commands/v1/log.yaml
type LogMessage struct {
	Hash      int64  `json:"hash,omitempty"` // Optional process identifier
	Message   string `json:"message"`        // Log message text
	Timestamp int64  `json:"timestamp"`      // Unix timestamp
}

// MeasurementReadyMessage indicates a measurement is ready for the server to perform.
// Matches falcon-api/embedded/commands/v1/measurement_ready.yaml
type MeasurementReadyMessage struct {
	Timestamp    int64    `json:"timestamp"`
	Getters      []string `json:"getters"`      // JSON-serialized InstrumentPorts for reading
	Setters      []string `json:"setters"`      // JSON-serialized InstrumentPorts for writing
	Requirements []string `json:"requirements"` // JSON-serialized requirement objects
	HasSet       bool     `json:"has_set"`      // Whether setters need to be applied
	HasTrigger   bool     `json:"has_trigger"`  // Whether trigger is required
	IsBuffered   bool     `json:"is_buffered"`  // Whether this is a buffered measurement
	ProcessID    int64    `json:"process_id"`   // Process identifier
	ChunkID      int64    `json:"chunk_id"`     // Chunk identifier within the process
}

// RequirementEntry represents a single requirement for an instrument.
type RequirementEntry struct {
	Setter   string    `json:"setter"`   // JSON-serialized InstrumentPort
	Property []string  `json:"property"` // Property names
	Values   []float64 `json:"values"`   // Property values
}

// ProcessDataMessage contains data collected from instruments.
// Matches falcon-api/embedded/commands/v1/process_data.yaml
type ProcessDataMessage struct {
	ChunkID   int64  `json:"chunk_id"`   // Chunk identifier
	Timestamp int64  `json:"timestamp"`  // Unix timestamp
	Data      string `json:"data"`       // JSON-serialized measurement data
	ProcessID int64  `json:"process_id"` // Process identifier
}

// ProcessDataMessageMap is an alternative representation with parsed data.
// Used internally when data is already parsed to a map structure.
type ProcessDataMessageMap struct {
	ChunkID   int64             `json:"chunk_id"`
	Timestamp int64             `json:"timestamp"`
	Data      map[string]string `json:"data"` // InstrumentPort JSON -> MeasuredArray1D JSON
	ProcessID int64             `json:"process_id"`
}

// ProcessRequestMessage is a request to the interpreter to process a measurement.
// Matches falcon-api/embedded/commands/v1/process_request.yaml
type ProcessRequestMessage struct {
	ProcessID      int64       `json:"process_id"`      // Process identifier
	Request        interface{} `json:"request"`         // The MeasurementRequest (jsonable)
	Configurations interface{} `json:"configurations"`  // Instrument configurations (json)
	DataPath       string      `json:"data_path"`       // Filepath to store collected data
}

// StatusMessage provides the status of the interpreter process.
// Matches falcon-api/embedded/commands/v1/status.yaml
type StatusMessage struct {
	Timestamp int64 `json:"timestamp"` // Unix timestamp
	Status    bool  `json:"status"`    // Active status
}

// UpdateDaemonPropertyMessage updates a property in the instrument daemon.
// Matches falcon-api/embedded/commands/v1/update_daemon_property.yaml
type UpdateDaemonPropertyMessage struct {
	Timestamp int64       `json:"timestamp"` // Unix timestamp
	Property  string      `json:"property"`  // Property name (e.g., "voltage_state")
	Name      string      `json:"name"`      // JSON-serialized InstrumentPort
	Value     interface{} `json:"value"`     // The value to set
}

// UploadDataMessage sends measurement data back to falcon.
// Matches falcon-api/embedded/commands/v1/upload_data.yaml
type UploadDataMessage struct {
	Timestamp int64  `json:"timestamp"`  // Unix timestamp
	ProcessID int64  `json:"process_id"` // Process identifier
	UnitHash  int64  `json:"unit_hash"`  // Algorithmic unit hash
	Channel   string `json:"channel"`    // NATS channel for data retrieval
	Stream    string `json:"stream"`     // JetStream stream name
}

// UploadDataMessageLegacy is the legacy format for backward compatibility.
// Deprecated: Use UploadDataMessage instead.
type UploadDataMessageLegacy struct {
	Timestamp int64  `json:"timestamp"`
	ProcessID int64  `json:"process_id"`
	Data      string `json:"data"` // JSON-serialized MeasurementResponse
}

// SupportedProperties defines the property names used in instrument configuration.
// These match the properties defined across falcon-api command schemas.
var SupportedProperties = struct {
	VoltageState                 string
	CurrentState                 string
	NumberOfSamples              string
	SampleRate                   string
	Timeout                      string
	Slope                        string
	Staircase                    string
	SupportsBufferedMeasurements string
}{
	VoltageState:                 "voltage_state",
	CurrentState:                 "current_state",
	NumberOfSamples:              "number_of_samples",
	SampleRate:                   "sample_rate",
	Timeout:                      "timeout",
	Slope:                        "slope",
	Staircase:                    "staircase",
	SupportsBufferedMeasurements: "supports_buffered_measurements",
}

// Default configuration values for the server interpreter.
const (
	DefaultSlope            = 100.0  // V/sec
	DefaultSampleRate       = 10000  // samples/sec
	MaxNumDataPoints        = 10000
	StaleMeasurementTimeout = 3600   // seconds
	StaleMeasurementCheckup = 60.0   // seconds
	TimeoutScaleFactor      = 1.5
)

// =============================================================================
// Instrument Coordination Messages
// These match falcon-api/embedded/commands/v1/ for instrument-server role
// =============================================================================

// SetMessage executes a set instruction on a sandboxed instrument.
// Matches falcon-api/embedded/commands/v1/set.yaml
type SetMessage struct {
	Timestamp int64       `json:"timestamp"`  // Unix timestamp
	ProcessID int64       `json:"process_id"` // Process identifier
	ChunkID   int64       `json:"chunk_id"`   // Chunk identifier
	Property  string      `json:"property"`   // Property to set
	Index     int         `json:"index"`      // Property index
	Value     interface{} `json:"value"`      // Value to set
}

// GetMessage executes a get instruction on a sandboxed instrument.
// Matches falcon-api/embedded/commands/v1/get.yaml
type GetMessage struct {
	Timestamp int64  `json:"timestamp"`  // Unix timestamp
	ProcessID int64  `json:"process_id"` // Process identifier
	ChunkID   int64  `json:"chunk_id"`   // Chunk identifier
	Property  string `json:"property"`   // Property to get
	Index     int    `json:"index"`      // Property index
}

// TriggerMessage triggers buffered instruments to execute.
// Matches falcon-api/embedded/commands/v1/trigger.yaml
type TriggerMessage struct {
	Timestamp int64 `json:"timestamp"`  // Unix timestamp
	ProcessID int64 `json:"process_id"` // Process identifier
	ChunkID   int64 `json:"chunk_id"`   // Chunk identifier
	IsSetter  bool  `json:"is_setter"`  // Whether triggering setters
}

// ArmedMessage indicates an instrument is armed and ready.
// Matches falcon-api/embedded/commands/v1/armed.yaml
type ArmedMessage struct {
	Timestamp int64 `json:"timestamp"`  // Unix timestamp
	ProcessID int64 `json:"process_id"` // Process identifier
	ChunkID   int64 `json:"chunk_id"`   // Chunk identifier
}

// ExecutingMessage indicates an instrument is currently executing.
// Matches falcon-api/embedded/commands/v1/executing.yaml
type ExecutingMessage struct {
	Timestamp int64 `json:"timestamp"`  // Unix timestamp
	ProcessID int64 `json:"process_id"` // Process identifier
	ChunkID   int64 `json:"chunk_id"`   // Chunk identifier
}

// ReturnDataMessage returns measurement data from an instrument.
// Matches falcon-api/embedded/commands/v1/return_data.yaml
type ReturnDataMessage struct {
	Timestamp int64       `json:"timestamp"`  // Unix timestamp
	ProcessID int64       `json:"process_id"` // Process identifier
	ChunkID   int64       `json:"chunk_id"`   // Chunk identifier
	Data      interface{} `json:"data"`       // Measurement data
}

// ReturnGetMessage returns the result of a get operation.
// Matches falcon-api/embedded/commands/v1/return_get.yaml
type ReturnGetMessage struct {
	Timestamp int64       `json:"timestamp"`  // Unix timestamp
	ProcessID int64       `json:"process_id"` // Process identifier
	ChunkID   int64       `json:"chunk_id"`   // Chunk identifier
	Value     interface{} `json:"value"`      // Retrieved value
}
