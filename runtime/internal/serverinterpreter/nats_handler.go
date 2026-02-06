// Package serverinterpreter provides an example integration with NATS messaging.
//
// This file demonstrates how to integrate the server interpreter with falcon-instrument-hub's
// NATS-based message handling system. It shows the complete flow from receiving a
// serialized MeasurementRequest to sending commands to the instrument-script-server.
package serverinterpreter

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/nats-io/nats.go"
)

// NATSBridgeHandler handles MeasurementRequest messages from NATS and bridges
// them to the instrument-script-server.
type NATSBridgeHandler struct {
	bridge       *Bridge
	nc           *nats.Conn
	subscription *nats.Subscription
	mutex        sync.RWMutex
}

// NATSMeasurementCommand is the expected format of measurement commands from falcon.
// This matches the MeasureCommand structure in api.go
type NATSMeasurementCommand struct {
	Request   string `json:"request"`   // The measurement request JSON (from falcon-core)
	Timestamp int64  `json:"timestamp"` // When the command was sent
	Hash      int64  `json:"hash"`      // Unique identifier for correlation
}

// NATSMeasurementResponse is the response format sent back to falcon.
// This matches the MeasureResponse structure in api.go
type NATSMeasurementResponse struct {
	Response  string `json:"response"`  // The measurement response JSON
	Timestamp int64  `json:"timestamp"` // When the response was completed
	Hash      int64  `json:"hash"`      // Hash from the original command
}

// NewNATSBridgeHandler creates a new NATS bridge handler.
func NewNATSBridgeHandler(config BridgeConfig) (*NATSBridgeHandler, error) {
	bridge, err := NewBridge(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create bridge: %w", err)
	}

	return &NATSBridgeHandler{
		bridge: bridge,
	}, nil
}

// Subscribe starts listening for measurement commands on NATS.
func (h *NATSBridgeHandler) Subscribe(nc *nats.Conn, subject string) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.nc = nc

	var err error
	h.subscription, err = nc.Subscribe(subject+".>", h.handleMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	log.Printf("NATSBridgeHandler: Subscribed to %s.>", subject)
	return nil
}

// Unsubscribe stops listening for commands.
func (h *NATSBridgeHandler) Unsubscribe() error {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
			return err
		}
		h.subscription = nil
	}

	return nil
}

// handleMessage processes incoming measurement commands.
func (h *NATSBridgeHandler) handleMessage(msg *nats.Msg) {
	log.Printf("NATSBridgeHandler: Received message on %s", msg.Subject)

	// Parse the incoming command
	var command NATSMeasurementCommand
	if err := json.Unmarshal(msg.Data, &command); err != nil {
		log.Printf("NATSBridgeHandler: Failed to parse command: %v", err)
		return
	}

	log.Printf("NATSBridgeHandler: Processing measurement request with hash %d", command.Hash)

	// Execute the measurement request through the bridge
	result, err := h.bridge.ExecuteMeasurementRequestJSON(command.Request)
	if err != nil {
		log.Printf("NATSBridgeHandler: Failed to execute measurement: %v", err)
		return
	}

	// Convert result to response
	responseJSON, err := result.ToSerializedResponse()
	if err != nil {
		log.Printf("NATSBridgeHandler: Failed to serialize response: %v", err)
		return
	}

	response := NATSMeasurementResponse{
		Response:  responseJSON,
		Timestamp: command.Timestamp,
		Hash:      command.Hash,
	}

	// Determine response subject
	responseSubject := "MEASURE_RESPONSE.external" // Adjust as needed

	responseData, err := json.Marshal(response)
	if err != nil {
		log.Printf("NATSBridgeHandler: Failed to marshal response: %v", err)
		return
	}

	if err := h.nc.Publish(responseSubject, responseData); err != nil {
		log.Printf("NATSBridgeHandler: Failed to publish response: %v", err)
		return
	}

	log.Printf("NATSBridgeHandler: Sent response for hash %d, status: %s", command.Hash, result.Status)
}

// Example usage demonstrating the complete flow:
//
//	// Setup NATS connection
//	nc, err := nats.Connect("nats://localhost:4222")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer nc.Close()
//
//	// Create bridge handler
//	config := serverinterpreter.BridgeConfig{
//		ScriptServerHost: "127.0.0.1",
//		ScriptServerPort: 8555,
//		ScriptOutputDir:  "/tmp/falcon-scripts",
//	}
//
//	handler, err := serverinterpreter.NewNATSBridgeHandler(config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Subscribe to measurement commands
//	if err := handler.Subscribe(nc, "MEASURE_COMMAND.external"); err != nil {
//		log.Fatal(err)
//	}
//	defer handler.Unsubscribe()
//
//	// Keep running...
//	select {}
