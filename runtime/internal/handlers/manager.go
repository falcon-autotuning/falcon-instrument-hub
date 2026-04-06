package handlers

import (
	"fmt"
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/measure"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
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
	mu                             sync.RWMutex
	logHandler                     *LogHandler
	deviceConfigHandler            *DeviceConfigHandler
	instrumentHandler              *instrument.Handler
	busyHandler                    *BusyHandler
	measureCommandHandler          *MeasureCommandHandler
	measureReadyHandler            *measure.MeasurementReadyHandler
	performInstrumentMethodHandler *PerformInstrumentMethodHandler
	statusHandler                  *StatusHandler
	portRequestHandler             *PortRequestHandler
	natsURL                        string
	isBusy                         bool
}

// NewManager creates a new handler manager
func NewManager(
	cfg *config.Config,
	logger *logging.Logger,
	nc *nats.Conn,
	natsURL string,
	measurementManager *measurements.Manager,
) *Manager {
	instrumentHandler, err := instrument.NewHandler(
		logger,
		natsURL,
		nc,
		cfg,
	)
	if err != nil {
		logger.Error(
			HandlerManagerName,
			fmt.Sprintf("Failed to create instrument handler: %v", err),
		)
		// For now, return a basic handler - you might want to return an error
		// instead
		instrumentHandler = &instrument.Handler{
			Instruments: make(
				map[instrument.Name]*instrument.InstrumentProcess,
			),
		}
	}

	manager := &Manager{
		config:              cfg,
		logger:              logger,
		nc:                  nc,
		natsURL:             natsURL,
		logHandler:          NewLogHandler(logger),
		deviceConfigHandler: NewDeviceConfigHandler(cfg, logger),
		instrumentHandler:   instrumentHandler,
		performInstrumentMethodHandler: NewPerformInstrumentMethodHandler(
			logger,
			instrumentHandler,
		),
		portRequestHandler: NewPortRequestHandler(
			logger,
			instrumentHandler,
			cfg,
		),
		measureReadyHandler: measure.NewMeasurementReadyHandler(
			logger,
			instrumentHandler,
			cfg,
		),
		statusHandler: NewStatusHandler(logger),
		isBusy:        false,
	}
	// Create busy handler with reference to manager's busy state
	manager.busyHandler = NewBusyHandler(logger, &manager.isBusy)
	// Create measure command handler with manager as busy manager
	manager.measureCommandHandler = NewMeasureCommandHandler(
		logger,
		measurementManager,
		instrumentHandler,
		manager,
	)

	return manager
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
			name:    "busy handler",
			startOp: func() error { return m.busyHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.busyHandler.Unsubscribe() },
		},
		{
			name:    "measure command handler",
			startOp: func() error { return m.measureCommandHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.measureCommandHandler.Unsubscribe() },
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
			name:    "measurement ready handler",
			startOp: func() error { return m.measureReadyHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.measureReadyHandler.Unsubscribe() },
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
	return m.isBusy
}

// SetIsBusy sets the busy state
func (m *Manager) SetIsBusy(busy bool) {
	m.isBusy = busy
}
