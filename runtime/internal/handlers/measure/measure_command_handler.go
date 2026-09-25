package measurehandlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/databuffer"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/interpreter"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
)

const (
	MeasureCommandHandlerName = "MEASURE_COMMAND_HANDLER"
	// INSTRUMENTHUB.MEASURE_COMMAND is the subject published by falcon-comms
	// RoutineComms on the controller side (routine_comms.cpp make_measure_command_subject).
	MeasureCommandSubject = "INSTRUMENTHUB.MEASURE_COMMAND"
	// FALCON.MEASURE_RESPONSE.<timestamp> is subscribed to by falcon-comms
	// RoutineComms on the controller side.
	MeasureResponseSubject = "FALCON.MEASURE_RESPONSE"
	MeasureCommandName     = "MEASURE_COMMAND"
	MeasureResponseName    = "MEASURE_RESPONSE"
)

// BusyManager interface allows the handler to manage busy state
type BusyManager interface {
	SetIsBusy(busy bool)
}

func measurementResponseSubject(timestamp int64) string {
	return MeasureResponseSubject + "." + strconv.FormatInt(timestamp, 10)
}

// Allows us to inject mocks instead of real FalconRequest
type FalconRequest interface {
	Close() error
}

var _ FalconRequest = (*interpreter.FalconMeasurementRequest)(nil)

// Allows us to inject mocks instead of real FalconResponse
type FalconResponse interface {
	ToJSON() (string, error)
	Close() error
}

var _ FalconResponse = (*interpreter.FalconMeasurementResponse)(nil)

type FalconRequestFactory interface {
	FromJSON(string) (FalconRequest, error)
}

type falconRequestFactory struct{}

func (falconRequestFactory) FromJSON(
	jsonStr string,
) (FalconRequest, error) {
	return interpreter.NewFalconMeasurementRequestFromJSON(
		jsonStr,
	)
}

var _ FalconRequestFactory = (*falconRequestFactory)(nil)

type MeasurementRouter interface {
	Handle(
		FalconRequest,
	) (FalconResponse, error)
}

type routerAdapter struct {
	router *interpreter.Router
}

func (r *routerAdapter) Handle(
	req FalconRequest,
) (FalconResponse, error) {
	falconReq, ok := req.(*interpreter.FalconMeasurementRequest)
	if !ok {
		return nil,
			fmt.Errorf(
				"expected *FalconMeasurementRequest, got %T",
				req,
			)
	}

	return r.router.Handle(
		falconReq,
	)
}

var _ MeasurementRouter = (*routerAdapter)(nil)

type MeasurementDispatcher interface {
	RunAll(
		requests []interpreter.MeasurementRequest,
	) []interpreter.MeasurementResult
}

var _ MeasurementDispatcher = (*interpreter.MeasurementDispatcher)(nil)

type BufferRegistrar interface {
	RegisterBuffer(
		requestorID string,
		bufferID string,
	) error
}

var _ BufferRegistrar = (*databuffer.DataBufferManager)(nil)

type MeasurementClient interface {
	Measure(
		scriptPath string,
		variables []instrumentserver.MeasureVariable,
	) ([]instrumentserver.CallResult, error)

	ReleaseBuffer(
		bufferID string,
	) error
}

// Handler handles MEASURE_COMMAND requests
type Handler struct {
	logger         *logging.Logger
	nc             *nats.Conn
	js             nats.JetStreamContext
	subscription   *nats.Subscription
	busyManager    BusyManager
	wiremap        *config.WireMap
	ports          *ports.ConnectedPorts
	router         MeasurementRouter
	requestFactory FalconRequestFactory
}

func newMeasureCommandHandler(
	logger *logging.Logger,
	busyManager BusyManager,
	router MeasurementRouter,
	requestFactory FalconRequestFactory,
	wireMap *config.WireMap,
	ports *ports.ConnectedPorts,
) *Handler {
	return &Handler{
		logger:         logger,
		busyManager:    busyManager,
		wiremap:        wireMap,
		ports:          ports,
		router:         router,
		requestFactory: requestFactory,
	}
}

