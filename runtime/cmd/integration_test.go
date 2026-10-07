package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	measure "github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/measure"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type DependencySpy struct {
	natsURL string

	measurementBaseDir string
	measurementDBPath  string

	loggerOutputPath string

	handlerStarted bool
	statusStarted  bool

	callOrder []string
}

type fakeConfigProvider struct{}

func (fakeConfigProvider) LoadConfig(
	path string,
) (*config.HubConfig, error) {
	return nil, nil
}

func (fakeConfigProvider) Validate(
	cfg *config.HubConfig,
) error {
	return nil
}

func (fakeConfigProvider) LoadDeviceConfig(
	path string,
) (string, error) {
	return "{}", nil
}

func (fakeConfigProvider) NewConnectedPorts(
	[]config.InstrumentConfig,
	config.WireMap,
) (*config.ConnectedPorts, error) {
	return &config.ConnectedPorts{}, nil
}

func TestRuntime_PassesCorrectMeasurementPaths(t *testing.T) {
	spy := &DependencySpy{}

	cfg := &config.HubConfig{
		QuantumDotConfig: "tmp/config.yaml",
		LocalDatabase:    "/tmp/measurements",
		RuntimePaths: config.RuntimePaths{
			DataCache: "/tmp/cache",
		},
		InstrumentServer: config.InstrumentServerConfig{
			AutoStart: true,
		},
	}

	deps := RuntimeDependencies{
		newNATSManager: func(url string) (NATSManager, error) {
			return &FakeNATSManager{}, nil
		},

		newLogger: func(
			string,
		) (*logging.Logger, error) {
			return logging.NewLogger(t.TempDir())
		},

		configProvider: fakeConfigProvider{},

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
			spy.measurementBaseDir = measurementScriptsPath

			return &FakeHandlerManager{}
		},
	}

	_, _ = deps.NewRuntime(cfg)

	assert.Equal(
		t,
		cfg.UserMeasurementLuasDir,
		spy.measurementBaseDir,
	)
}

func TestRuntime_PassesCorrectNATSURL(t *testing.T) {
	spy := &DependencySpy{}

	cfg := &config.HubConfig{
		NATSURL: "nats://localhost:4222",

		InstrumentServer: config.InstrumentServerConfig{
			AutoStart: true,
		},
	}

	deps := RuntimeDependencies{
		newNATSManager: func(
			url string,
		) (NATSManager, error) {
			spy.natsURL = url

			return nil, fmt.Errorf("stop here")
		},
	}

	_, _ = deps.NewRuntime(cfg)

	assert.Equal(
		t,
		"nats://localhost:4222",
		spy.natsURL,
	)
}

func TestRuntime_PassesCorrectLoggerPath(t *testing.T) {
	spy := &DependencySpy{}

	cfg := &config.HubConfig{
		RuntimePaths: config.RuntimePaths{
			Logs: "/tmp/log",
		},

		InstrumentServer: config.InstrumentServerConfig{
			AutoStart: true,
		},
	}

	deps := RuntimeDependencies{
		newNATSManager: func(
			string,
		) (NATSManager, error) {
			return &FakeNATSManager{}, nil
		},

		newLogger: func(
			outputPath string,
		) (*logging.Logger, error) {
			spy.loggerOutputPath = outputPath

			return nil, fmt.Errorf("stop here")
		},
	}

	_, _ = deps.NewRuntime(cfg)

	assert.Equal(
		t,
		"/tmp/log",
		spy.loggerOutputPath,
	)
}

type validationFailureProvider struct{}

func (validationFailureProvider) LoadConfig(
	path string,
) (*config.HubConfig, error) {
	return &config.HubConfig{}, nil
}

func (validationFailureProvider) Validate(
	*config.HubConfig,
) error {
	return fmt.Errorf(
		"at least one instrument is required",
	)
}

func (validationFailureProvider) LoadDeviceConfig(
	string,
) (string, error) {
	return "{}", nil
}

