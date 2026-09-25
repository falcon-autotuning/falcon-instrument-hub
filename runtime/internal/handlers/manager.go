package handlers

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	deviceconfighandlers "github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/device_config"
	measure "github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/measure"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
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
	logger                *logging.Logger
	nc                    *nats.Conn
	deviceConfigHandler   *deviceconfighandlers.Handler
	measureCommandHandler *measure.Handler
	statusHandler         *StatusHandler
	portRequestHandler    *PortRequestHandler
	isBusy                bool
	instrumentError       error
	metadataError         error
}

// NewManager creates a new handler manager
func NewManager(
	deviceConfigJSON string,
	wiremap *config.WireMap,
	instrumentAPIPaths []string,
	measurementScriptsPath string,
	logger *logging.Logger,
	nc *nats.Conn,
	dispatcher measure.MeasurementClient,
) *Manager {
	ports, err := ports.NewConnectedPorts(instrumentAPIPaths, wiremap)
	if err != nil {
		logger.Error(
			HandlerManagerName,
			fmt.Sprintf("Failed to load connected ports: %v", err),
		)
	}

	manager := &Manager{
		logger:              logger,
		nc:                  nc,
		deviceConfigHandler: deviceconfighandlers.NewDeviceConfigHandler(deviceConfigJSON, logger),
		portRequestHandler: NewPortRequestHandler(
			logger,
			ports,
		),
		statusHandler: NewStatusHandler(logger),
		metadataError: err,
	}
	manager.measureCommandHandler = measure.NewMeasureCommandHandler(
		logger,
		manager,
		measurementScriptsPath,
		dispatcher,
		wiremap,
		ports,
	)

	return manager
}

// Start initializes all handlers and their subscriptions
func (m *Manager) Start() error {
	if m.instrumentError != nil {
		return fmt.Errorf("invalid instrument configuration: %w", m.instrumentError)
	}
	if m.metadataError != nil {
		return fmt.Errorf("invalid measurement metadata: %w", m.metadataError)
	}
	m.logger.Info(HandlerManagerName, "Starting handler manager")

	// Execute each startup operation
	for _, op := range m.getHandlerOperations(true) {
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

// StartCoreHandlers starts all handlers except status publishing. The hub uses
// this during startup so STATUS.instrument-server is only emitted after ISS
// instruments are also ready.
func (m *Manager) StartCoreHandlers() error {
	if m.instrumentError != nil {
		return fmt.Errorf("invalid instrument configuration: %w", m.instrumentError)
	}
	if m.metadataError != nil {
		return fmt.Errorf("invalid measurement metadata: %w", m.metadataError)
	}
	m.logger.Info(HandlerManagerName, "Starting core handler manager")

	for _, op := range m.getHandlerOperations(false) {
		if err := op.startOp(); err != nil {
			m.logger.Error(
				HandlerManagerName,
				fmt.Sprintf("Failed to start %s", op.name),
			)
			return err
		}
	}

	m.logger.Info(HandlerManagerName, "Core handlers started successfully")
	return nil
}

// StartStatus begins STATUS.instrument-server publication.
func (m *Manager) StartStatus() error {
	if err := m.statusHandler.Start(m.nc); err != nil {
		m.logger.Error(HandlerManagerName, "Failed to start status handler")
		return err
	}
	m.logger.Info(HandlerManagerName, "All handlers started successfully")
	return nil
}

// Stop gracefully shuts down all handlers
func (m *Manager) Stop() error {
	m.logger.Info(HandlerManagerName, "Stopping handler manager")

	// Execute each shutdown operation in reverse order (continue on errors)
	ops := m.getHandlerOperations(true)
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

// GetDeviceConfigHandler returns the device config handler for testing purposes
func (m *Manager) GetDeviceConfigHandler() *deviceconfighandlers.Handler {
	return m.deviceConfigHandler
}

// getHandlerOperations returns the ordered list of handler operations
func (m *Manager) getHandlerOperations(includeStatus bool) []handlerOperation {
	ops := []handlerOperation{
		{
			name:    "device config handler",
			startOp: func() error { return m.deviceConfigHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.deviceConfigHandler.Unsubscribe() },
		},
		{
			name:    "measure command handler",
			startOp: func() error { return m.measureCommandHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.measureCommandHandler.Unsubscribe() },
		},
		{
			name:    "port request handler",
			startOp: func() error { return m.portRequestHandler.Subscribe(m.nc) },
			stopOp:  func() error { return m.portRequestHandler.Unsubscribe() },
		},
	}
	if includeStatus {
		ops = append(ops, handlerOperation{
			name:    "status handler",
			startOp: func() error { return m.statusHandler.Start(m.nc) },
			stopOp:  func() error { return m.statusHandler.Stop() },
		})
	}
	return ops
}

// IsBusy checks if the system is currently busy with any operations
func (m *Manager) IsBusy() bool {
	return m.isBusy
}

// SetIsBusy sets the busy state
func (m *Manager) SetIsBusy(busy bool) {
	m.isBusy = busy
}
