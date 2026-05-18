// Package serverinterpreter provides the main bridge between falcon MeasurementRequest
// objects and the instrument-script-server.
//
// There are two modes of operation:
//  1. Direct mode (Bridge): Uses HTTP RPC to communicate directly with instrument-script-server
//  2. Internal API mode (InterpreterDaemon): Uses NATS internal messaging aligned with falcon-api specs
package serverinterpreter

// Bridge orchestrates the conversion of falcon MeasurementRequest objects
// to instrument-script-server commands and handles the response flow.
type Bridge struct {
	client *ScriptServerClient
}

// BridgeConfig holds configuration for the Bridge.
type BridgeConfig struct {
	// ScriptServerHost is the host address of the instrument-script-server.
	ScriptServerHost string
	// ScriptServerPort is the port of the instrument-script-server RPC API.
	ScriptServerPort int
}

// DefaultBridgeConfig returns a default configuration.
func DefaultBridgeConfig() BridgeConfig {
	return BridgeConfig{
		ScriptServerHost: "127.0.0.1",
		ScriptServerPort: 8555,
	}
}

// NewBridge creates a new Bridge with the given configuration.
func NewBridge(config BridgeConfig) (*Bridge, error) {
	client := NewScriptServerClient(config.ScriptServerHost, config.ScriptServerPort)

	return &Bridge{
		client: client,
	}, nil
}
