package instrument

import (
	"fmt"
	"time"

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

// Unsubscribe stops all subscriptions and destroys all instruments
func (h *Handler) Unsubscribe() error {
	h.Log.Info("Unsubscribing from instrument handler")

	// First, destroy all instruments using the destroy queue
	h.destroyAllInstruments()

	// Then stop accepting new destroy requests
	close(h.destroyQueue)

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

// destroyAllInstruments queues all instruments for destruction and waits for
// completion
func (h *Handler) destroyAllInstruments() {
	h.mutex.RLock()
	instrumentCount := len(h.Instruments)

	if instrumentCount == 0 {
		h.mutex.RUnlock()
		h.Log.Debug("No instruments to destroy")
		return
	}

	h.Log.Info("Queuing %d instruments for destruction", instrumentCount)

	// Get list of instrument names to destroy
	instrumentNames := make([]Name, 0, instrumentCount)
	for name, instrument := range h.Instruments {
		if !instrument.Completed {
			instrumentNames = append(instrumentNames, name)
		}
	}
	h.mutex.RUnlock()

	// Queue all instruments for destruction
	for _, name := range instrumentNames {
		select {
		case h.destroyQueue <- name:
			h.Log.Debug("Queued instrument %s for destruction", name)
		default:
			h.Log.Error("Destroy queue full, forcing destruction of %s", name)
			// If queue is full, destroy directly (fallback)
			h.destroyInstrumentDirect(name)
		}
	}

	// Wait for all instruments to be destroyed
	h.waitForAllInstrumentsDestroyed()
}

// waitForAllInstrumentsDestroyed polls until all instruments are gone
func (h *Handler) waitForAllInstrumentsDestroyed() {
	maxWait := 30 * time.Second
	checkInterval := 100 * time.Millisecond
	timeout := time.After(maxWait)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			h.mutex.RLock()
			remaining := len(h.Instruments)
			h.mutex.RUnlock()
			h.Log.Error(
				"Timeout waiting for instrument destruction, %d instruments remain",
				remaining,
			)
			return

		case <-ticker.C:
			h.mutex.RLock()
			remaining := len(h.Instruments)
			h.mutex.RUnlock()

			if remaining == 0 {
				h.Log.Info("All instruments successfully destroyed")
				return
			}
			h.Log.Debug("Waiting for %d instruments to be destroyed", remaining)
		}
	}
}

// destroyInstrumentDirect destroys an instrument directly (fallback when queue
// is full)
func (h *Handler) destroyInstrumentDirect(name Name) {
	h.mutex.Lock()
	process, exists := h.Instruments[name]
	if exists && !process.Completed {
		delete(h.Instruments, name)
		h.mutex.Unlock()
		h.Log.Info("Directly destroying instrument: %s", name)
		h.stopInstrument(process)
	} else {
		h.mutex.Unlock()
	}
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
