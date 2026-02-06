// Package serverinterpreter provides the server interpreter for falcon measurements.
//
// The server interpreter bridges falcon-core MeasurementRequest objects to
// instrument commands, coordinating through NATS messaging. Message types
// are aligned with falcon-api/embedded/commands/v1/ specifications.
//
// # Overview
//
// The serverinterpreter package processes measurement requests from falcon,
// breaks them into instruction chunks, coordinates with instruments, and
// uploads results. It supports two modes of operation:
//
//  1. Direct Mode (Bridge): Generates Lua scripts and submits via HTTP RPC
//  2. Daemon Mode (InterpreterDaemon): Uses NATS internal messaging
//
// # Architecture
//
// The package consists of several components:
//
//   - channels.go: NATS channel definitions aligned with falcon-api specs
//   - interpreter_daemon.go: Main daemon for NATS-based measurement processing
//   - data_collector.go: Async data collection using Go channels
//   - waveform_processor.go: Waveform chunking and instruction generation
//   - instructions.go: Measurement instruction types
//   - types.go: Core data structures for falcon-measurement-lib schemas
//   - bridge.go: Direct mode orchestration with HTTP RPC
//   - client.go: HTTP RPC client for instrument-script-server
//   - generator.go: Lua script template generation
//   - falcon_core.go: falcon-core-libs Go bindings wrapper
//
// # Daemon Mode Usage (Recommended)
//
// The InterpreterDaemon provides full measurement processing:
//
//	config := serverinterpreter.DefaultInterpreterConfig()
//	daemon := serverinterpreter.NewInterpreterDaemon(config)
//
//	if err := daemon.Start(); err != nil {
//		log.Fatal(err)
//	}
//	defer daemon.Stop()
//
//	// The daemon handles:
//	// - PROCESS_REQUEST: Receives measurement requests
//	// - PROCESS_DATA: Receives collected data chunks
//	// - Sends: MEASUREMENT_READY, UPDATE_DAEMON_PROPERTY, UPLOAD_DATA
//
// # NATS Channel Protocol
//
// Message schemas are defined in falcon-api/embedded/commands/v1/.
// Core channels owned by server-interpreter (per mapping.yaml):
//
//   - LOG: Logging messages with hash, message, timestamp
//   - MEASUREMENT_READY: Signal with getters, setters, requirements, buffered flag
//   - PROCESS_DATA: Data chunks with chunk_id, timestamp, data, process_id
//   - PROCESS_REQUEST: Request with process_id, request, configurations, data_path
//   - STATUS: Daemon status with timestamp and status flag
//   - UPDATE_DAEMON_PROPERTY: Property updates with name, property, value
//   - UPLOAD_DATA: Result notification with channel, stream, process_id
//
// Instrument coordination channels (shared with instrument-server):
//
//   - SET: Execute set instruction with property, index, value
//   - GET: Execute get instruction with property, index
//   - TRIGGER: Trigger buffered instruments
//   - ARMED: Instrument armed notification
//   - EXECUTING: Instrument executing notification
//   - RETURN_DATA: Measurement data response
//   - RETURN_GET: Get operation response
//
// # Direct Mode Usage
//
// For direct HTTP RPC communication with instrument-script-server:
//
//	config := serverinterpreter.BridgeConfig{
//		ScriptServerHost: "127.0.0.1",
//		ScriptServerPort: 8555,
//		ScriptOutputDir:  "/tmp/falcon-scripts",
//	}
//
//	bridge, err := serverinterpreter.NewBridge(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	result, err := bridge.ExecuteSetVoltage("DAC1", 0, 1.5)
//
// # Waveform Processing
//
// The WaveformProcessor breaks down complex waveforms into instructions:
//
//   - Chunks raw time traces at staircase boundaries
//   - Decides buffered vs unbuffered measurements
//   - Generates MEASUREMENT_READY messages for instrument daemon
//   - Supports ramp interjection between buffered measurements
//
// # Async Data Collection
//
// The DataCollector handles asynchronous data from instruments:
//
//   - Uses Go channels for concurrent data processing
//   - Tracks pending measurements with expected chunk counts
//   - Automatically triggers completion handlers
//   - Cleans up stale measurements after timeout
//
// # falcon-core Integration
//
// When built with -tags falcon_core, the package integrates with
// falcon-core-libs Go bindings for proper MeasurementRequest parsing.
// Without this tag, a pure-Go JSON parser is used for testing.
//
// # Error Handling
//
// Errors are returned at key points:
//
//   - JSON parsing failures
//   - Script generation failures
//   - RPC communication failures
//   - Job execution failures
//
// ExecutionResult includes status and error fields for detailed feedback.
package serverinterpreter
