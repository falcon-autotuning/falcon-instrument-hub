package measurements

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/client"
	"github.com/nats-io/nats.go"
)

// CommandHandler handles measurement commands received via NATS
// and executes them via the instrument-script-server
type CommandHandler struct {
	natsConn          *nats.Conn
	instrumentClient  *client.InstrumentServerClient
	userScriptsDir    string
	scriptNameMapping map[string]string // Maps measurement type to script filename
}

// NewCommandHandler creates a new measurement command handler
func NewCommandHandler(natsURL string, instrumentClient *client.InstrumentServerClient, userScriptsDir string) (*CommandHandler, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	handler := &CommandHandler{
		natsConn:         nc,
		instrumentClient: instrumentClient,
		userScriptsDir:   userScriptsDir,
		scriptNameMapping: map[string]string{
			"set_voltage":          "set_voltage.lua",
			"get_voltage":          "get_voltage.lua",
			"set_many_voltages":    "set_many_voltages.lua",
			"get_many_voltages":    "get_many_voltages.lua",
			"get_all_voltages":     "get_all_voltages.lua",
			"measure_1D_buffered":  "measure_1D_buffered.lua",
			"measure_2D_buffered":  "measure_2D_buffered.lua",
			"measure_current":      "measure_current.lua",
			"measure_get_set":      "measure_get_set.lua",
			"measure_illumination": "measure_illumination.lua",
			"measure_leakage":      "measure_leakage.lua",
			"ramp":                 "ramp.lua",
			"set_slope":            "set_slope.lua",
			"get_slope":            "get_slope.lua",
			"set_sample_rate":      "set_sample_rate.lua",
			"get_sample_rate":      "get_sample_rate.lua",
			"set_number_of_samples": "set_number_of_samples.lua",
			"get_number_of_samples": "get_number_of_samples.lua",
			"set_trigger_leader":   "set_trigger_leader.lua",
			"get_trigger_leader":   "get_trigger_leader.lua",
		},
	}

	return handler, nil
}

// Close closes the NATS connection
func (h *CommandHandler) Close() {
	if h.natsConn != nil {
		h.natsConn.Close()
	}
}

// MeasurementCommand represents a generic measurement command
type MeasurementCommand struct {
	// Type is the measurement type (e.g., "set_voltage", "measure_1D_buffered")
	Type string `json:"type"`
	// Input is the JSON-encoded input struct for the measurement
	Input json.RawMessage `json:"input"`
	// RequestID is a unique identifier for tracking this request
	RequestID string `json:"request_id"`
}

// MeasurementCommandResponse represents the response to a measurement command
type MeasurementCommandResponse struct {
	// RequestID matches the request that generated this response
	RequestID string `json:"request_id"`
	// Success indicates if the measurement succeeded
	Success bool `json:"success"`
	// Output is the JSON-encoded output struct for the measurement
	Output json.RawMessage `json:"output,omitempty"`
	// Error contains error details if Success is false
	Error string `json:"error,omitempty"`
}

// StartListening starts listening for measurement commands on NATS
// Commands are expected on the subject "measurement.command"
// Responses are sent to "measurement.response.{request_id}"
func (h *CommandHandler) StartListening(ctx context.Context) error {
	// Subscribe to measurement commands
	sub, err := h.natsConn.Subscribe("measurement.command", func(msg *nats.Msg) {
		// Process command in background
		go h.handleCommand(ctx, msg)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to measurement.command: %w", err)
	}

	log.Println("Measurement command handler listening on 'measurement.command'")

	// Wait for context cancellation
	<-ctx.Done()

	// Unsubscribe
	sub.Unsubscribe()
	return nil
}

// handleCommand processes a single measurement command
func (h *CommandHandler) handleCommand(ctx context.Context, msg *nats.Msg) {
	var cmd MeasurementCommand
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		log.Printf("Failed to unmarshal command: %v", err)
		h.sendErrorResponse("", fmt.Errorf("invalid command format: %w", err))
		return
	}

	log.Printf("Received measurement command: type=%s, request_id=%s", cmd.Type, cmd.RequestID)

	// Execute the measurement
	result, err := h.executeMeasurement(ctx, &cmd)
	if err != nil {
		log.Printf("Measurement failed: %v", err)
		h.sendErrorResponse(cmd.RequestID, err)
		return
	}

	// Send success response
	response := MeasurementCommandResponse{
		RequestID: cmd.RequestID,
		Success:   true,
		Output:    result,
	}

	responseData, err := json.Marshal(response)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}

	// Publish response
	responseTopic := fmt.Sprintf("measurement.response.%s", cmd.RequestID)
	if err := h.natsConn.Publish(responseTopic, responseData); err != nil {
		log.Printf("Failed to publish response: %v", err)
	}
}

