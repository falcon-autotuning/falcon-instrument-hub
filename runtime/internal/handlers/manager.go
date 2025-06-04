package handlers

import (
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

// Manager manages all message handlers
type Manager struct {
	config              *config.Config
	logger              *logging.Logger
	nc                  *nats.Conn
	logHandler          *LogHandler
	deviceConfigHandler *DeviceConfigHandler
	instrumentHandler   *InstrumentHandler
	natsURL             string
}

// NewManager creates a new handler manager
func NewManager(cfg *config.Config, logger *logging.Logger, nc *nats.Conn, natsURL string) *Manager {
	return &Manager{
		config:              cfg,
		logger:              logger,
		nc:                  nc,
		natsURL:             natsURL,
		logHandler:          NewLogHandler(logger),
		deviceConfigHandler: NewDeviceConfigHandler(cfg, logger),
		instrumentHandler:   NewInstrumentHandler(logger, natsURL),
	}
}

// Start initializes all handlers and their subscriptions
func (m *Manager) Start() error {
	m.logger.Info("HANDLER_MANAGER", "Starting handler manager")

	// Subscribe to log messages
	if err := m.logHandler.Subscribe(m.nc); err != nil {
		m.logger.Error("HANDLER_MANAGER", "Failed to start log handler")
		return err
	}

	// Subscribe to device config requests
	if err := m.deviceConfigHandler.Subscribe(m.nc); err != nil {
		m.logger.Error("HANDLER_MANAGER", "Failed to start device config handler")
		return err
	}

	// Subscribe to instrument commands
	if err := m.instrumentHandler.Subscribe(m.nc); err != nil {
		m.logger.Error("HANDLER_MANAGER", "Failed to start instrument handler")
		return err
	}

	m.logger.Info("HANDLER_MANAGER", "All handlers started successfully")
	return nil
}

// Stop gracefully shuts down all handlers
func (m *Manager) Stop() error {
	m.logger.Info("HANDLER_MANAGER", "Stopping handler manager")

	// Unsubscribe from device config requests
	if err := m.deviceConfigHandler.Unsubscribe(); err != nil {
		m.logger.Error("HANDLER_MANAGER", "Failed to stop device config handler")
	}

	// Unsubscribe from instrument commands
	if err := m.instrumentHandler.Unsubscribe(); err != nil {
		m.logger.Error("HANDLER_MANAGER", "Failed to stop instrument handler")
	}

	// Unsubscribe from log messages
	if err := m.logHandler.Unsubscribe(); err != nil {
		m.logger.Error("HANDLER_MANAGER", "Failed to stop log handler")
	}

	m.logger.Info("HANDLER_MANAGER", "Handler manager stopped")
	return nil
}

// GetLogHandler returns the log handler for testing purposes
func (m *Manager) GetLogHandler() *LogHandler {
	return m.logHandler
}

// GetDeviceConfigHandler returns the device config handler for testing purposes
func (m *Manager) GetDeviceConfigHandler() *DeviceConfigHandler {
	return m.deviceConfigHandler
}
