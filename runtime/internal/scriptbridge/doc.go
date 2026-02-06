// Package scriptbridge provides a bridge between falcon-core MeasurementRequest
// objects and the instrument-script-server.
//
// # Overview
//
// The scriptbridge package handles the conversion of serialized falcon measurement
// requests into Lua scripts that can be executed by the instrument-script-server.
// This enables the falcon-instrument-hub to:
//
//  1. Receive serialized MeasurementRequest objects over NATS from falcon
//  2. Parse and convert them using falcon-measurement-lib type definitions
//  3. Generate appropriate Lua measurement scripts
//  4. Submit scripts to instrument-script-server via HTTP RPC
//  5. Wait for job completion and return results
//  6. Serialize results back to falcon format for NATS transmission
//
// # Architecture
//
// The package consists of several components:
//
//   - Types (types.go): Core data structures matching falcon-measurement-lib schemas
//   - Client (client.go): HTTP RPC client for instrument-script-server
//   - Generator (generator.go): Lua script template generation
//   - Bridge (bridge.go): Main orchestration logic
//   - NATS Handler (nats_handler.go): NATS message handling integration
//
// # Usage
//
// Basic usage for executing a set_voltage command:
//
//	config := scriptbridge.BridgeConfig{
//		ScriptServerHost: "127.0.0.1",
//		ScriptServerPort: 8555,
//		ScriptOutputDir:  "/tmp/falcon-scripts",
//	}
//
//	bridge, err := scriptbridge.NewBridge(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Execute a simple set_voltage
//	result, err := bridge.ExecuteSetVoltage("DAC1", 0, 1.5)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Job %s completed with status: %s\n", result.JobID, result.Status)
//
// # NATS Integration
//
// For full NATS integration with falcon:
//
//	handler, err := scriptbridge.NewNATSBridgeHandler(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	nc, err := nats.Connect("nats://localhost:4222")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	if err := handler.Subscribe(nc, "MEASURE_COMMAND.external"); err != nil {
//		log.Fatal(err)
//	}
//
// # Supported Operations
//
// Currently supported measurement operations:
//
//   - set_voltage: Sets a voltage on an instrument/channel
//   - get_voltage: Reads a voltage from an instrument/channel
//   - measure_get_set: Combined set then get operation
//
// The generated Lua scripts follow the instrument-script-server's main() function
// format with proper runtime context handling.
//
// # Type Mapping
//
// The package uses types that mirror falcon-measurement-lib schemas:
//
//   - InstrumentTarget: Reference to an instrument with optional channel
//   - SetVoltageRequest: Parameters for set_voltage operation
//   - GetVoltageRequest: Parameters for get_voltage operation
//   - MeasurementResponse: Standard response wrapper with value and metadata
//
// # Configuration
//
// BridgeConfig provides the following options:
//
//   - ScriptServerHost: Host address of instrument-script-server (default: 127.0.0.1)
//   - ScriptServerPort: RPC port of instrument-script-server (default: 8555)
//   - ScriptOutputDir: Directory for generated Lua scripts (default: /tmp/falcon-scripts)
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
package scriptbridge
