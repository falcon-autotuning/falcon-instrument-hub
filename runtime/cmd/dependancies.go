package main

import (
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/networking"
	"github.com/nats-io/nats.go"
)

var defaultHost = instrumentserver.DefaultISSHost

type InstrumentManager interface {
	StartInstrument(string, string) error
	StopInstrument(string) error
	ListInstruments() ([]string, error)
	DaemonStatus() (bool, error)
	StopDaemon() error
	Close() error
}

// FIX: this implementation missing DaemonStatus
// var _ InstrumentManager = (*serverinterpreter.ScriptServerClient)(nil)
type HandlerManager interface {
	StartCoreHandlers() error
	StartStatus() error
	Stop() error
}

// TODO: insert var

type NATSManager interface {
	GetConnection() *nats.Conn
	Close()
}

var _ NATSManager = (*networking.NATSManager)(nil)

type MeasurementManager interface {
	Close() error
}

var _ MeasurementManager = (*measurements.Manager)(nil)

type ISSRuntimeClient interface {
	InstrumentManager
	handlers.MeasurementClient
}

type RuntimeDependencies struct {
	newISSClient func(
		host string,
		port int,
		binary string,
	) ISSRuntimeClient

	newDispatcher func(
		client handlers.MeasurementClient,
		scriptsPath string,
	) handlers.Dispatcher

	newHandlerManager func(
		cfg *config.Config,
		logger *logging.Logger,
		nc *nats.Conn,
		dispatcher handlers.Dispatcher,
	) HandlerManager

	newNATSManager func(
		url string,
	) (NATSManager, error)

	newMeasurementManager func(
		baseDataDir string,
		databasePath string,
	) (MeasurementManager, error)

	newLogger func(
		outputPath string,
	) (*logging.Logger, error)

	newConfig func(
		deviceConfig string,
		wiremap string,
	) (*config.Config, error)
}

// FIX: this implementation missing DaemonStatus
// var _ ISSRuntimeClient = (*serverinterpreter.ScriptServerClient)(nil)
var ProductionDependancies = RuntimeDependencies{
	// newISSClient: func(
	// 	host string,
	// 	port int,
	// 	issBinary string,
	// ) ISSRuntimeClient {
	// 	return serverinterpreter.NewScriptServerClientWithOptions(
	// 		host,
	// 		port,
	// 		serverinterpreter.ScriptServerClientOptions{
	// 			ISSBinary: issBinary,
	// 		},
	// 	)
	// },
	newDispatcher: func(
		client handlers.MeasurementClient,
		scriptsPath string,
	) handlers.Dispatcher {
		return handlers.NewScriptDispatcher(
			client,
			scriptsPath,
		)
	},
	newHandlerManager: func(
		cfg *config.Config,
		logger *logging.Logger,
		nc *nats.Conn,
		dispatcher handlers.Dispatcher,
	) HandlerManager {
		return handlers.NewManager(
			cfg,
			logger,
			nc,
			dispatcher,
		)
	},
	newNATSManager: func(
		url string,
	) (NATSManager, error) {
		return networking.NewNATSManager(url)
	},
	newMeasurementManager: func(
		baseDataDir string,
		databasePath string,
	) (MeasurementManager, error) {
		return measurements.NewManager(baseDataDir, databasePath)
	},
	newLogger: func(
		outputPath string,
	) (*logging.Logger, error) {
		return logging.NewLoggerWithOptions(outputPath, logging.LoggerOptions{
			Diagnostics: false,
		})
	},
	newConfig: func(
		deviceConfig string,
		wiremap string,
	) (*config.Config, error) {
		return config.Load(
			deviceConfig,
			wiremap,
		)
	},
}
