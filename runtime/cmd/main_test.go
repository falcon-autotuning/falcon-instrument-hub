package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/serverinterpreter"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(
		t,
		8555,
		cfg.InstrumentServer.RPCPort,
	)

	assert.True(
		t,
		cfg.InstrumentServer.AutoStart,
	)
}

func TestLoadConfig_FullConfig(t *testing.T) {
	tmp := t.TempDir()

	const (
		wiremapPath         = "wiremap.yaml"
		quantumDotConfig    = "quantum_dot.yaml"
		natsURL             = "nats://localhost:4222"
		localDatabase       = "/tmp/database"
		workingDirectory    = "/tmp/workdir"
		userMeasurementLuas = "/tmp/scripts"

		rpcPort = 9000

		instrument1Config = "instrument1.yaml"
		instrument1Plugin = "plugin1.so"

		instrument2Config = "instrument2.yaml"
		instrument2Plugin = "plugin2.so"
	)
	autostart := true

	cfgFile := filepath.Join(tmp, "hub.yaml")

	err := os.WriteFile(
		cfgFile,
		[]byte(fmt.Sprintf(
			`
wiremap: %s
quantum-dot-config: %s
nats-url: %s
local-database: %s
working-directory: %s
user-measurement-luas: %s

instrument-server:
  rpc-port: %d
  autostart: %t 

  instruments:
    - config: %s
      plugin: %s

    - config: %s
      plugin: %s
`,
			wiremapPath,
			quantumDotConfig,
			natsURL,
			localDatabase,
			workingDirectory,
			userMeasurementLuas,
			rpcPort,
			autostart,
			instrument1Config,
			instrument1Plugin,
			instrument2Config,
			instrument2Plugin,
		)),
		0644,
	)
	require.NoError(t, err)

	cfg, err := LoadConfig(cfgFile)
	require.NoError(t, err)

	assert.Equal(t, wiremapPath, cfg.Wiremap)
	assert.Equal(t, quantumDotConfig, cfg.QuantumDotConfig)
	assert.Equal(t, natsURL, cfg.NATSURL)
	assert.Equal(t, localDatabase, cfg.LocalDatabase)
	assert.Equal(t, workingDirectory, cfg.WorkingDirectory)
	assert.Equal(t, userMeasurementLuas, cfg.UserMeasurementLuasDir)

	assert.Equal(t, rpcPort, cfg.InstrumentServer.RPCPort)
	assert.Equal(t, autostart, cfg.InstrumentServer.AutoStart)

	require.Len(t, cfg.InstrumentServer.Instruments, 2)

	assert.Equal(
		t,
		instrument1Config,
		cfg.InstrumentServer.Instruments[0].ConfigPath,
	)
	assert.Equal(
		t,
		instrument1Plugin,
		cfg.InstrumentServer.Instruments[0].PluginPath,
	)

	assert.Equal(
		t,
		instrument2Config,
		cfg.InstrumentServer.Instruments[1].ConfigPath,
	)
	assert.Equal(
		t,
		instrument2Plugin,
		cfg.InstrumentServer.Instruments[1].PluginPath,
	)
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()

	cfgFile := filepath.Join(tmp, "hub.yaml")

	err := os.WriteFile(
		cfgFile,
		[]byte(`
instrument-server:
  instruments:
    - config: foo
      plugin: bad
    - :
`),
		0644,
	)
	require.NoError(t, err)

	_, err = LoadConfig(cfgFile)

	require.Error(t, err)
}

func TestLoadConfig_FileDoesNotExist(t *testing.T) {
	_, err := LoadConfig("does_not_exist.yaml")

	require.Error(t, err)
}

func TestValidate_NoInstruments(t *testing.T) {
	cfg := DefaultConfig()

	tmpDir := t.TempDir()

	cfg.WorkingDirectory = tmpDir

	err := Validate(&cfg)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"at least one instrument is required",
	)
}

