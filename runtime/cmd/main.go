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

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
	"github.com/spf13/cobra"
)

var execCommand = exec.Command

const (
	DaemonStartStopPollTime = 10 * time.Millisecond
	LogsDir                 = "log"
	DataDir                 = "data"
	DataCacheDir            = "datacache"
)

func (deps RuntimeDependencies) waitForISSDaemonReady(cfg config.InstrumentServerConfig, timeout time.Duration) (ISSClient, error) {
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

func stopISSDaemonViaCLI(cfg config.InstrumentServerConfig) {
	cmd := execCommand(cfg.ISSBinary, "daemon", "stop")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("warning: instrument-script-server daemon stop returned: %v", err)
	}
}

func waitForISSDaemonStopped(cfg config.InstrumentServerConfig, timeout time.Duration) bool {
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

func CheckEnvironment(cfg *config.HubConfig) error {
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

func InitializeRuntimeEnvironment(cfg *config.HubConfig) error {
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
	cfg *config.HubConfig

	natsManager    NATSManager
	logger         *logging.Logger
	handlerManager HandlerManager
	issProcess     *os.Process
	issClient      ISSClient
}

func (r *Runtime) startISSDaemon() (*os.Process, error) {
	// TODO: See if we can attach to already running daemon
	// Stop any stale daemon from a previous run before starting fresh.
	instrumentServerConfig := r.cfg.InstrumentServer
	stopISSDaemonViaCLI(instrumentServerConfig)
	if !waitForISSDaemonStopped(instrumentServerConfig, 5*time.Second) {
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

func BuildInstrumentMetadata(
	cfg *config.HubConfig,
) map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata {
	result := make(
		map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	)

	for _, instrument := range cfg.InstrumentServer.Instruments {
		if instrument.ConfigFile == nil {
			continue
		}

		result[settingrouter.InstrumentName(
			instrument.ConfigFile.Name,
		)] = settingrouter.InstrumentMetadata{
			Characteristics: instrument.Characteristics,
		}
	}

	return result
}

func (deps RuntimeDependencies) NewRuntime(
	cfg *config.HubConfig,
) (*Runtime, error) {
	services := &Runtime{
		cfg: cfg,
	}

	// Production supplies newISSClient. Tests may omit it when exercising an
	// unrelated startup failure before handler construction.
	if deps.newISSClient != nil {
		if cfg.InstrumentServer.AutoStart {
			proc, err := services.startISSDaemon()
			if err != nil {
				stopISSDaemonViaCLI(cfg.InstrumentServer)
				return nil, fmt.Errorf("could not start instrument-script-server: %w", err)
			}
			services.issProcess = proc
			if proc != nil {
				log.Printf("instrument-script-server daemon started (pid=%d)", proc.Pid)
			}
		}

		client, err := deps.waitForISSDaemonReady(cfg.InstrumentServer, 10*time.Second)
		if err != nil {
			if services.issProcess != nil {
				stopISSDaemonViaCLI(cfg.InstrumentServer)
			}
			return services, fmt.Errorf("could not connect to instrument-script-server: %w", err)
		}
		services.issClient = client
	}

	natsManager, err := deps.newNATSManager(cfg.NATSURL)
	if err != nil {
		return nil, fmt.Errorf("failed to setup nats: %w", err)
	}
	services.natsManager = natsManager

	logger, err := deps.newLogger(cfg.RuntimePaths.Logs)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	services.logger = logger

	configJSON, err := deps.configProvider.LoadDeviceConfig(cfg.QuantumDotConfig)
	if err != nil {
		return services, fmt.Errorf("failed to load configuration: %w", err)
	}
	ports, err := deps.configProvider.NewConnectedPorts(cfg.InstrumentServer.Instruments, cfg.Wiremap)

	logger.LogStats()

	handlerManager := deps.newHandlerManager(
		configJSON,
		cfg.Wiremap,
		ports,
		cfg.UserMeasurementLuasDir,
		logger,
		natsManager.GetConnection(),
		services.issClient,
		BuildInstrumentMetadata(cfg),
	)
	services.handlerManager = handlerManager

	// Subscribe operational handlers first. Status publishing starts only after
	// ISS instruments are started so STATUS.instrument-server means fully ready.
	if err := handlerManager.StartCoreHandlers(); err != nil {
		return services, fmt.Errorf("failed to start handlers: %w", err)
	}
	if cfg.InstrumentServer.AutoStart && services.issProcess != nil {
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
	Config string
}

func NewRunHub(
	deps RuntimeDependencies,
	cli *CLIOptions,
) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cfg := config.DefaultConfig()

		if cli.Config != "" {
			loadedCfg, err := config.LoadConfig(cli.Config)
			if err != nil {
				return err
			}

			cfg = *loadedCfg
		}

		if err := config.Validate(&cfg); err != nil {
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
