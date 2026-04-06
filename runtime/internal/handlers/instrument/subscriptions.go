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

	// Configure all subscriptions
	subscriptions := h.getSubscriptionConfigs()

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

// Unsubscribe stops all subscriptions and clears instrument registrations
func (h *Handler) Unsubscribe() error {
	h.Log.Info("Unsubscribing from instrument handler")

	// Clear all instrument registrations
	h.mutex.Lock()
	for name := range h.Instruments {
		delete(h.Instruments, name)
	}
	h.mutex.Unlock()

	// Unsubscribe from NATS
	var unsubscribeErrors []error
	for i, sub := range h.subscriptions {
		if sub != nil {
			if err := sub.Unsubscribe(); err != nil {
				h.Log.Error(
					"Failed to unsubscribe from subscription %d: %v",
					i,
					err,
				)
				unsubscribeErrors = append(unsubscribeErrors, err)
			}
		}
	}

	h.subscriptions = nil

	if len(unsubscribeErrors) > 0 {
		return fmt.Errorf(
			"failed to unsubscribe from %d subscriptions",
			len(unsubscribeErrors),
		)
	}

	h.Log.Info(
		"Successfully unsubscribed from all instrument handler subscriptions",
	)
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
