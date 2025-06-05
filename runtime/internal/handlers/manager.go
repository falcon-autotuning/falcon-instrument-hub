//go:generate ./copy_script.sh
package handlers

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

const (
	HandlerManagerName = "HANDLER_MANAGER"
)

// handlerOperation represents a handler operation for startup or shutdown
type handlerOperation struct {
	name    string
	startOp func() error
	stopOp  func() error
}

// Manager manages all message handlers
type Manager struct {
	config                         *config.Config
	logger                         *logging.Logger
	nc                             *nats.Conn
	logHandler                     *LogHandler
	deviceConfigHandler            *DeviceConfigHandler
	instrumentHandler              *instrument.Handler
	interpreterHandler             *InterpreterHandler
	busyHandler                    *BusyHandler
	performInstrumentMethodHandler *PerformInstrumentMethodHandler
	statusHandler                  *StatusHandler
	portRequestHandler             *PortRequestHandler
	natsURL                        string
}

// NewManager creates a new handler manager
func NewManager(
	cfg *config.Config,
	logger *logging.Logger,
	nc *nats.Conn,
	natsURL string,
) *Manager {
	instrumentHandler := instrument.NewHandler(logger, natsURL, nc)
	return &Manager{
		config:              cfg,
		logger:              logger,
		nc:                  nc,
		natsURL:             natsURL,
		logHandler:          NewLogHandler(logger),
		deviceConfigHandler: NewDeviceConfigHandler(cfg, logger),
		instrumentHandler:   instrumentHandler,
		interpreterHandler:  NewInterpreterHandler(logger, natsURL),
		busyHandler:         NewBusyHandler(logger),
		performInstrumentMethodHandler: NewPerformInstrumentMethodHandler(
			logger,
			instrumentHandler,
		),
		portRequestHandler: NewPortRequestHandler(logger, instrumentHandler),
		statusHandler:      NewStatusHandler(logger),
	}
}

// Start initializes all handlers and their subscriptions
func (m *Manager) Start() error {
	m.logger.Info(HandlerManagerName, "Starting handler manager")

	// Execute each startup operation
	for _, op := range m.getHandlerOperations() {
		if err := op.startOp(); err != nil {
			m.logger.Error(
				HandlerManagerName,
				fmt.Sprintf("Failed to start %s", op.name),
			)
			return err
		}
	}

	m.logger.Info(HandlerManagerName, "All handlers started successfully")
	return nil
}

// Stop gracefully shuts down all handlers
func (m *Manager) Stop() error {
	m.logger.Info(HandlerManagerName, "Stopping handler manager")

	// Execute each shutdown operation in reverse order (continue on errors)
	ops := m.getHandlerOperations()
	for i := len(ops) - 1; i >= 0; i-- {
		if err := ops[i].stopOp(); err != nil {
			m.logger.Error(
				HandlerManagerName,
				fmt.Sprintf("Failed to stop %s", ops[i].name),
			)
		}
	}

	m.logger.Info(HandlerManagerName, "Handler manager stopped")
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

// GetInstrumentHandler returns the instrument handler for testing purposes
func (m *Manager) GetInstrumentHandler() *instrument.Handler {
	return m.instrumentHandler
}

// getHandlerOperations returns the ordered list of handler operations
func (m *Manager) getHandlerOperations() []handlerOperation {
	return []handlerOperation{
		{
			name:    "log handler",
			startOp: func() error { return m.logHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.logHandler.Unsubscribe() },
		},
		{
			name:    "device config handler",
			startOp: func() error { return m.deviceConfigHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.deviceConfigHandler.Unsubscribe() },
		},
		{
			name:    "instrument handler",
			startOp: func() error { return m.instrumentHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.instrumentHandler.Unsubscribe() },
		},
		{
			name:    "interpreter handler",
			startOp: func() error { return m.interpreterHandler.Start() },
			stopOp:  func() error { return m.interpreterHandler.Stop() },
		},
		{
			name:    "busy handler",
			startOp: func() error { return m.busyHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.busyHandler.Unsubscribe() },
		},
		{
			name:    "perform instrument method handler",
			startOp: func() error { return m.performInstrumentMethodHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.performInstrumentMethodHandler.Unsubscribe() },
		},
		{
			name:    "port request handler",
			startOp: func() error { return m.portRequestHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.portRequestHandler.Unsubscribe() },
		},
		{
			name:    "status handler",
			startOp: func() error { return m.statusHandler.Start(m.nc) },
			stopOp:  func() error { return m.statusHandler.Stop() },
		},
	}
}

// IsBusy checks if the system is currently busy with any operations
func (m *Manager) IsBusy() bool {
	// TODO: upgrade this to flagging when measurement is taking place

	return false
}