func TestValidate_MissingPlugin(t *testing.T) {
	cfg := DefaultConfig()

	cfg.WorkingDirectory = t.TempDir()

	cfg.InstrumentServer.Instruments = []InstrumentConfig{
		{
			ConfigPath: "config.yaml",
		},
	}

	err := Validate(&cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin is required")
}

func TestValidate_MissingWorkingDirectory(t *testing.T) {
	cfg := DefaultConfig()

	cfg.WorkingDirectory = filepath.Join(
		t.TempDir(),
		"does-not-exist",
	)

	cfg.InstrumentServer.Instruments = []InstrumentConfig{
		{
			ConfigPath: "instrument.yaml", PluginPath: "plugin.so",
		},
	}

	err := Validate(&cfg)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"working directory does not exist",
	)
}

func TestUpdateConfig_OverridesFields(t *testing.T) {
	cfg := DefaultConfig()

	err := CLIOptions{
		NATSURL:          "nats://localhost:4222",
		DeviceConfig:     "device.yaml",
		Wiremap:          "wiremap.yaml",
		WorkingDirectory: "/tmp/test",
	}.Update(&cfg)

	require.NoError(t, err)

	assert.Equal(t, "nats://localhost:4222", cfg.NATSURL)
	assert.Equal(t, "device.yaml", cfg.QuantumDotConfig)
	assert.Equal(t, "wiremap.yaml", cfg.Wiremap)
	assert.Equal(t, "/tmp/test", cfg.WorkingDirectory)
}

func TestUpdateConfig_ParsesInstrumentList(t *testing.T) {
	cfg := DefaultConfig()

	err := CLIOptions{
		Instruments: []string{
			"config1.yaml:plugin1.so",
			"config2.yaml:plugin2.so",
		},
	}.Update(&cfg)

	require.NoError(t, err)

	require.Len(
		t,
		cfg.InstrumentServer.Instruments,
		2,
	)

	assert.Equal(
		t,
		"config1.yaml",
		cfg.InstrumentServer.Instruments[0].ConfigPath,
	)

	assert.Equal(
		t,
		"plugin1.so",
		cfg.InstrumentServer.Instruments[0].PluginPath,
	)
}

func TestUpdateConfig_InvalidInstrument(t *testing.T) {
	cfg := DefaultConfig()

	err := CLIOptions{
		Instruments: []string{
			"invalid",
		},
	}.Update(&cfg)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"expected config.yaml:plugin",
	)
}

func TestRootCommandFlags(t *testing.T) {
	cmd, _ := buildRootCmd(RuntimeDependencies{})
	require.NotNil(t, cmd.RunE)

	flags := cmd.Flags()

	tests := []string{
		"config",
		"nats-url",
		"device-config",
		"wiremap",
		"working-dir",
		"local-database",
		"user-measurement-luas",
		"measurement-metadata",
		"no-iss",
		"instrument",
	}

	for _, name := range tests {
		assert.NotNil(
			t,
			flags.Lookup(name),
			"missing flag %s",
			name,
		)
	}
}

func TestCheckEnvironment_SetsDefaultRPCPort(t *testing.T) {
	t.Setenv("INSTRUMENT_SCRIPT_SERVER_RPC_PORT", "")

	cfg := DefaultConfig()

	// Pretend we found ISS
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "instrument-script-server")

	err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0755)
	require.NoError(t, err)

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)

	err = CheckEnvironment(&cfg)
	require.NoError(t, err)

	assert.Equal(
		t,
		"8555",
		os.Getenv("INSTRUMENT_SCRIPT_SERVER_RPC_PORT"),
	)
}

func TestCheckEnvironment_RespectsExistingRPCPort(t *testing.T) {
	t.Setenv(
		"INSTRUMENT_SCRIPT_SERVER_RPC_PORT",
		"9999",
	)

	cfg := DefaultConfig()

	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "instrument-script-server")

	err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0755)
	require.NoError(t, err)

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)

	err = CheckEnvironment(&cfg)
	require.NoError(t, err)

	assert.Equal(t, 9999, cfg.InstrumentServer.RPCPort)
}

