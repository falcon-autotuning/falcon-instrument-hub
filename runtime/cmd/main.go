package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/spf13/cobra"
)

var execCommand = exec.Command

const (
	DaemonStartStopPollTime = 10 * time.Millisecond
)

type InstrumentConfig struct {
	ConfigPath string `yaml:"config"`
	PluginPath string `yaml:"plugin"`
}

type InstrumentServerConfig struct {
	RPCPort     int                `yaml:"rpc-port"`
	AutoStart   bool               `yaml:"autostart"`
	Instruments []InstrumentConfig `yaml:"instruments"`
	ISSBinary   string             `yaml:"-"`
}

func (deps RuntimeDependencies) waitForISSDaemonReady(cfg InstrumentServerConfig, timeout time.Duration) (ISSRuntimeClient, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		client := deps.newISSClient(
			defaultHost,
			cfg.RPCPort,
			cfg.ISSBinary,
		)
		status, err := client.DaemonStatus()
		if (err == nil) && status {
			return client, nil
		}
		client.Close()
		lastErr = err
		time.Sleep(DaemonStartStopPollTime)
	}
	if lastErr == nil {
		return nil, fmt.Errorf("instrument-script-server daemon on %s:%d did not become ready within %s", defaultHost, cfg.RPCPort, timeout)
	}
	return nil, fmt.Errorf("instrument-script-server daemon on %s:%d did not become ready within %s: %w", defaultHost, cfg.RPCPort, timeout, lastErr)
}

func stopISSDaemonViaCLI(cfg InstrumentServerConfig) {
	cmd := execCommand(cfg.ISSBinary, "daemon", "stop")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("warning: instrument-script-server daemon stop returned: %v", err)
	}
}

func (cfg InstrumentServerConfig) waitForISSDaemonStopped(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	address := fmt.Sprintf("%s:%d", defaultHost, cfg.RPCPort)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err != nil {
			return true
		}
		_ = conn.Close()
		time.Sleep(DaemonStartStopPollTime)
	}
	return false
}

const (
	LogsDir      = "log"
	DataDir      = "data"
	DataCacheDir = "datacache"
)

type RuntimePaths struct {
	Logs      string
	Data      string
	DataCache string
}

type HubConfig struct {
	Wiremap                string                 `yaml:"wiremap"`
	QuantumDotConfig       string                 `yaml:"quantum-dot-config"`
	NATSURL                string                 `yaml:"nats-url"`
	LocalDatabase          string                 `yaml:"local-database"`
	WorkingDirectory       string                 `yaml:"working-directory"`
	UserMeasurementLuasDir string                 `yaml:"user-measurement-luas"`
	InstrumentServer       InstrumentServerConfig `yaml:"instrument-server"`
	RuntimePaths           RuntimePaths           `yaml:"-"`
}

func DefaultConfig() HubConfig {
	return HubConfig{
		InstrumentServer: InstrumentServerConfig{
			RPCPort:   8555,
			AutoStart: true,
		},
	}
}

func LoadConfig(path string) (*HubConfig, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func Validate(c *HubConfig) error {
	if c.QuantumDotConfig != "" {
		if _, err := os.Stat(c.QuantumDotConfig); os.IsNotExist(err) {
			return fmt.Errorf("device config file does not exist: %s", c.QuantumDotConfig)
		}
	}

	if c.UserMeasurementLuasDir != "" {
		if _, err := os.Stat(c.UserMeasurementLuasDir); os.IsNotExist(err) {
			return fmt.Errorf("the measurement luas dir does not exist: %s", c.UserMeasurementLuasDir)
		}
	}

	if c.Wiremap != "" {
		if _, err := os.Stat(c.Wiremap); os.IsNotExist(err) {
			return fmt.Errorf("wiremap file does not exist: %s", c.Wiremap)
		}
	}

	if c.WorkingDirectory == "" {
		var err error
		c.WorkingDirectory, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("Could not get the current working directory: %s", err)
		}
	}
	if _, err := os.Stat(c.WorkingDirectory); os.IsNotExist(err) {
		return fmt.Errorf("working directory does not exist: %s", c.WorkingDirectory)
	}

	if c.LocalDatabase == "" {
		c.LocalDatabase = filepath.Join(c.WorkingDirectory, DataDir)
	}

	if len(c.InstrumentServer.Instruments) == 0 {
		return fmt.Errorf("at least one instrument is required")
	}

	for i, inst := range c.InstrumentServer.Instruments {
		if inst.ConfigPath == "" {
			return fmt.Errorf("instrument[%d].config is required", i)
		}

		if inst.PluginPath == "" {
			return fmt.Errorf("instrument[%d].plugin is required", i)
		}
	}

	return nil
}

