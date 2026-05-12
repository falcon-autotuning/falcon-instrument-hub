package handlers

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/serverinterpreter"
)

const (
	MeasureCommandHandlerName = "MEASURE_COMMAND_HANDLER"
	// INSTRUMENTHUB.MEASURE_COMMAND is the subject published by falcon-comms
	// RoutineComms on the controller side (routine_comms.cpp make_measure_command_subject).
	MeasureCommandSubject = "INSTRUMENTHUB.MEASURE_COMMAND"
	// FALCON.MEASURE_RESPONSE is the subject subscribed to by falcon-comms
	// RoutineComms on the controller side (routine_comms.cpp make_measure_response_subject).
	MeasureResponseSubject = "FALCON.MEASURE_RESPONSE"
	ProcessRequestSubject  = "PROCESS_REQUEST"
	UploadDataSubject      = "UPLOAD_DATA"
	MeasureCommandName     = "MEASURE_COMMAND"
	MeasureResponseName    = "MEASURE_RESPONSE"
	ProcessRequestName     = "PROCESS_REQUEST"
	UploadDataName         = "UPLOAD_DATA"
)

// BusyManager interface allows the handler to manage busy state
type BusyManager interface {
	SetIsBusy(busy bool)
}

// PendingMeasurement tracks measurements waiting for UPLOAD_DATA
type PendingMeasurement struct {
	Hash      int64
	ProcessId instrument.ID
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
	dispatcher          *serverinterpreter.ScriptDispatcher
	pendingMeasurements map[instrument.ID]PendingMeasurement
	mutex               sync.RWMutex
}

// NewMeasureCommandHandler creates a new handler
func NewMeasureCommandHandler(
	logger *logging.Logger,
	measurementManager *measurements.Manager,
	instrumentHandler *instrument.Handler,
	busyManager BusyManager,
	dispatcher *serverinterpreter.ScriptDispatcher,
) *MeasureCommandHandler {
	return &MeasureCommandHandler{
		logger:              logger,
		measurementManager:  measurementManager,
		instrumentHandler:   instrumentHandler,
		busyManager:         busyManager,
		dispatcher:          dispatcher,
		pendingMeasurements: make(map[instrument.ID]PendingMeasurement),
	}
}

// Subscribe starts listening for MEASURE_COMMAND requests and UPLOAD_DATA
func (h *MeasureCommandHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error

	// Subscribe to MEASURE_COMMAND (flat subject — no wildcard suffix needed)
	h.subscription, err = nc.Subscribe(
		MeasureCommandSubject,
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

	// Subject is the flat INSTRUMENTHUB.MEASURE_COMMAND — no name suffix.
	// Use the Hash field from the command payload as the correlation key.

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
	processId := instrument.ID(uniqueID)
	h.mutex.Lock()
	h.pendingMeasurements[processId] = PendingMeasurement{
		Hash:      measureCommand.Hash,
		ProcessId: processId,
	}
	h.mutex.Unlock()

	// Send PROCESS_REQUEST to interpreter
	if err := h.sendProcessRequest(measureCommand.Request, processId, expectedPath); err != nil {
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
	var found bool
	var pendingMeasurement PendingMeasurement
	id := instrument.ID(uploadData.ProcessId)
	h.mutex.Lock()
	if pM, exists := h.pendingMeasurements[id]; exists {
		pendingMeasurement = pM
		found = true
		delete(h.pendingMeasurements, id)
	}
	h.mutex.Unlock()

	if !found {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf(
				"Received %s but no pending measurements found for ProcessId %d. The available IDs are %v",
				UploadDataName,
				id,
				getKeys(h.pendingMeasurements),
			),
		)
		return
	}

	// Create the response using the uploaded data
	_ = pendingMeasurement.Hash // TODO: use hash in new measure_command_handler implementation
	measureResponse := api.MeasureResponse{
		Stream:    uploadData.Data,
		Timestamp: time.Now().UnixMicro(),
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

	// Send response on FALCON.MEASURE_RESPONSE (flat subject — no name suffix).
	// The controller's falcon-comms RoutineComms subscribes on this exact subject.
	if err := h.nc.Publish(MeasureResponseSubject, responseData); err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf(
				"Failed to publish response to %s: %v",
				MeasureResponseSubject,
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
			MeasureResponseSubject,
			pendingMeasurement.Hash,
		),
	)
}

func getKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// sendProcessRequest sends a PROCESS_REQUEST to the interpreter
func (h *MeasureCommandHandler) sendProcessRequest(
	request string,
	uniqueID instrument.ID,
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
		ProcessId:      int64(uniqueID),
	}

	// Marshal the request
	requestData, err := json.Marshal(processRequest)
	if err != nil {
		return fmt.Errorf(
			"failed to marshal %s : %w",
			ProcessRequestSubject,
			err,
		)
	}

	// Send to interpreter
	if err := h.nc.Publish(ProcessRequestSubject, requestData); err != nil {
		return fmt.Errorf(
			"failed to publish %s : %w",
			ProcessRequestSubject,
			err,
		)
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		fmt.Sprintf(
			"Sent %s to interpreter for measurement ID %d with %d port configurations",
			ProcessRequestSubject,
			uniqueID,
			len(configurations),
		),
	)
	if len(configurations) == 0 {
		h.logger.Error(
			MeasureCommandHandlerName,
			"0 port configurations. Did you set up your instruments correctly?",
		)
	}

	return nil
}