func TestCheckEnvironment_InvalidRPCPort(t *testing.T) {
	t.Setenv(
		"INSTRUMENT_SCRIPT_SERVER_RPC_PORT",
		"not-a-number",
	)

	cfg := DefaultConfig()

	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "instrument-script-server")

	err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0755)
	require.NoError(t, err)

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+originalPath)
	err = CheckEnvironment(&cfg)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"invalid INSTRUMENT_SCRIPT_SERVER_RPC_PORT",
	)
}

func TestCheckEnvironment_MissingISSBinary(t *testing.T) {
	t.Setenv("PATH", "")

	cfg := DefaultConfig()

	err := CheckEnvironment(&cfg)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"instrument-script-server binary not found",
	)
}

func TestInitializeRuntimeEnvironment_CreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := DefaultConfig()
	cfg.WorkingDirectory = tmpDir

	err := InitializeRuntimeEnvironment(&cfg)
	require.NoError(t, err)

	tests := []struct {
		name     string
		expected string
		actual   string
	}{
		{
			name:     LogsDir,
			expected: filepath.Join(tmpDir, LogsDir),
			actual:   cfg.RuntimePaths.Logs,
		},
		{
			name:     DataDir,
			expected: filepath.Join(tmpDir, DataDir),
			actual:   cfg.RuntimePaths.Data,
		},
		{
			name:     DataCacheDir,
			expected: filepath.Join(tmpDir, DataCacheDir),
			actual:   cfg.RuntimePaths.DataCache,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.DirExists(t, tt.expected)
			assert.Equal(t, tt.expected, tt.actual)
		})
	}
}

type FakeISSClient struct {
	started          []string
	closed           bool
	stopDaemonCalled bool
	closeCalled      bool
	status           bool // can set what shoudl happen

	startDaemonError     error
	stopDaemonError      error
	statusDaemonError    error
	startInstrumentError error
	stopInstrumentError  error
	measureError         error
	readError            error
	closeError           error
	listError            error
}

// For the purpose of this mock, the config is the name of the instrument that is started
func (f *FakeISSClient) StartInstrument(
	config string,
	plugin string,
) error {
	if f.startInstrumentError != nil {
		return f.startInstrumentError
	}
	f.started = append(
		f.started,
		config+":"+plugin,
	)
	return nil
}