func CheckEnvironment(cfg *HubConfig) error {
	var err error
	cfg.InstrumentServer.ISSBinary, err = exec.LookPath("instrument-script-server")
	if err != nil {
		return fmt.Errorf("instrument-script-server binary not found in PATH: %w", err)
	}
	if portStr := os.Getenv("INSTRUMENT_SCRIPT_SERVER_RPC_PORT"); portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf(
				"invalid INSTRUMENT_SCRIPT_SERVER_RPC_PORT %q: %w",
				portStr,
				err,
			)
		}
		cfg.InstrumentServer.RPCPort = p
	} else {
		os.Setenv("INSTRUMENT_SCRIPT_SERVER_RPC_PORT", strconv.Itoa(cfg.InstrumentServer.RPCPort))
	}
	return nil
}

func InitializeRuntimeEnvironment(cfg *HubConfig) error {
	if err := os.Chdir(cfg.WorkingDirectory); err != nil {
		return fmt.Errorf("failed to change to the working directory: %w", err)
	}
	log.Printf("working directory set to: %s", cfg.WorkingDirectory)

	cfg.RuntimePaths.Logs = path.Join(cfg.WorkingDirectory, LogsDir)
	if err := os.MkdirAll(cfg.RuntimePaths.Logs, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	cfg.RuntimePaths.Data = path.Join(cfg.WorkingDirectory, DataDir)
	if err := os.MkdirAll(cfg.RuntimePaths.Data, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	cfg.RuntimePaths.DataCache = path.Join(cfg.WorkingDirectory, DataCacheDir)
	if err := os.MkdirAll(cfg.RuntimePaths.DataCache, 0755); err != nil {
		return fmt.Errorf("failed to create datacache directory: %w", err)
	}
	log.Printf(
		"created %s, %s, and %s directories",
		LogsDir,
		DataDir,
		DataCacheDir,
	)

	return nil
}

type Runtime struct {
	cfg *HubConfig

	natsManager        NATSManager
	measurementManager MeasurementManager
	logger             *logging.Logger
	handlerManager     HandlerManager
	issProcess         *os.Process
	issClient          ISSRuntimeClient
}

func (r *Runtime) startISSDaemon() (*os.Process, error) {
	// Stop any stale daemon from a previous run before starting fresh.
	instrumentServerConfig := r.cfg.InstrumentServer
	stopISSDaemonViaCLI(instrumentServerConfig)
	if !instrumentServerConfig.waitForISSDaemonStopped(5 * time.Second) {
		return nil, fmt.Errorf("instrument-script-server did not release port %d after stop", r.cfg.InstrumentServer.RPCPort)
	}

	cmd := execCommand(instrumentServerConfig.ISSBinary, "daemon", "start")
	env := os.Environ()
	cmd.Env = env

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		logPath := filepath.Join(LogsDir, "iss-daemon.log")
		if logFile, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); openErr == nil {
			_, _ = logFile.Write(output)
			_ = logFile.Close()
		}
	}
	outputText := string(output)
	if strings.Contains(outputText, "Daemon is already running") {
		return nil, fmt.Errorf("instrument-script-server refused fresh startup: %s", strings.TrimSpace(outputText))
	}
	if err != nil {
		return nil, fmt.Errorf("failed to start instrument-script-server: %w: %s", err, strings.TrimSpace(outputText))
	}
	if !strings.Contains(outputText, "Daemon started") {
		return nil, fmt.Errorf("instrument-script-server start did not report success: %s", strings.TrimSpace(outputText))
	}

	return cmd.Process, nil
}

