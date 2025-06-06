package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

const (
	BusyHandlerName    = "BUSY_HANDLER"
	busySegment        = "BUSY"
	busyExternalPrefix = busySegment + "." + externalSegment
	busyPattern        = busyExternalPrefix + ".*"
)

// BusyHandler handles BUSY.external.<name> messages
type BusyHandler struct {
	logger       *logging.Logger
	nc           *nats.Conn
	subscription *nats.Subscription
	busyState    *bool // Direct reference to manager's busy state
}

// NewBusyHandler creates a new busy handler
func NewBusyHandler(logger *logging.Logger, busyState *bool) *BusyHandler {
	return &BusyHandler{
		logger:    logger,
		busyState: busyState,
	}
}

// Subscribe subscribes to BUSY.external.* channels
func (h *BusyHandler) Subscribe(nc *nats.Conn) error {
	h.nc = nc

	sub, err := nc.Subscribe(busyPattern, h.handleBusyRequest)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", busyPattern, err)
	}
	h.subscription = sub

	h.logger.Info(
		BusyHandlerName,
		fmt.Sprintf("Subscribed to %s channels", busyPattern),
	)

	return nil
}

// Unsubscribe unsubscribes from BUSY.external.* channels
func (h *BusyHandler) Unsubscribe() error {
	if h.subscription != nil {
		err := h.subscription.Unsubscribe()
		if err != nil {
			h.logger.Error(
				BusyHandlerName,
				fmt.Sprintf("Failed to unsubscribe: %v", err),
			)
			return err
		}
		h.logger.Info(
			BusyHandlerName,
			fmt.Sprintf("Unsubscribed from %s channels", busyPattern),
		)
		h.subscription = nil
	}
	return nil
}

// handleBusyRequest processes incoming BUSY messages
func (h *BusyHandler) handleBusyRequest(msg *nats.Msg) {
	h.logger.Info(
		BusyHandlerName,
		fmt.Sprintf("Received BUSY on subject: %s", msg.Subject),
	)

	var req api.Busy
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		h.logger.Error(
			BusyHandlerName,
			fmt.Sprintf("Failed to unmarshal BUSY request: %v", err),
		)
		return
	}

	// Check if the system is busy
	isBusy := h.busyState != nil && *h.busyState

	if isBusy {
		fmt.Println("Currently busy")
	} else {
		fmt.Println("Accepting measurements")
	}

	h.logger.Info(
		BusyHandlerName,
		fmt.Sprintf("BUSY status checked - busy: %t", isBusy),
	)
}

// GetSubscription returns the current subscription (for testing)
func (h *BusyHandler) GetSubscription() *nats.Subscription {
	return h.subscription
}
