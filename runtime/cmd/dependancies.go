package main

import (
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers"
	measure "github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/measure"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/networking"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
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

type ConfigProvider interface {
	LoadConfig(path string) (*config.HubConfig, error)
	Validate(*config.HubConfig) error
	LoadDeviceConfig(path string) (string, error)
	NewConnectedPorts([]config.InstrumentConfig, config.WireMap) (*config.ConnectedPorts, error)
}

type DefaultConfigProvider struct{}

func (DefaultConfigProvider) LoadConfig(
	path string,
) (*config.HubConfig, error) {
	return config.LoadConfig(path)
}

func (DefaultConfigProvider) Validate(
	cfg *config.HubConfig,
) error {
	return config.Validate(cfg)
}

func (DefaultConfigProvider) LoadDeviceConfig(
	path string,
) (string, error) {
	return config.LoadDeviceConfig(path)
}

func (DefaultConfigProvider) NewConnectedPorts(
	instruments []config.InstrumentConfig,
	wiremap config.WireMap,
) (*config.ConnectedPorts, error) {
	return config.NewConnectedPorts(instruments, wiremap)
}

type RuntimeDependencies struct {
	newISSClient func(
		host string,
		port int,
		binary string,
	) ISSClient

	newHandlerManager func(
		deviceConfigJSON string,
		wiremap config.WireMap,
		ports *config.ConnectedPorts,
		measurementScriptsPath string,
		logger *logging.Logger,
		nc *nats.Conn,
		dispatcher measure.MeasurementClient,
		instrumentMetadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	) HandlerManager

	newNATSManager func(
		url string,
	) (NATSManager, error)

	newLogger func(
		outputPath string,
	) (*logging.Logger, error)

	configProvider ConfigProvider
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
		wiremap config.WireMap,
		ports *config.ConnectedPorts,
		measurementScriptsPath string,
		logger *logging.Logger,
		nc *nats.Conn,
		dispatcher measure.MeasurementClient,
		instrumentMetadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	) HandlerManager {
		return handlers.NewManager(
			deviceConfigJSON,
			wiremap,
			ports,
			measurementScriptsPath,
			logger,
			nc,
			dispatcher,
			instrumentMetadata,
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
	configProvider: DefaultConfigProvider{},
}