func (r Runtime) startInstruments() error {
	for _, instrument := range r.cfg.InstrumentServer.Instruments {
		if err := r.issClient.StartInstrument(instrument.ConfigPath, instrument.PluginPath); err != nil {
			return fmt.Errorf("failed to start instrument at from %s via ISS RPC: %w", instrument.ConfigPath, err)
		}
		log.Printf("started instrument at: %s", instrument.ConfigPath)
	}
	return nil
}

const (
	MeasurementsDB = "measurements.db"
)

func (deps RuntimeDependencies) NewRuntime(
	cfg *HubConfig,
) (*Runtime, error) {
	services := &Runtime{
		cfg: cfg,
	}

	if !cfg.InstrumentServer.AutoStart {
		proc, err := services.startISSDaemon()
		if err != nil {
			stopISSDaemonViaCLI(cfg.InstrumentServer)
			return nil, fmt.Errorf("could not stop instrument-script-server via cli: %w", err)
		}
		client, err := deps.waitForISSDaemonReady(cfg.InstrumentServer, 10*time.Second)
		if err != nil {
			return services, fmt.Errorf("could not start instrument-script-server: %w", err)
		}
		services.issProcess = proc
		services.issClient = client
		log.Printf("instrument-script-server daemon started (pid=%d)", proc.Pid)
	}
	// TODO: figure out how to reattach to existing ISS daemon if it is running

	natsManager, err := deps.newNATSManager(cfg.NATSURL)
	if err != nil {
		return nil, fmt.Errorf("failed to setup nats: %w", err)
	}
	services.natsManager = natsManager

	measurementManager, err := deps.newMeasurementManager(
		cfg.LocalDatabase,
		filepath.Join(cfg.RuntimePaths.DataCache, MeasurementsDB),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to initialize measurement manager: %w",
			err,
		)
	}
	services.measurementManager = measurementManager

	logger, err := deps.newLogger(cfg.RuntimePaths.Logs)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	services.logger = logger

	dispatcher := deps.newDispatcher(
		services.issClient,
		cfg.UserMeasurementLuasDir,
	)

	configJSON, err := deps.newConfig(cfg.QuantumDotConfig)
	if err != nil {
		return services, fmt.Errorf("failed to load configuration: %w", err)
	}
	wiremap, err := deps.newWiremap(cfg.Wiremap, cfg.QuantumDotConfig)
	if err != nil {
		return services, fmt.Errorf("failed to load wiremap: %w", err)
	}

	logger.LogStats()

	handlerManager := deps.newHandlerManager(
		configJSON,
		wiremap,
		[]string{},
		// instrumentAPIPaths,      // FIX: Can get generated from the instrument configs
		"",
		// measurementMetadataPath, // FIX:what are these?
		cfg.UserMeasurementLuasDir,
		logger,
		natsManager.GetConnection(),
		dispatcher,
	)
	services.handlerManager = handlerManager

	// Subscribe operational handlers first. Status publishing starts only after
	// ISS instruments are started so STATUS.instrument-server means fully ready.
	if err := handlerManager.StartCoreHandlers(); err != nil {
		return services, fmt.Errorf("failed to start handlers: %w", err)
	}
	if !cfg.InstrumentServer.AutoStart && services.issProcess != nil {
		if err := services.startInstruments(); err != nil {
			return services, fmt.Errorf("failed to start instruments: %w", err)
		}
	}
	if handlerManager != nil {
		if err := handlerManager.StartStatus(); err != nil {
			return services, fmt.Errorf("failed to start status handler: %w", err)
		}
	}

	return services, nil
}

