package main

import (
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers"
	measure "github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/measure"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/networking"
	"github.com/nats-io/nats.go"
)

var defaultHost = instrumentserver.DefaultISSHost

type ISSClient interface {
	StartInstrument(string, string) error
	StopInstrument(string) error
	ListInstruments() ([]string, error)
	DaemonStatus() (bool, error)
	StopDaemon() error
	Close() error
	Measure(
		scriptPath string,
		variables []instrumentserver.MeasureVariable,
	) ([]instrumentserver.CallResult, error)
	ReleaseBuffer(
		bufferID string,
	) error
}

var _ ISSClient = (*instrumentserver.ScriptServerClient)(nil)

type HandlerManager interface {
	StartCoreHandlers() error
	StartStatus() error
	Stop() error
}

var _ HandlerManager = (*handlers.Manager)(nil)

type NATSManager interface {
	GetConnection() *nats.Conn
	Close()
}

var _ NATSManager = (*networking.NATSManager)(nil)

type RuntimeDependencies struct {
	newISSClient func(
		host string,
		port int,
		binary string,
	) ISSClient

	newHandlerManager func(
		deviceConfigJSON string,
		wiremap *config.WireMap,
		instrumentAPIPaths []string,
		measurementScriptsPath string,
		logger *logging.Logger,
		nc *nats.Conn,
		dispatcher measure.MeasurementClient,
	) HandlerManager

	newNATSManager func(
		url string,
	) (NATSManager, error)

	newLogger func(
		outputPath string,
	) (*logging.Logger, error)

	newWiremap func(
		wiremapPath string,
		deviceConfigPath string,
	) (*config.WireMap, error)

	newConfig func(
		deviceConfigPath string,
	) (string, error)
}

var ProductionDependancies = RuntimeDependencies{
	newISSClient: func(
		host string,
		port int,
		issBinary string,
	) ISSClient {
		return instrumentserver.NewScriptServerClientWithOptions(
			host,
			port,
			instrumentserver.ScriptServerClientOptions{
				ISSBinary: issBinary,
			},
		)
	},
	newHandlerManager: func(
		deviceConfigJSON string,
		wiremap *config.WireMap,
		instrumentAPIPaths []string,
		measurementScriptsPath string,
		logger *logging.Logger,
		nc *nats.Conn,
		dispatcher measure.MeasurementClient,
	) HandlerManager {
		return handlers.NewManager(
			deviceConfigJSON,
			wiremap,
			instrumentAPIPaths,
			measurementScriptsPath,
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
	newLogger: func(
		outputPath string,
	) (*logging.Logger, error) {
		return logging.NewLoggerWithOptions(outputPath, logging.LoggerOptions{
			Diagnostics: false,
		})
	},
	newConfig: func(
		deviceConfigPath string,
	) (string, error) {
		return config.LoadConfig(
			deviceConfigPath,
		)
	},
	newWiremap: func(
		wiremapPath string,
		deviceConfigPath string,
	) (*config.WireMap, error) {
		return config.LoadWiremap(
			wiremapPath,
			deviceConfigPath,
		)
	},
}
