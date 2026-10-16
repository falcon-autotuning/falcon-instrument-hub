package settinghandler

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingrequest"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/databuffer"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	SettingCommandHandlerName = "SETTING_COMMAND_HANDLER"
	// INSTRUMENTHUB.SETTING_COMMAND is the subject published by
	// falcon-comms RoutineComms on the controller side (routine_comms.cpp
	// make_measure_command_subject).
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

type SettingRouter interface {
	Handle(
		falconcore.SettingRequest,
	) (falconcore.SettingResponse, error)
}

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
	logger       *logging.Logger
	nc           *nats.Conn
	subscription *nats.Subscription
	busyManager  BusyManager
	router       SettingRouter
}

// NewSettingCommandHandler creates a new handler
func NewSettingCommandHandler(
	logger *logging.Logger,
	busyManager BusyManager,
	router SettingRouter,
) *Handler {
	return &Handler{
		logger:      logger,
		busyManager: busyManager,
		router:      router,
	}
}

// Subscribe starts listening for MEASURE_COMMAND requests
func (h *Handler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error

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

func (h *Handler) publishSettingmentResponse(
	cmd api.SettingCommand,
	responseSubject, respJSON string,
) bool {
	measureResp := api.SettingResponse{
		Response:  respJSON,
		Timestamp: cmd.Timestamp,
		Hash:      cmd.Hash,
	}
	respData, err := json.Marshal(measureResp)
	if err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to marshal SettingResponse: %v",
				err,
			),
		)
		return false
	}

	h.logger.Info(
		SettingCommandHandlerName,
		fmt.Sprintf(
			"Publishing %s: subject=%s bytes=%d",
			SettingResponseName,
			responseSubject,
			len(respData),
		),
	)
	if err := h.nc.Publish(responseSubject, respData); err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to publish %s: %v",
				responseSubject,
				err,
			),
		)
		return false
	}
	h.logger.Info(
		SettingCommandHandlerName,
		fmt.Sprintf(
			"Published %s: subject=%s ",
			SettingResponseName,
			responseSubject,
		),
	)
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

	rawReq, err := settingrequest.FromJSON(cmd.Request)
	if err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to parse SettingRequest: %v",
				err,
			),
		)
		return
	}
	defer rawReq.Close()

	falconReq, err := falconcore.SettingRequestFromFalcon(
		rawReq,
	)
	if err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to convert SettingRequest: %v",
				err,
			),
		)
		return
	}

	falconResp, err := h.router.Handle(
		falconReq,
	)
	if err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to build new falcon response %v",
				err,
			),
		)
		return
	}

	rawResp, err := falconResp.NewFalconSettingResponse()
	if err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to build falcon response: %v",
				err,
			),
		)
		return
	}
	defer rawResp.Close()

	respJSON, err := rawResp.ToJSON()
	if err != nil {
		h.logger.Error(
			SettingCommandHandlerName,
			fmt.Sprintf(
				"failed to encode SettingResponse: %v",
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