func (validationFailureProvider) NewConnectedPorts(
	[]config.InstrumentConfig,
	config.WireMap,
) (*config.ConnectedPorts, error) {
	return &config.ConnectedPorts{}, nil
}

func TestNewRunHub_ValidationFailure(t *testing.T) {
	cfgFile := filepath.Join(
		t.TempDir(),
		"config.yaml",
	)

	err := os.WriteFile(
		cfgFile,
		[]byte(`
instrument-server:
  instruments: []
`),
		0644,
	)
	require.NoError(t, err)

	cli := &CLIOptions{
		Config: cfgFile,
	}

	run := NewRunHub(
		RuntimeDependencies{
			configProvider: validationFailureProvider{},
		},
		cli,
	)

	err = run(nil, nil)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"at least one instrument",
	)
}

type passingConfigProvider struct {
	cfg *config.HubConfig
}

func (p passingConfigProvider) LoadConfig(
	path string,
) (*config.HubConfig, error) {
	return p.cfg, nil
}

func (passingConfigProvider) Validate(
	*config.HubConfig,
) error {
	return nil
}

func (passingConfigProvider) LoadDeviceConfig(
	string,
) (string, error) {
	return "{}", nil
}

func (passingConfigProvider) NewConnectedPorts(
	[]config.InstrumentConfig,
	config.WireMap,
) (*config.ConnectedPorts, error) {
	return &config.ConnectedPorts{}, nil
}

func TestNewRunHub_CheckEnvironmentFailure(
	t *testing.T,
) {
	t.Setenv("PATH", "")

	cli := &CLIOptions{
		Config: "ignored.yaml",
	}

	run := NewRunHub(
		RuntimeDependencies{
			configProvider: passingConfigProvider{
				cfg: &config.HubConfig{
					WorkingDirectory: t.TempDir(),
					InstrumentServer: config.InstrumentServerConfig{
						AutoStart: true,
					},
				},
			},
		},
		cli,
	)

	err := run(nil, nil)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"instrument-script-server binary not found",
	)
}

func TestRunServer(t *testing.T) {
	r := Runtime{
		cfg: &config.HubConfig{},
		natsManager: &FakeNATSManager{
			conn: &nats.Conn{},
		},
	}

	go func() {
		time.Sleep(100 * time.Millisecond)

		_ = syscall.Kill(
			os.Getpid(),
			syscall.SIGTERM,
		)
	}()

	err := r.runServer()

	require.NoError(t, err)
}

type RuntimeCallTracker struct {
	callOrder []string

	handler *FakeHandlerManager
}

func TestNewRuntime_HappyPath(t *testing.T) {
	tracker := []string{}

	handler := &FakeHandlerManager{}

	cfg := &config.HubConfig{
		NATSURL: "nats://localhost:4222",

		LocalDatabase: t.TempDir(),

		RuntimePaths: config.RuntimePaths{
			Logs:      t.TempDir(),
			DataCache: t.TempDir(),
		},

		InstrumentServer: config.InstrumentServerConfig{
			AutoStart: true,
		},
	}

	deps := RuntimeDependencies{
		newNATSManager: func(
			url string,
		) (NATSManager, error) {
			tracker = append(tracker, "nats")
			return &FakeNATSManager{}, nil
		},

		newLogger: func(
			path string,
		) (*logging.Logger, error) {
			tracker = append(tracker, "logger")

			return logging.NewLogger(
				t.TempDir(),
			)
		},

		configProvider: fakeConfigProvider{},

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
			tracker = append(tracker, "handlers")

			return handler
		},
	}

	runtime, err := deps.NewRuntime(cfg)

	require.NoError(t, err)
	require.NotNil(t, runtime)

	assert.True(t, handler.coreStarted)
	assert.True(t, handler.statusStarted)

	assert.Equal(
		t,
		[]string{
			"nats",
			"logger",
			"handlers",
		},
		tracker,
	)

	runtime.Close()
}