func (r Runtime) stopInstruments() {
	instruments, err := r.issClient.ListInstruments()
	if err != nil {
		log.Printf("warning: could not list instruments for shutdown: %v", err)
		return
	}
	for _, name := range instruments {
		if err := r.issClient.StopInstrument(name); err != nil {
			log.Printf("warning: failed to stop instrument %s: %v", name, err)
		} else {
			log.Printf("stopped instrument: %s", name)
		}
	}
}

func (r *Runtime) Close() {
	if r.handlerManager != nil {
		r.handlerManager.Stop()
	}
	if r.measurementManager != nil {
		r.measurementManager.Close()
	}
	if r.logger != nil {
		r.logger.Close()
	}
	if r.natsManager != nil {
		r.natsManager.Close()
	}
	if r.issProcess != nil {
		log.Println("stopping instrument-script-server daemon...")
		r.stopInstruments()
		if err := r.issClient.StopDaemon(); err != nil {
			log.Printf("warning: instrument-script-server daemon stop returned: %v", err)
		}
	}
	if r.issClient != nil {
		r.issClient.Close()
	}
}

func (r Runtime) runServer() error {
	log.Printf("starting Falcon Instrument Hub...")
	log.Printf("device config: %s", r.cfg.QuantumDotConfig)
	log.Printf("wiremap: %s", r.cfg.Wiremap)
	log.Printf("working directory: %s", r.cfg.WorkingDirectory)
	log.Printf(
		"nats url: %s",
		r.natsManager.GetConnection().ConnectedUrl(),
	)

	log.Println("Falcon Instrument Hub is ready and listening for commands...")

	// wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("received shutdown signal")
	return nil
}

type CLIOptions struct {
	Config              string
	NATSURL             string
	Wiremap             string
	DeviceConfig        string
	WorkingDirectory    string
	LocalDatabase       string
	UserMeasurementLua  string
	MeasurementMetadata string
	LogDiagnostics      bool
	NoISS               bool

	// repeatable:
	// --instrument config.yaml:plugin
	Instruments []string
}

func (cli CLIOptions) Update(cfg *HubConfig) error {
	if cli.NATSURL != "" {
		cfg.NATSURL = cli.NATSURL
	}

	if cli.Wiremap != "" {
		cfg.Wiremap = cli.Wiremap
	}

	if cli.DeviceConfig != "" {
		cfg.QuantumDotConfig = cli.DeviceConfig
	}

	if cli.WorkingDirectory != "" {
		cfg.WorkingDirectory = cli.WorkingDirectory
	}

	if cli.LocalDatabase != "" {
		cfg.LocalDatabase = cli.LocalDatabase
	}

	if cli.UserMeasurementLua != "" {
		cfg.UserMeasurementLuasDir = cli.UserMeasurementLua
	}

	if cli.NoISS {
		cfg.InstrumentServer.AutoStart = false
	}

	if len(cli.Instruments) > 0 {
		cfg.InstrumentServer.Instruments = nil

		for _, instrument := range cli.Instruments {
			parts := strings.SplitN(instrument, ":", 2)

			if len(parts) != 2 {
				return fmt.Errorf(
					"invalid instrument %q, expected config.yaml:plugin",
					instrument,
				)
			}

			cfg.InstrumentServer.Instruments = append(
				cfg.InstrumentServer.Instruments,
				InstrumentConfig{
					ConfigPath: parts[0],
					PluginPath: parts[1],
				},
			)
		}
	}

	return nil
}

func NewRunHub(
	deps RuntimeDependencies,
	cli *CLIOptions,
) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cfg := DefaultConfig()

		if cli.Config != "" {
			loadedCfg, err := LoadConfig(cli.Config)
			if err != nil {
				return err
			}

			cfg = *loadedCfg
		}

		if err := cli.Update(&cfg); err != nil {
			return err
		}

		if err := Validate(&cfg); err != nil {
			return err
		}

		if err := CheckEnvironment(&cfg); err != nil {
			return err
		}

		if err := InitializeRuntimeEnvironment(&cfg); err != nil {
			return err
		}

		runtime, err := deps.NewRuntime(&cfg)
		if err != nil {
			return err
		}
		defer runtime.Close()
		return runtime.runServer()
	}
}