func (f *FakeISSClient) StopInstrument(name string) error {
	if f.stopInstrumentError != nil {
		return f.stopInstrumentError
	}
	for i, val := range f.started {
		if val == name {
			f.started = append(f.started[:i], f.started[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("instrument %s not found", name)
}

func (f *FakeISSClient) ListInstruments() ([]string, error) {
	return f.started, f.listError
}

func (f *FakeISSClient) DaemonStatus() (bool, error) {
	return f.status, f.statusDaemonError
}

func (f *FakeISSClient) StopDaemon() error {
	f.stopDaemonCalled = true
	return f.stopDaemonError
}

func (f *FakeISSClient) Close() error {
	f.closed = true
	f.closeCalled = true
	return f.closeError
}

func (f *FakeISSClient) Measure(
	scriptPath string,
	globals map[string]interface{},
	typeManifest map[string]interface{},
) ([]serverinterpreter.ISSCallResult, error) {
	return nil, f.measureError
}

func (f *FakeISSClient) ReadBuffer(bufferID string) ([]float64, error) {
	return nil, f.readError
}

func TestStartInstruments(t *testing.T) {
	client := &FakeISSClient{}

	runtime := Runtime{
		cfg: &HubConfig{
			InstrumentServer: InstrumentServerConfig{
				Instruments: []InstrumentConfig{
					{
						ConfigPath: "a.yaml",
						PluginPath: "a.so",
					},
					{
						ConfigPath: "b.yaml",
						PluginPath: "b.so",
					},
				},
			},
		},
		issClient: client,
	}

	err := runtime.startInstruments()

	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{
			"a.yaml:a.so",
			"b.yaml:b.so",
		},
		client.started,
	)
}

func TestFailStartInstruments(t *testing.T) {
	client := &FakeISSClient{startInstrumentError: fmt.Errorf("Broken start")}

	runtime := Runtime{
		cfg: &HubConfig{
			InstrumentServer: InstrumentServerConfig{
				Instruments: []InstrumentConfig{
					{
						ConfigPath: "a.yaml",
						PluginPath: "a.so",
					},
					{
						ConfigPath: "b.yaml",
						PluginPath: "b.so",
					},
				},
			},
		},
		issClient: client,
	}

	err := runtime.startInstruments()

	require.Error(t, err)
}

func TestStopInstruments(t *testing.T) {
	client := &FakeISSClient{
		started: []string{
			"oscilloscope",
			"awg",
		},
	}

	runtime := Runtime{
		issClient: client,
	}

	runtime.stopInstruments()

	assert.Equal(
		t,
		[]string{},
		client.started,
	)
}

func TestFailStopInstruments(t *testing.T) {
	client := &FakeISSClient{
		started: []string{
			"oscilloscope",
			"awg",
		},
		stopInstrumentError: fmt.Errorf("Broken stop"),
	}

	runtime := Runtime{
		issClient: client,
	}

	runtime.stopInstruments()

	assert.Equal(
		t,
		[]string{
			"oscilloscope",
			"awg",
		},
		client.started,
	)
}

type FakeHandlerManager struct {
	stopped bool

	coreStarted   bool
	statusStarted bool

	startCoreHandlersError error
	startStatusError       error
	stopError              error
}

func (f *FakeHandlerManager) StartCoreHandlers() error {
	f.coreStarted = true
	return f.startCoreHandlersError
}

func (f *FakeHandlerManager) StartStatus() error {
	f.statusStarted = true
	return f.startStatusError
}

func (f *FakeHandlerManager) Stop() error {
	f.stopped = true
	return f.stopError
}

type FakeMeasurementManager struct {
	closed bool
	err    error
}

func (f *FakeMeasurementManager) Close() error {
	f.closed = true
	return f.err
}

type FakeNATSManager struct {
	closed bool
	conn   *nats.Conn
}

func (f *FakeNATSManager) GetConnection() *nats.Conn {
	return f.conn
}

func (f *FakeNATSManager) Close() {
	f.closed = true
}

func TestClose_ShutsDownServices(t *testing.T) {
	handler := &FakeHandlerManager{}
	measurements := &FakeMeasurementManager{}
	nats := &FakeNATSManager{}

	runtime := Runtime{
		handlerManager:     handler,
		measurementManager: measurements,
		natsManager:        nats,
	}

	runtime.Close()

	assert.True(t, handler.stopped)
	assert.True(t, measurements.closed)
	assert.True(t, nats.closed)
}

func TestClose_ShutsDownISS(t *testing.T) {
	client := &FakeISSClient{
		started: []string{
			"awg",
			"scope",
		},
	}

	runtime := Runtime{
		issClient:  client,
		issProcess: &os.Process{Pid: 1234},
	}

	runtime.Close()

	assert.True(t, client.stopDaemonCalled)
	assert.True(t, client.closeCalled)

	assert.Empty(
		t,
		client.started,
	)
}

func TestNewRuntime_NATSFailure(t *testing.T) {
	cfg := &HubConfig{
		InstrumentServer: InstrumentServerConfig{
			AutoStart: true,
		},
	}

	deps := RuntimeDependencies{
		newNATSManager: func(string) (NATSManager, error) {
			return nil, fmt.Errorf("broken nats")
		},
	}

	_, err := deps.NewRuntime(cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to setup nats")
}

func TestNewRuntime_MeasurementManagerFailure(t *testing.T) {
	cfg := &HubConfig{
		InstrumentServer: InstrumentServerConfig{
			AutoStart: true,
		},
		RuntimePaths: RuntimePaths{
			DataCache: t.TempDir(),
		},
	}

	deps := RuntimeDependencies{
		newNATSManager: func(string) (NATSManager, error) {
			return &FakeNATSManager{}, nil
		},
		newMeasurementManager: func(
			string,
			string,
		) (MeasurementManager, error) {
			return nil, fmt.Errorf("broken measurements")
		},
	}

	_, err := deps.NewRuntime(cfg)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"failed to initialize measurement manager",
	)
}

func TestNewRuntime_LoggerFailure(t *testing.T) {
	cfg := &HubConfig{
		InstrumentServer: InstrumentServerConfig{
			AutoStart: true,
		},
		RuntimePaths: RuntimePaths{
			DataCache: t.TempDir(),
			Logs:      t.TempDir(),
		},
	}

	deps := RuntimeDependencies{
		newNATSManager: func(string) (NATSManager, error) {
			return &FakeNATSManager{}, nil
		},
		newMeasurementManager: func(
			string,
			string,
		) (MeasurementManager, error) {
			return &FakeMeasurementManager{}, nil
		},
		newLogger: func(
			string,
		) (*logging.Logger, error) {
			return nil, fmt.Errorf("broken logger")
		},
	}

	_, err := deps.NewRuntime(cfg)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"failed to create logger",
	)
}

func TestWaitForISSDaemonReady_Success(t *testing.T) {
	client := &FakeISSClient{
		status: true,
	}

	deps := RuntimeDependencies{
		newISSClient: func(
			host string,
			port int,
			binary string,
		) ISSRuntimeClient {
			return client
		},
	}

	cfg := InstrumentServerConfig{
		RPCPort: 8555,
	}

	result, err := deps.waitForISSDaemonReady(
		cfg,
		50*time.Millisecond,
	)

	require.NoError(t, err)
	assert.Same(t, client, result)
}

func TestWaitForISSDaemonReady_Timeout(t *testing.T) {
	client := &FakeISSClient{
		status: false,
	}

	deps := RuntimeDependencies{
		newISSClient: func(
			host string,
			port int,
			binary string,
		) ISSRuntimeClient {
			return client
		},
	}

	cfg := InstrumentServerConfig{
		RPCPort: 8555,
	}

	_, err := deps.waitForISSDaemonReady(
		cfg,
		50*time.Millisecond,
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"did not become ready",
	)
}

func TestWaitForISSDaemonStopped_PortOpen(t *testing.T) {
	listener, err := net.Listen(
		"tcp",
		"127.0.0.1:0",
	)
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	cfg := InstrumentServerConfig{
		RPCPort: port,
	}

	assert.False(
		t,
		cfg.waitForISSDaemonStopped(
			50*time.Millisecond,
		),
	)
}

func TestWaitForISSDaemonStopped_PortClosed(t *testing.T) {
	listener, err := net.Listen(
		"tcp",
		"127.0.0.1:0",
	)
	require.NoError(t, err)

	port := listener.Addr().(*net.TCPAddr).Port

	require.NoError(t, listener.Close())

	cfg := InstrumentServerConfig{
		RPCPort: port,
	}

	assert.True(
		t,
		cfg.waitForISSDaemonStopped(
			time.Second,
		),
	)
}

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{
		"-test.run=TestHelperProcess",
		"--",
		command,
	}
	cs = append(cs, args...)

	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = append(
		os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
	)

	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	idx := slices.Index(os.Args, "--")
	if idx < 0 {
		os.Exit(1)
	}

	args := os.Args[idx+1:]

	if len(args) < 3 {
		os.Exit(1)
	}

	command := args[1]
	action := args[2]

	switch {
	case command == "daemon" && action == "start":
		fmt.Fprint(os.Stdout, "Daemon started")
		os.Exit(0)

	case command == "daemon" && action == "stop":
		os.Exit(0)
	}

	os.Exit(1)
}

func TestStopISSDaemonViaCLI(t *testing.T) {
	oldExec := execCommand
	execCommand = fakeExecCommand
	defer func() {
		execCommand = oldExec
	}()

	cfg := InstrumentServerConfig{
		ISSBinary: "instrument-script-server",
	}

	stopISSDaemonViaCLI(cfg)
}

func TestStartISSDaemon(t *testing.T) {
	oldExec := execCommand
	execCommand = fakeExecCommand
	defer func() {
		execCommand = oldExec
	}()

	cfg := &HubConfig{
		InstrumentServer: InstrumentServerConfig{
			ISSBinary: "instrument-script-server",
			RPCPort:   65534,
		},
	}

	r := Runtime{
		cfg: cfg,
	}
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	proc, err := r.startISSDaemon()

	require.NoError(t, err)
	require.NotNil(t, proc)
}
