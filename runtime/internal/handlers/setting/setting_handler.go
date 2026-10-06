package settinghandler

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/databuffer"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settinginterpreter"
)

const (
	SettingCommandHandlerName = "SETTING_COMMAND_HANDLER"
	// INSTRUMENTHUB.SETTING_COMMAND is the subject published by falcon-comms
	// RoutineComms on the controller side (routine_comms.cpp make_measure_command_subject).
	SettingCommandSubject = "INSTRUMENTHUB.SETTING_COMMAND"
	// FALCON.SETTING_RESPONSE.<timestamp> is subscribed to by falcon-comms
	// RoutineComms on the controller side.
	SettingResponseSubject = "FALCON.SETTING_RESPONSE"
	SettingCommandName     = "SETTING_COMMAND"
	SettingResponseName    = "SETTING_RESPONSE"
)

// BusyManager interface allows the handler to manage busy state
type BusyManager interface {
	SetIsBusy(busy bool)
}

func settingResponseSubject(timestamp int64) string {
	return SettingResponseSubject + "." + strconv.FormatInt(timestamp, 10)
}

// Allows us to inject mocks instead of real FalconRequest
type FalconRequest interface {
	Close() error
	Setters() (map[config.ConnectedPort]settinginterpreter.Quantity, error)
	Getters() ([]config.ConnectedPort, error)
}

var _ FalconRequest = (*settinginterpreter.FalconSettingRequest)(nil)

type FalconRequestFactory interface {
	FromJSON(string) (FalconRequest, error)
}

type falconRequestFactory struct{}

func (falconRequestFactory) FromJSON(
	jsonStr string,
) (FalconRequest, error) {
	return settinginterpreter.NewFalconSettingRequestFromJSON(
		jsonStr)
}

var _ FalconRequestFactory = (*falconRequestFactory)(nil)

// Allows us to inject mocks instead of real FalconResponse
type FalconResponse interface {
	ToJSON() (string, error)
	Close() error
}

var _ FalconResponse = (*settinginterpreter.FalconSettingResponse)(nil)

type SettingRouter interface {
	Handle(
		FalconRequest,
	) (FalconResponse, error)
}

type routerAdapter struct {
	router *settinginterpreter.Router
}

func (r *routerAdapter) Handle(
	req FalconRequest,
) (FalconResponse, error) {
	falconReq, ok := req.(*settinginterpreter.FalconSettingRequest)
	if !ok {
		return nil,
			fmt.Errorf(
				"expected *FalconSettingRequest, got %T",
				req,
			)
	}

	return r.router.Handle(
		falconReq,
	)
}

var _ SettingRouter = (*routerAdapter)(nil)

type BufferRegistrar interface {
	RegisterBuffer(
		requestorID string,
		bufferID string,
	) error
}

var _ BufferRegistrar = (*databuffer.DataBufferManager)(nil)

type SettingClient interface {
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
	wiremap        config.WireMap
	ports          *config.ConnectedPorts
	requestFactory FalconRequestFactory
	dispatcher     dispatcher.MeasurementDispatcher
	router         SettingRouter
}

func newSettingCommandHandler(
	logger *logging.Logger,
	busyManager BusyManager,
	router SettingRouter,
	requestFactory FalconRequestFactory,
	wireMap config.WireMap,
	ports *config.ConnectedPorts,
	dispatcher dispatcher.MeasurementDispatcher,
) *Handler {
	return &Handler{
		logger:         logger,
		busyManager:    busyManager,
		wiremap:        wireMap,
		ports:          ports,
		requestFactory: requestFactory,
		router:         router,
	}
}

// NewSettingCommandHandler creates a new handler
func NewSettingCommandHandler(
	logger *logging.Logger,
	busyManager BusyManager,
	scriptsPath string,
	issClient SettingClient,
	wireMap config.WireMap,
	ports *config.ConnectedPorts,
) *Handler {
	bufferManager := databuffer.NewDataBufferManager(
		issClient,
	)

	measurementDispatcher := dispatcher.NewMeasurementDispatcher(
		issClient,
		bufferManager,
		scriptsPath,
	)

	router := &routerAdapter{
		router: settinginterpreter.NewRouter(
			measurementDispatcher,
			wireMap,
			ports,
		),
	}

	return newSettingCommandHandler(
		logger,
		busyManager,
		router,
		falconRequestFactory{},
		wireMap,
		ports,
		*measurementDispatcher,
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
		SettingCommandSubject,
		h.handleMessage,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+SettingCommandSubject+": %w",
			err,
		)
	}

	h.logger.Info(
		SettingCommandHandlerName,
		"Subscribed to "+SettingCommandSubject,
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
		SettingCommandHandlerName,
		"Unsubscribed from "+SettingCommandSubject,
	)
	return nil
}

func (h *Handler) publishSettingmentResponse(cmd api.SettingCommand, responseSubject, respJSON string) bool {
	measureResp := api.SettingResponse{
		Response:  respJSON,
		Timestamp: cmd.Timestamp,
		Hash:      cmd.Hash,
	}
	respData, err := json.Marshal(measureResp)
	if err != nil {
		h.logger.Error(SettingCommandHandlerName,
			fmt.Sprintf("failed to marshal SettingResponse: %v", err))
		return false
	}

	h.logger.Info(SettingCommandHandlerName,
		fmt.Sprintf("Publishing %s: subject=%s bytes=%d", SettingResponseName, responseSubject, len(respData)))
	if err := h.nc.Publish(responseSubject, respData); err != nil {
		h.logger.Error(SettingCommandHandlerName,
			fmt.Sprintf("failed to publish %s: %v", responseSubject, err))
		return false
	}
	h.logger.Info(SettingCommandHandlerName,
		fmt.Sprintf("Published %s: subject=%s ", SettingResponseName, responseSubject))
	return true
}

func (h *Handler) handleMessage(msg *nats.Msg) {
	h.logger.Debug(
		SettingCommandHandlerName,
		fmt.Sprintf("Received command: %s", string(msg.Data)),
	)

	var cmd api.SettingCommand
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to unmarshal MEASURE_COMMAND: %v",
				err,
			),
		)
		return
	}

	if cmd.Request == "" {
		h.logger.Debug(
			SettingCommandHandlerName,
			"empty request, ignoring",
		)
		return
	}

	responseSubject := settingResponseSubject(
		cmd.Timestamp,
	)

	h.busyManager.SetIsBusy(true)
	defer h.busyManager.SetIsBusy(false)

	falconReq, err := h.requestFactory.FromJSON(
		cmd.Request,
	)
	if err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to parse SettingmentRequest: %v",
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
		h.logger.Error(SettingCommandHandlerName, fmt.Sprintf("failed to build new falcon response %v", err))
		return
	}
	defer falconResp.Close()

	respJSON, err := falconResp.ToJSON()
	if err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to encode SettingmentResponse: %v",
				err,
			),
		)
		return
	}

	h.publishSettingmentResponse(
		cmd,
		responseSubject,
		respJSON,
	)
}