// NewMeasureCommandHandler creates a new handler
func NewMeasureCommandHandler(
	logger *logging.Logger,
	busyManager BusyManager,
	scriptsPath string,
	issClient MeasurementClient,
	wireMap *config.WireMap,
	ports *ports.ConnectedPorts,
) *Handler {
	bufferManager := databuffer.NewDataBufferManager(
		issClient,
	)

	dispatcher := interpreter.NewMeasurementDispatcher(
		issClient,
		bufferManager,
		scriptsPath,
	)

	router := &routerAdapter{
		router: interpreter.NewRouter(
			dispatcher,
			wireMap,
			ports,
		),
	}

	return newMeasureCommandHandler(
		logger,
		busyManager,
		router,
		falconRequestFactory{},
		wireMap,
		ports,
	)
}

// Subscribe starts listening for MEASURE_COMMAND requests
func (h *Handler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error

	h.js, err = nc.JetStream()
	if err != nil {
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}
	_, addErr := h.js.AddStream(&nats.StreamConfig{
		Name:     "FALCON_MEASURE",
		Subjects: []string{"FALCON.MEASURE_DATA.*"},
		MaxAge:   60 * time.Second,
	})
	if addErr != nil && addErr != nats.ErrStreamNameAlreadyInUse {
		return fmt.Errorf("failed to ensure FALCON_MEASURE stream: %w", addErr)
	}

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

	h.logger.Info(
		MeasureCommandHandlerName,
		"Subscribed to "+MeasureCommandSubject,
	)
	return nil
}

// Unsubscribe stops listening for commands
func (h *Handler) Unsubscribe() error {
	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
			return fmt.Errorf("failed to unsubscribe: %w", err)
		}
		h.subscription = nil
	}

	h.logger.Info(
		MeasureCommandHandlerName,
		"Unsubscribed from "+MeasureCommandSubject,
	)
	return nil
}

func (h *Handler) publishMeasurementResponse(cmd api.MeasureCommand, responseSubject, respJSON string) bool {
	measureSubject := "FALCON.MEASURE_DATA." + strconv.FormatInt(cmd.Timestamp, 10)
	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Publishing measurement data: subject=%s bytes=%d", measureSubject, len(respJSON)))
	if _, err := h.js.Publish(measureSubject, []byte(respJSON)); err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to publish measurement to JetStream subject %s: %v", measureSubject, err))
		return false
	}
	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Published measurement data: subject=%s", measureSubject))

	measureResp := api.MeasureResponse{
		Stream:    measureSubject,
		Response:  respJSON,
		Timestamp: cmd.Timestamp,
		Hash:      cmd.Hash,
	}
	respData, err := json.Marshal(measureResp)
	if err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to marshal MeasureResponse: %v", err))
		return false
	}

	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Publishing %s: subject=%s stream=%s bytes=%d", MeasureResponseName, responseSubject, measureSubject, len(respData)))
	if err := h.nc.Publish(responseSubject, respData); err != nil {
		h.logger.Error(MeasureCommandHandlerName,
			fmt.Sprintf("failed to publish %s: %v", responseSubject, err))
		return false
	}
	h.logger.Info(MeasureCommandHandlerName,
		fmt.Sprintf("Published %s: subject=%s stream=%s", MeasureResponseName, responseSubject, measureSubject))
	return true
}

func (h *Handler) handleMessage(msg *nats.Msg) {
	h.logger.Debug(
		MeasureCommandHandlerName,
		fmt.Sprintf("Received command: %s", string(msg.Data)),
	)

	var cmd api.MeasureCommand
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf(
				"failed to unmarshal MEASURE_COMMAND: %v",
				err,
			),
		)
		return
	}

	if cmd.Request == "" {
		h.logger.Debug(
			MeasureCommandHandlerName,
			"empty request, ignoring",
		)
		return
	}

	responseSubject := measurementResponseSubject(
		cmd.Timestamp,
	)

	h.busyManager.SetIsBusy(true)
	defer h.busyManager.SetIsBusy(false)

	falconReq, err := h.requestFactory.FromJSON(
		cmd.Request,
	)
	if err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf(
				"failed to parse MeasurementRequest: %v",
				err,
			),
		)
		return
	}
	defer falconReq.Close()

	falconResp, err := h.router.Handle(
		falconReq,
	)
	if err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf(
				"measurement routing failed: %v",
				err,
			),
		)
		return
	}
	defer falconResp.Close()

	respJSON, err := falconResp.ToJSON()
	if err != nil {
		h.logger.Error(
			MeasureCommandHandlerName,
			fmt.Sprintf(
				"failed to encode MeasurementResponse: %v",
				err,
			),
		)
		return
	}

	h.publishMeasurementResponse(
		cmd,
		responseSubject,
		respJSON,
	)
}
