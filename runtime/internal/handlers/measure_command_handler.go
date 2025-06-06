package handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
)

const (
	MeasureCommandHandlerName = "MEASURE_COMMAND_HANDLER"
	MeasureCommandSubject     = "MEASURE_COMMAND.external"
	MeasureResponseSubject    = "MEASURE_RESPONSE.external"
	ProcessRequestSubject     = "PROCESS_REQUEST.interpreter"
	UploadDataSubject         = "UPLOAD_DATA"
	MeasureCommandName        = "MEASURE_COMMAND"
	MeasureResponseName       = "MEASURE_RESPONSE"
	ProcessRequestName        = "PROCESS_REQUEST"
	UploadDataName            = "UPLOAD_DATA"
)

// BusyManager interface allows the handler to manage busy state
type BusyManager interface {
	SetIsBusy(busy bool)
}

// PendingMeasurement tracks measurements waiting for UPLOAD_DATA
type PendingMeasurement struct {
	Hash         int64
	ResponseName string
	ProcessId    string
}

// MeasureCommandHandler handles MEASURE_COMMAND requests
type MeasureCommandHandler struct {
	logger              *logging.Logger
	nc                  *nats.Conn
	subscription        *nats.Subscription
	uploadSubscription  *nats.Subscription
	measurementManager  *measurements.Manager
	instrumentHandler   *instrument.Handler
	busyManager         BusyManager
	pendingMeasurements map[string]PendingMeasurement
	mutex               sync.RWMutex
}

// NewMeasureCommandHandler creates a new handler
func NewMeasureCommandHandler(
	logger *logging.Logger,
	measurementManager *measurements.Manager,
	instrumentHandler *instrument.Handler,
	busyManager BusyManager,
) *MeasureCommandHandler {
	return &MeasureCommandHandler{
		logger:              logger,
		measurementManager:  measurementManager,
		instrumentHandler:   instrumentHandler,
		busyManager:         busyManager,
		pendingMeasurements: make(map[string]PendingMeasurement),
	}
}

// Subscribe starts listening for MEASURE_COMMAND requests and UPLOAD_DATA
func (h *MeasureCommandHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error

	// Subscribe to MEASURE_COMMAND
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

	// Subscribe to UPLOAD_DATA
	h.uploadSubscription, err = nc.Subscribe(
		UploadDataSubject,
		h.handleUploadData,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+UploadDataSubject+": %w",
			err,
		)
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		"Subscribed to "+MeasureCommandSubject+".> and "+UploadDataSubject,
	)
	return nil
}

// Unsubscribe stops listening for commands
func (h *MeasureCommandHandler) Unsubscribe() error {
	var errs []error

	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
			errs = append(errs, err)
		}
		h.subscription = nil
	}

	if h.uploadSubscription != nil {
		if err := h.uploadSubscription.Unsubscribe(); err != nil {
			errs = append(errs, err)
		}
		h.uploadSubscription = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to unsubscribe: %v", errs)
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		"Unsubscribed from "+MeasureCommandSubject+" and "+UploadDataSubject,
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

	// Set IsBusy flag to true when starting measurement
	h.busyManager.SetIsBusy(true)
	h.logger.Debug(
		MeasureCommandHandlerName,
		"Set IsBusy flag to true - measurement started",
	)

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
		// Reset IsBusy flag on error
		h.busyManager.SetIsBusy(false)
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

	// Store pending measurement for correlation with UPLOAD_DATA
	processId := strconv.Itoa(uniqueID)
	h.mutex.Lock()
	h.pendingMeasurements[processId] = PendingMeasurement{
		Hash:         measureCommand.Hash,
		ResponseName: name,
		ProcessId:    processId,
	}
	h.mutex.Unlock()

	// Send PROCESS_REQUEST to interpreter
	if err := h.sendProcessRequest(measureCommand.Request, uniqueID, expectedPath); err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf("Failed to send PROCESS_REQUEST: %v", err),
		)
		// Clean up pending measurement and reset IsBusy flag on error
		h.mutex.Lock()
		delete(h.pendingMeasurements, processId)
		h.mutex.Unlock()
		h.busyManager.SetIsBusy(false)
		return
	}
}

// handleUploadData processes incoming UPLOAD_DATA and sends MEASURE_RESPONSE
func (h *MeasureCommandHandler) handleUploadData(msg *nats.Msg) {
	h.logger.Debug(
		MeasureCommandHandlerName,
		fmt.Sprintf("Received %s: %s", UploadDataName, string(msg.Data)),
	)

	// TODO: Update the data stored in the database to have a copy of the real
	// config.

	// Parse the incoming UPLOAD_DATA
	var uploadData api.UploadData
	if err := json.Unmarshal(msg.Data, &uploadData); err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf("Failed to unmarshal %s: %v", UploadDataName, err),
		)
		return
	}

	// We need to identify which measurement this belongs to
	// For now, we'll assume the latest pending measurement
	h.mutex.Lock()
	var pendingMeasurement PendingMeasurement
	var found bool

	// Find the first pending measurement (in a real implementation,
	// you'd need a better correlation mechanism)
	for pid, pm := range h.pendingMeasurements {
		pendingMeasurement = pm
		found = true
		delete(h.pendingMeasurements, pid)
		break
	}
	h.mutex.Unlock()

	if !found {
		h.logger.Error(
			MeasureCommandHandlerName,
			"Received "+UploadDataName+" but no pending measurements found",
		)
		return
	}

	// Create the response using the uploaded data
	measureResponse := api.MeasureResponse{
		Response:  uploadData.Data, // Forward the uploaded data
		Timestamp: time.Now().UnixMicro(),
		Hash:      pendingMeasurement.Hash,
	}

	// Marshal the response
	responseData, err := json.Marshal(measureResponse)
	if err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf("Failed to marshal %s: %v", MeasureResponseName, err),
		)
		return
	}

	// Send response on MEASURE_RESPONSE.external.<name>
	responseSubject := fmt.Sprintf(
		"%s.%s",
		MeasureResponseSubject,
		pendingMeasurement.ResponseName,
	)
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

	// Reset IsBusy flag to false when measurement response is sent
	h.busyManager.SetIsBusy(false)
	h.logger.Debug(
		MeasureCommandHandlerName,
		"Set IsBusy flag to false - measurement completed",
	)

	h.logger.Info(
		MeasureCommandHandlerName,
		fmt.Sprintf(
			"Sent %s to %s for hash %d with uploaded data",
			MeasureResponseName,
			responseSubject,
			pendingMeasurement.Hash,
		),
	)
}

// sendProcessRequest sends a PROCESS_REQUEST to the interpreter
func (h *MeasureCommandHandler) sendProcessRequest(
	request string,
	uniqueID int,
	dataPath string,
) error {
	// Build configurations from current instrument ports
	configurations, err := h.instrumentHandler.BuildConfigurations()
	if err != nil {
		return fmt.Errorf("failed to build configurations: %w", err)
	}

	// Marshal configurations to JSON string
	configurationsJSON, err := json.Marshal(configurations)
	if err != nil {
		return fmt.Errorf("failed to marshal configurations: %w", err)
	}

	// Create the ProcessRequest
	processRequest := api.ProcessRequest{
		Request:        request,
		Configurations: string(configurationsJSON),
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
			"Sent PROCESS_REQUEST to interpreter for measurement ID %d with %d port configurations",
			uniqueID,
			len(configurations),
		),
	)

	return nil
}
