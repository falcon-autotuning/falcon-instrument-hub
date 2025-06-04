package handlers

import (
	"fmt"
	"log"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

// Manager manages all NATS message handlers
type Manager struct {
	logger *logging.Logger
	nc     *nats.Conn
	mu     sync.RWMutex

	// Handlers
	logHandler *LogHandler
	// Add more handlers here as needed
}

// NewManager creates a new handler manager
func NewManager(logger *logging.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// Subscribe subscribes all handlers to their respective NATS subjects
func (m *Manager) Subscribe(nc *nats.Conn) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nc = nc

	// Subscribe LOG handler
	m.logHandler = NewLogHandler(m.logger)
	if err := m.logHandler.Subscribe(nc); err != nil {
		return fmt.Errorf("failed to subscribe LOG handler: %w", err)
	}

	// Add more handler subscriptions here

	m.logger.Info("MANAGER", "All handlers subscribed successfully")
	log.Printf("Handler manager: All handlers subscribed successfully")

	return nil
}

// Unsubscribe unsubscribes all handlers from their NATS subjects
func (m *Manager) Unsubscribe() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []error

	// Unsubscribe LOG handler
	if m.logHandler != nil {
		if err := m.logHandler.Unsubscribe(); err != nil {
			errors = append(errors, fmt.Errorf("LOG handler unsubscribe: %w", err))
		}
		m.logHandler = nil
	}

	// Add more handler unsubscriptions here

	if len(errors) > 0 {
		return fmt.Errorf("handler unsubscribe errors: %v", errors)
	}

	return nil
}

// GetLogHandler returns the log handler (for testing)
func (m *Manager) GetLogHandler() *LogHandler {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.logHandler
}