func buildRootCmd(deps RuntimeDependencies) (*cobra.Command, *CLIOptions) {
	cli := &CLIOptions{}
	rootCmd := &cobra.Command{
		Use:   "instrument-hub",
		Short: "Falcon Instrument Hub",
		Long:  "Falcon Instrument Hub orchestrates NATS, instrument-script-server, and measurement handlers",
		RunE:  NewRunHub(deps, cli),
	}

	flags := rootCmd.Flags()

	flags.StringVar(
		&cli.Config,
		"config",
		"",
		"path to hub configuration yaml",
	)

	flags.StringVar(
		&cli.NATSURL,
		"nats-url",
		"",
		"nats server url",
	)

	flags.StringVar(
		&cli.DeviceConfig,
		"device-config",
		"",
		"path to device configuration yaml",
	)

	flags.StringVar(
		&cli.Wiremap,
		"wiremap",
		"",
		"path to wiremap yaml",
	)

	flags.StringVar(
		&cli.WorkingDirectory,
		"working-dir",
		"",
		"working directory",
	)

	flags.StringVar(
		&cli.LocalDatabase,
		"local-database",
		"",
		"path to local database",
	)

	flags.StringVar(
		&cli.UserMeasurementLua,
		"user-measurement-luas",
		"",
		"path to user lua measurement scripts",
	)

	flags.StringVar(
		&cli.MeasurementMetadata,
		"measurement-metadata",
		"",
		"path to measurement metadata yaml",
	)

	flags.BoolVar(
		&cli.NoISS,
		"no-iss",
		false,
		"disable ISS autostart",
	)

	flags.StringSliceVar(
		&cli.Instruments,
		"instrument",
		nil,
		"instrument definition: config.yaml:plugin",
	)
	return rootCmd, cli
}

func main() {
	fmt.Print(`
 ______                        __                                                             __     
|      \                      |  \                                                           |  \    
 \$$$$$$ _______    _______  _| $$_     ______   __    __  ______ ____    ______   _______  _| $$_   
  | $$  |       \  /       \|   $$ \   /      \ |  \  |  \|      \    \  /      \ |       \|   $$ \  
  | $$  | $$$$$$$\|  $$$$$$$ \$$$$$$  |  $$$$$$\| $$  | $$| $$$$$$\$$$$\|  $$$$$$\| $$$$$$$\\$$$$$$  
  | $$  | $$  | $$ \$$    \   | $$ __ | $$   \$$| $$  | $$| $$ | $$ | $$| $$    $$| $$  | $$ | $$ __ 
 _| $$_ | $$  | $$ _\$$$$$$\  | $$|  \| $$      | $$__/ $$| $$ | $$ | $$| $$$$$$$$| $$  | $$ | $$|  \
|   $$ \| $$  | $$|       $$   \$$  $$| $$       \$$    $$| $$ | $$ | $$ \$$     \| $$  | $$  \$$  $$
 \$$$$$$ \$$   \$$ \$$$$$$$     \$$$$  \$$        \$$$$$$  \$$  \$$  \$$  \$$$$$$$ \$$   \$$   \$$$$ 
                                                                                                     
 __    __            __                                                                              
|  \  |  \          |  \                                                                             
| $$  | $$ __    __ | $$____                                                                         
| $$__| $$|  \  |  \| $$    \                                                                        
| $$    $$| $$  | $$| $$$$$$$\                                                                       
| $$$$$$$$| $$  | $$| $$  | $$                                                                       
| $$  | $$| $$__/ $$| $$__/ $$                                                                       
| $$  | $$ \$$    $$| $$    $$                                                                       
 \$$   \$$  \$$$$$$  \$$$$$$$                                                                        
`)
	rootCmd, _ := buildRootCmd(ProductionDependancies)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
