package instrument

import (
	"fmt"

	"github.com/nats-io/nats.go"
)

// Subscribe sets up NATS subscriptions for instrument commands
func (h *Handler) Subscribe(nc *nats.Conn) error {
	h.logger.Info(
		HandlerName,
		"Setting up instrument handler subscriptions",
	)

	// Ensure script is up to date
	if err := h.ensureScriptExists(); err != nil {
		return fmt.Errorf("failed to ensure script exists: %w", err)
	}

	// Configure all subscriptions
	subscriptions := h.getSubscriptionConfigs()
	go h.cleanupLoop()

	// Subscribe to each configured subscription
	for _, config := range subscriptions {
		sub, err := nc.Subscribe(config.subject, config.handler)
		if err != nil {
			return fmt.Errorf("failed to subscribe to %s: %w", config.name, err)
		}
		h.subscriptions = append(h.subscriptions, sub)

		h.logger.Info(
			HandlerName,
			fmt.Sprintf(
				"Subscribed to %s on subject: %s",
				config.name,
				config.subject,
			),
		)
	}

	h.logger.Info(
		HandlerName,
		"Instrument handler subscriptions ready",
	)
	return nil
}

// Unsubscribe removes NATS subscriptions
func (h *Handler) Unsubscribe() error {
	h.logger.Info(
		HandlerName,
		"Unsubscribing from instrument handler",
	)

	// Unsubscribe from all subscriptions
	for _, sub := range h.subscriptions {
		if sub != nil {
			if err := sub.Unsubscribe(); err != nil {
				h.logger.Error(
					HandlerName,
					fmt.Sprintf("Failed to unsubscribe: %v", err),
				)
			}
		}
	}
	h.subscriptions = nil

	// Clean up all running instruments
	h.mutex.Lock()
	defer h.mutex.Unlock()

	for name, process := range h.Instruments {
		h.logger.Info(
			HandlerName,
			fmt.Sprintf("Stopping instrument %s during cleanup", name),
		)
		h.stopInstrument(process)
	}
	h.Instruments = make(map[string]*InstrumentProcess)
	if h.cleanupStop != nil {
		close(h.cleanupStop)
	}

	return nil
}

// getSubscriptionConfigs returns the configured subscriptions
func (h *Handler) getSubscriptionConfigs() []subscriptionConfig {
	return []subscriptionConfig{
		{
			subject: SetupInstrumentSubject,
			handler: h.handleSetupInstrument,
			name:    SetupInstrumentCommand,
		},
		{
			subject: DestroyInstrumentSubject,
			handler: h.handleDestroyInstrument,
			name:    DestroyInstrumentCommand,
		},
		{
			subject: ConfirmInitializationSubject,
			handler: h.handleConfirmInitialization,
			name:    ConfirmInitializationCommand,
		},
		{
			subject: UpdateDaemonPropertySubject,
			handler: h.handleUpdateDaemonProperty,
			name:    UpdateDaemonPropertyCommand,
		},
	}
}
