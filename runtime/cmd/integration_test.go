package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
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

func TestRuntime_PassesCorrectMeasurementPaths(t *testing.T) {
	spy := &DependencySpy{}

	cfg := &HubConfig{
		LocalDatabase: "/tmp/measurements",
		RuntimePaths: RuntimePaths{
			DataCache: "/tmp/cache",
		},
		InstrumentServer: InstrumentServerConfig{
			AutoStart: true,
		},
	}

	deps := RuntimeDependencies{
		newNATSManager: func(url string) (NATSManager, error) {
			return &FakeNATSManager{}, nil
		},

		newMeasurementManager: func(
			baseDir string,
			dbPath string,
		) (MeasurementManager, error) {
			spy.measurementBaseDir = baseDir
			spy.measurementDBPath = dbPath

			return &FakeMeasurementManager{}, nil
		},

		newLogger: func(
			string,
		) (*logging.Logger, error) {
			return nil, fmt.Errorf("stop here")
		},
	}

	_, _ = deps.NewRuntime(cfg)

	assert.Equal(
		t,
		cfg.LocalDatabase,
		spy.measurementBaseDir,
	)

	assert.Equal(
		t,
		filepath.Join(
			cfg.RuntimePaths.DataCache,
			MeasurementsDB,
		),
		spy.measurementDBPath,
	)
}

func TestRuntime_PassesCorrectNATSURL(t *testing.T) {
	spy := &DependencySpy{}

	cfg := &HubConfig{
		NATSURL: "nats://localhost:4222",

		InstrumentServer: InstrumentServerConfig{
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

	cfg := &HubConfig{
		RuntimePaths: RuntimePaths{
			Logs: "/tmp/log",
		},

		InstrumentServer: InstrumentServerConfig{
			AutoStart: true,
		},
	}

	deps := RuntimeDependencies{
		newNATSManager: func(
			string,
		) (NATSManager, error) {
			return &FakeNATSManager{}, nil
		},

		newMeasurementManager: func(
			string,
			string,
		) (MeasurementManager, error) {
			return &FakeMeasurementManager{}, nil
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

func TestCLIToConfig(t *testing.T) {
	cfg := DefaultConfig()

	cmd, cli := buildRootCmd(
		RuntimeDependencies{},
	)

	err := cmd.Flags().Parse([]string{
		"--config", "hub.yaml",
		"--nats-url", "nats://localhost:4222",
		"--device-config", "device.yaml",
		"--wiremap", "wiremap.yaml",
		"--working-dir", "/tmp/workdir",
		"--local-database", "/tmp/database",
		"--user-measurement-luas", "/tmp/scripts",
		"--measurement-metadata", "metadata.yaml",
		"--no-iss",
		"--instrument", "awg.yaml:awg.so",
		"--instrument", "dac.yaml:dac.so",
	})
	require.NoError(t, err)

	err = cli.Update(&cfg)
	require.NoError(t, err)

	assert.Equal(
		t,
		"hub.yaml",
		cli.Config,
	)

	assert.Equal(
		t,
		"nats://localhost:4222",
		cfg.NATSURL,
	)

	assert.Equal(
		t,
		"device.yaml",
		cfg.QuantumDotConfig,
	)

	assert.Equal(
		t,
		"wiremap.yaml",
		cfg.Wiremap,
	)

	assert.Equal(
		t,
		"/tmp/workdir",
		cfg.WorkingDirectory,
	)

	assert.Equal(
		t,
		"/tmp/database",
		cfg.LocalDatabase,
	)

	assert.Equal(
		t,
		"/tmp/scripts",
		cfg.UserMeasurementLuasDir,
	)

	// --no-iss should disable autostart
	assert.False(
		t,
		cfg.InstrumentServer.AutoStart,
	)

	require.Len(
		t,
		cfg.InstrumentServer.Instruments,
		2,
	)

	assert.Equal(
		t,
		"awg.yaml",
		cfg.InstrumentServer.Instruments[0].ConfigPath,
	)

	assert.Equal(
		t,
		"awg.so",
		cfg.InstrumentServer.Instruments[0].PluginPath,
	)

	assert.Equal(
		t,
		"dac.yaml",
		cfg.InstrumentServer.Instruments[1].ConfigPath,
	)

	assert.Equal(
		t,
		"dac.so",
		cfg.InstrumentServer.Instruments[1].PluginPath,
	)
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
		RuntimeDependencies{},
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

func TestNewRunHub_CheckEnvironmentFailure(t *testing.T) {
	tmp := t.TempDir()

	cfgFile := filepath.Join(tmp, "config.yaml")

	err := os.WriteFile(
		cfgFile,
		[]byte(fmt.Sprintf(`
working-directory: %s

instrument-server:
  instruments:
    - config: a.yaml
      plugin: a.so
`, tmp)),
		0644,
	)

	require.NoError(t, err)

	t.Setenv("PATH", "")

	cli := &CLIOptions{
		Config: cfgFile,
	}

	run := NewRunHub(
		RuntimeDependencies{},
		cli,
	)

	err = run(nil, nil)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"instrument-script-server binary not found",
	)
}

func TestRunServer(t *testing.T) {
	r := Runtime{
		cfg: &HubConfig{},
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

type FakeDispatcher struct{}

func (f *FakeDispatcher) RunMeasurement(
	string,
	map[string]interface{},
	map[string]interface{},
) ([]handlers.ResolvedCallResult, error) {
	return nil, nil
}

func TestNewRuntime_HappyPath(t *testing.T) {
	tracker := []string{}

	handler := &FakeHandlerManager{}

	cfg := &HubConfig{
		NATSURL: "nats://localhost:4222",

		LocalDatabase: t.TempDir(),

		RuntimePaths: RuntimePaths{
			Logs:      t.TempDir(),
			DataCache: t.TempDir(),
		},

		InstrumentServer: InstrumentServerConfig{
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

		newMeasurementManager: func(
			string,
			string,
		) (MeasurementManager, error) {
			tracker = append(tracker, "measurements")
			return &FakeMeasurementManager{}, nil
		},

		newLogger: func(
			path string,
		) (*logging.Logger, error) {
			tracker = append(tracker, "logger")

			return logging.NewLogger(
				t.TempDir(),
			)
		},

		newConfig: func(
			string,
			string,
		) (*config.Config, error) {
			tracker = append(tracker, "config")

			return &config.Config{}, nil
		},

		newDispatcher: func(
			client handlers.MeasurementClient,
			scriptsPath string,
		) handlers.Dispatcher {
			tracker = append(tracker, "dispatcher")

			return &FakeDispatcher{}
		},

		newHandlerManager: func(
			cfg *config.Config,
			logger *logging.Logger,
			nc *nats.Conn,
			dispatcher handlers.Dispatcher,
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
			"measurements",
			"logger",
			"dispatcher",
			"config",
			"handlers",
		},
		tracker,
	)

	runtime.Close()
}
