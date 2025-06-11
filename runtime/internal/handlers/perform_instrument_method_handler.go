package handlers

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	PerformInstrumentMethodHandlerName = "PERFORM_INSTRUMENT_METHOD_HANDLER"
	PerformInstrumentMethodSubject     = "PERFORM_INSTRUMENT_METHOD.external.instrument-server"
	PerformArbitraryMethodSubject      = "PERFORM_ARBITRARY_METHOD"
)

// PerformInstrumentMethodHandler handles PERFORM_INSTRUMENT_METHOD commands
type PerformInstrumentMethodHandler struct {
	logger            *logging.Logger
	nc                *nats.Conn
	subscription      *nats.Subscription
	instrumentHandler *instrument.Handler
}

// NewPerformInstrumentMethodHandler creates a new handler
func NewPerformInstrumentMethodHandler(
	logger *logging.Logger,
	instrumentHandler *instrument.Handler,
) *PerformInstrumentMethodHandler {
	return &PerformInstrumentMethodHandler{
		logger:            logger,
		instrumentHandler: instrumentHandler,
	}
}

// Subscribe starts listening for PERFORM_INSTRUMENT_METHOD commands
func (h *PerformInstrumentMethodHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	var err error
	h.subscription, err = nc.Subscribe(
		PerformInstrumentMethodSubject,
		h.handleMessage,
	)
	if err != nil {
		return fmt.Errorf(
			"failed to subscribe to "+PerformInstrumentMethodSubject+": %w",
			err,
		)
	}

	h.logger.Info(
		PerformInstrumentMethodHandlerName,
		"Subscribed to "+PerformInstrumentMethodSubject,
	)
	return nil
}

// Unsubscribe stops listening for commands
func (h *PerformInstrumentMethodHandler) Unsubscribe() error {
	if h.subscription != nil {
		if err := h.subscription.Unsubscribe(); err != nil {
			h.logger.Error(
				PerformInstrumentMethodHandlerName,
				fmt.Sprintf("Failed to unsubscribe: %v", err),
			)
			return err
		}
		h.subscription = nil
	}

	h.logger.Info(
		PerformInstrumentMethodHandlerName,
		fmt.Sprintf("Unsubscribed from %s",
			PerformArbitraryMethodSubject),
	)
	return nil
}

// handleMessage processes incoming PERFORM_INSTRUMENT_METHOD commands
func (h *PerformInstrumentMethodHandler) handleMessage(msg *nats.Msg) {
	h.logger.Debug(
		PerformInstrumentMethodHandlerName,
		fmt.Sprintf("Received command: %s", string(msg.Data)),
	)

	// Parse the incoming message
	var performInstrMethod api.PerformInstrumentMethod
	if err := json.Unmarshal(msg.Data, &performInstrMethod); err != nil {
		h.logger.Error(
			PerformInstrumentMethodHandlerName,
			fmt.Sprintf(
				"Failed to unmarshal %s: %v",
				PerformArbitraryMethodSubject,
				err,
			),
		)
		return
	}

	// Check if the instrument is active
	activeInstruments := h.instrumentHandler.GetActiveInstruments()

	if !slices.Contains(
		activeInstruments,
		instrument.Name(performInstrMethod.Instrument),
	) {

		h.logger.Error(
			PerformInstrumentMethodHandlerName,
			fmt.Sprintf("Instrument '%s' is not active. Active instruments: %v",
				performInstrMethod.Instrument, activeInstruments),
		)
		return
	}

	// Create the PerformArbitraryMethod command to forward
	arbitraryMethod := api.PerformArbitraryMethod{
		Method:      performInstrMethod.Method,
		KeywordArgs: performInstrMethod.KeywordArgs,
		Timestamp:   performInstrMethod.Timestamp,
	}

	// Marshal the command
	arbitraryMethodData, err := json.Marshal(arbitraryMethod)
	if err != nil {
		h.logger.Error(
			PerformInstrumentMethodHandlerName,
			fmt.Sprintf(
				"Failed to marshal %s: %v",
				PerformArbitraryMethodSubject,
				err,
			),
		)
		return
	}

	// Forward to the specific instrument
	subject := fmt.Sprintf(
		"%s.%s",
		PerformArbitraryMethodSubject,
		performInstrMethod.Instrument,
	)
	if err := h.nc.Publish(subject, arbitraryMethodData); err != nil {
		h.logger.Error(
			PerformInstrumentMethodHandlerName,
			fmt.Sprintf("Failed to forward command to %s: %v", subject, err),
		)
		return
	}

	h.logger.Info(
		PerformInstrumentMethodHandlerName,
		fmt.Sprintf("Forwarded method '%s' to instrument '%s' on subject '%s'",
			performInstrMethod.Method, performInstrMethod.Instrument, subject),
	)
}
