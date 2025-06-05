package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
)

const (
	MeasureCommandHandlerName = "MEASURE_COMMAND_HANDLER"
	MeasureCommandSubject     = "MEASURE_COMMAND.external"
	MeasureResponseSubject    = "MEASURE_RESPONSE.external"
	ProcessRequestSubject     = "PROCESS_REQUEST.interpreter"
)

// MeasureCommandHandler handles MEASURE_COMMAND requests
type MeasureCommandHandler struct {
	logger             *logging.Logger
	nc                 *nats.Conn
	subscription       *nats.Subscription
	measurementManager *measurements.Manager
}

// NewMeasureCommandHandler creates a new handler
func NewMeasureCommandHandler(
	logger *logging.Logger,
	measurementManager *measurements.Manager,
) *MeasureCommandHandler {
	return &MeasureCommandHandler{
		logger:             logger,
		measurementManager: measurementManager,
	}
}

// Subscribe starts listening for MEASURE_COMMAND requests
func (h *MeasureCommandHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error
	h.subscription, err = nc.Subscribe(
		MeasureCommandSubject+".>",
		h.handleMessage,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+MeasureCommandSubject+": %w",
			err,
		)
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		"Subscribed to "+MeasureCommandSubject+".>",
	)
	return nil
}

// Unsubscribe stops listening for commands
func (h *MeasureCommandHandler) Unsubscribe() error {
	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
			h.logger.Error(
				MeasureCommandHandlerName,
				fmt.Sprintf("Failed to unsubscribe: %v", err),
			)
			return err
		}
		h.subscription = nil
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		"Unsubscribed from "+MeasureCommandSubject,
	)
	return nil
}

// handleMessage processes incoming MEASURE_COMMAND requests
func (h *MeasureCommandHandler) handleMessage(msg *nats.Msg) {
	h.logger.Debug(
		MeasureCommandHandlerName,
		fmt.Sprintf("Received command: %s", string(msg.Data)),
	)

	// Extract the name from the subject (MEASURE_COMMAND.external.<name>)
	subjectParts := strings.Split(msg.Subject, ".")
	if len(subjectParts) < 3 {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf("Invalid subject format: %s", msg.Subject),
		)
		return
	}
	name := subjectParts[2]

	// Parse the incoming message
	var measureCommand api.MeasureCommand
	if err := json.Unmarshal(msg.Data, &measureCommand); err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf("Failed to unmarshal MEASURE_COMMAND: %v", err),
		)
		return
	}

	// Allocate measurement ID and get expected path
	timestamp := time.Now()
	uniqueID, expectedPath, err := h.measurementManager.AllocateMeasurementID(
		timestamp,
	)
	if err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf("Failed to allocate measurement ID: %v", err),
		)
		return
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		fmt.Sprintf(
			"Allocated measurement ID %d for hash %d",
			uniqueID,
			measureCommand.Hash,
		),
	)

	// Send PROCESS_REQUEST to interpreter
	if err := h.sendProcessRequest(measureCommand.Request, uniqueID, expectedPath); err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf("Failed to send PROCESS_REQUEST: %v", err),
		)
		return
	}

	// Store the request and response for processing
	// TODO: Implement actual measurement logic here
	// For now, we'll echo the request as response
	response := h.processMeasureRequest(measureCommand.Request)

	// Create the response
	measureResponse := api.MeasureResponse{
		Response:  response,
		Timestamp: time.Now().UnixMicro(),
		Hash:      measureCommand.Hash, // Transfer hash from request
	}

	// Marshal the response
	responseData, err := json.Marshal(measureResponse)
	if err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf("Failed to marshal MEASURE_RESPONSE: %v", err),
		)
		return
	}

	// Send response on MEASURE_RESPONSE.external.<name>
	responseSubject := fmt.Sprintf("%s.%s", MeasureResponseSubject, name)
	if err := h.nc.Publish(responseSubject, responseData); err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf(
				"Failed to publish response to %s: %v",
				responseSubject,
				err,
			),
		)
		return
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		fmt.Sprintf(
			"Sent MEASURE_RESPONSE to %s for hash %d",
			responseSubject,
			measureCommand.Hash,
		),
	)
}

// sendProcessRequest sends a PROCESS_REQUEST to the interpreter
func (h *MeasureCommandHandler) sendProcessRequest(
	request string,
	uniqueID int,
	dataPath string,
) error {
	// Create the ProcessRequest
	processRequest := api.ProcessRequest{
		Request:        request,
		Configurations: "", // TODO: Build configurations
		DataPath:       dataPath,
		ProcessId:      strconv.Itoa(uniqueID),
	}

	// Marshal the request
	requestData, err := json.Marshal(processRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal PROCESS_REQUEST: %w", err)
	}

	// Send to interpreter
	if err := h.nc.Publish(ProcessRequestSubject, requestData); err != nil {
		return fmt.Errorf("failed to publish PROCESS_REQUEST: %w", err)
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		fmt.Sprintf(
			"Sent PROCESS_REQUEST to interpreter for measurement ID %d",
			uniqueID,
		),
	)

	return nil
}

// processMeasureRequest processes the measurement request
// TODO: Implement actual measurement logic
func (h *MeasureCommandHandler) processMeasureRequest(request string) string {
	// For now, just echo the request with a timestamp
	// In a real implementation, this would interface with instruments
	return fmt.Sprintf(
		"Processed request: %s at %d",
		request,
		time.Now().UnixMicro(),
	)
}