// executeMeasurement executes a measurement command via instrument-script-server
func (h *CommandHandler) executeMeasurement(ctx context.Context, cmd *MeasurementCommand) (json.RawMessage, error) {
	// Get the script path for this measurement type
	scriptFilename, ok := h.scriptNameMapping[cmd.Type]
	if !ok {
		return nil, fmt.Errorf("unknown measurement type: %s", cmd.Type)
	}

	scriptPath := filepath.Join(h.userScriptsDir, scriptFilename)
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("measurement script not found: %s", scriptPath)
	}

	// Parse the input to extract globals for the Lua script
	var globals map[string]interface{}
	if err := json.Unmarshal(cmd.Input, &globals); err != nil {
		return nil, fmt.Errorf("failed to parse input: %w", err)
	}

	// Execute measurement via instrument-script-server
	measureReq := client.MeasureRequest{
		ScriptPath: scriptPath,
		Globals:    globals,
		OutputJSON: true,
	}

	result, err := h.instrumentClient.Measure(ctx, measureReq)
	if err != nil {
		return nil, fmt.Errorf("measurement execution failed: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("measurement error: %s", result.Error)
	}

	// Convert results to JSON
	outputData, err := json.Marshal(result.Results)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal results: %w", err)
	}

	return outputData, nil
}

// sendErrorResponse sends an error response for a failed measurement
func (h *CommandHandler) sendErrorResponse(requestID string, err error) {
	response := MeasurementCommandResponse{
		RequestID: requestID,
		Success:   false,
		Error:     err.Error(),
	}

	responseData, _ := json.Marshal(response)

	// If we have a request ID, send to specific response topic
	if requestID != "" {
		responseTopic := fmt.Sprintf("measurement.response.%s", requestID)
		h.natsConn.Publish(responseTopic, responseData)
	}
}

// Helper functions for executing specific measurement types with type safety

// ExecuteSetVoltage is a typed helper for executing a set_voltage measurement
func ExecuteSetVoltage(ctx context.Context, handler *CommandHandler, req *SetVoltageRequest, requestID string) (*SetVoltageResponse, error) {
	inputData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := &MeasurementCommand{
		Type:      "set_voltage",
		Input:     inputData,
		RequestID: requestID,
	}

	result, err := handler.executeMeasurement(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var response SetVoltageResponse
	if len(result) > 0 {
		if err := json.Unmarshal(result, &response); err != nil {
			return nil, err
		}
	}

	return &response, nil
}

// ExecuteMeasure1DBuffered is a typed helper for executing a measure_1D_buffered measurement
func ExecuteMeasure1DBuffered(ctx context.Context, handler *CommandHandler, req *Measure1DBufferedRequest, requestID string) (*Measure1DBufferedResponse, error) {
	inputData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	cmd := &MeasurementCommand{
		Type:      "measure_1D_buffered",
		Input:     inputData,
		RequestID: requestID,
	}

	result, err := handler.executeMeasurement(ctx, cmd)
	if err != nil {
		return nil, err
	}

	var response Measure1DBufferedResponse
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// Additional typed helpers can be added for other measurement types as needed
