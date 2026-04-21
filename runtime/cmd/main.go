package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/networking"
	"github.com/spf13/cobra"
)

const (
	// Directory names
	LogsDir      = "log"
	DataDir      = "data"
	DataCacheDir = "datacache"

	// Database file name
	MeasurementsDB = "measurements.db"
)

var (
	packages      []string
	natsurl       string
	deviceconfig  string
	wiremap       string
	workingdir    string
	hubconfig     string
	issBinary     string
	issLibPath    string
	issNoAutoStart bool
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "start the falcon instrument server",
	Long:  "start the falcon instrument server with the specified configuration",
	RunE:  runStart,
}

func init() {
	startCmd.Flags().
		StringSliceVar(&packages, "packages", []string{}, "python modules containing instrument templates")
	startCmd.Flags().
		StringVar(&natsurl, "nats-url", "", "nats server url (if not provided, starts embedded nats)")
	startCmd.Flags().
		StringVar(&deviceconfig, "device-config", "", "path to device configuration yaml file")
	startCmd.Flags().
		StringVar(&wiremap, "wiremap", "", "path to wiremap yaml file")
	startCmd.Flags().
		StringVar(&workingdir, "working-dir", ".", "working directory for logs and data (default: current directory)")
	startCmd.Flags().
		StringVar(&hubconfig, "hub-config", "", "path to instrument_hub_config.yaml (sets device-config, wiremap, nats-url if not provided)")
	startCmd.Flags().
		StringVar(&issBinary, "iss-binary", "/opt/falcon/bin/instrument-script-server", "path to instrument-script-server binary")
	startCmd.Flags().
		StringVar(&issLibPath, "iss-lib-path", "", "additional library path prepended to LD_LIBRARY_PATH for instrument-script-server")
	startCmd.Flags().
		BoolVar(&issNoAutoStart, "no-iss", false, "skip auto-starting instrument-script-server daemon")
}

func runStart(cmd *cobra.Command, args []string) error {
	// apply hub config overrides before validation
	if err := applyHubConfig(); err != nil {
		return err
	}

	// validate and setup environment
	if err := initializeEnvironment(); err != nil {
		return err
	}

	// start instrument-script-server daemon unless disabled
	var issProcess *os.Process
	if !issNoAutoStart {
		proc, err := startISSDaemon()
		if err != nil {
			log.Printf("warning: could not start instrument-script-server: %v", err)
		} else {
			issProcess = proc
			log.Printf("instrument-script-server daemon started (pid=%d)", proc.Pid)
		}
	}

	// setup core services
	services, err := setupCoreServices()
	if err != nil {
		return err
	}
	services.issProcess = issProcess
	defer services.cleanup()

	// load configuration and create handlers
	if err := setupHandlers(services); err != nil {
		return err
	}

	// start the server
	return runServer(services)
}

type coreServices struct {
	natsManager        *networking.NATSManager
	measurementManager *measurements.Manager
	logger             *logging.Logger
	handlerManager     *handlers.Manager
	issProcess         *os.Process
}

func (s *coreServices) cleanup() {
	if s.handlerManager != nil {
		s.handlerManager.Stop()
	}
	if s.measurementManager != nil {
		s.measurementManager.Close()
	}
	if s.logger != nil {
		s.logger.Close()
	}
	if s.natsManager != nil {
		s.natsManager.Close()
	}
	if s.issProcess != nil {
		log.Println("stopping instrument-script-server daemon...")
		stopISSDaemon()
	}
}

func initializeEnvironment() error {
	// validate required files exist
	if err := validateFiles(); err != nil {
		return err
	}

	// change to working directory and create required folders
	if err := setupWorkingDirectory(); err != nil {
		return err
	}

	return nil
}

func setupCoreServices() (*coreServices, error) {
	services := &coreServices{}

	// set up nats connection using the networking package
	natsManager, err := networking.NewNATSManager(natsurl)
	if err != nil {
		return nil, fmt.Errorf("failed to setup nats: %w", err)
	}
	services.natsManager = natsManager

	// create measurement manager
	measurementManager, err := measurements.NewManager(
		filepath.Join(workingdir, DataDir),
		filepath.Join(workingdir, DataCacheDir, MeasurementsDB),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to initialize measurement manager: %w",
			err,
		)
	}
	services.measurementManager = measurementManager

	// create logger for handlers
	logger, err := logging.NewLogger(filepath.Join(workingdir, LogsDir))
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	services.logger = logger

	return services, nil
}

func setupHandlers(services *coreServices) error {
	if deviceconfig == "" || wiremap == "" {
		log.Println("warning: device-config or wiremap not specified, skipping handler setup")
		log.Println("hint: provide --device-config and --wiremap (or --hub-config) to enable full measurement handling")
		return nil
	}

	// load device configuration and wiremap first
	cfg, err := config.LoadConfig(deviceconfig, wiremap)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	log.Printf(
		"loaded device config with %d groups and %d wiring specs",
		len(cfg.DeviceConfig.Groups),
		len(cfg.DeviceConfig.WiringDC),
	)

	services.logger.LogStats()

	// create handler manager from handlers package
	services.handlerManager = handlers.NewManager(
		cfg,
		services.logger,
		services.natsManager.GetConnection(),
		services.natsManager.GetNATSURL(),
		services.measurementManager,
	)

	// subscribe all handlers using the handlers manager
	if err := services.handlerManager.Start(); err != nil {
		return fmt.Errorf("failed to start handlers: %w", err)
	}

	return nil
}

func runServer(services *coreServices) error {
	log.Printf("starting falcon instrument server...")
	log.Printf("packages: %v", packages)
	log.Printf("device config: %s", deviceconfig)
	log.Printf("wiremap: %s", wiremap)
	log.Printf("working directory: %s", workingdir)
	log.Printf(
		"nats url: %s",
		services.natsManager.GetConnection().ConnectedUrl(),
	)

	// todo: initialize python instrument templates with config paths
	// you can pass cfg.DeviceConfigPath and cfg.WiremapPath to python scripts

	log.Println("falcon runtime is ready and listening for commands...")

	// wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("received shutdown signal")
	return nil
}

func setupWorkingDirectory() error {
	// change to working directory
	if err := os.Chdir(workingdir); err != nil {
		return fmt.Errorf(
			"failed to change to working directory %s: %w",
			workingdir,
			err,
		)
	}

	// create log directory
	if err := os.MkdirAll(LogsDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// create data directory
	if err := os.MkdirAll(DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// create datacache directory for database indexes
	if err := os.MkdirAll(DataCacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create datacache directory: %w", err)
	}

	log.Printf("working directory set to: %s", workingdir)
	log.Printf(
		"created %s, %s, and %s directories",
		LogsDir,
		DataDir,
		DataCacheDir,
	)
	return nil
}

func validateFiles() error {
	// check device config file only if specified
	if deviceconfig != "" {
		if _, err := os.Stat(deviceconfig); os.IsNotExist(err) {
			return fmt.Errorf("device config file does not exist: %s", deviceconfig)
		}
	}

	// check wiremap file only if specified
	if wiremap != "" {
		if _, err := os.Stat(wiremap); os.IsNotExist(err) {
			return fmt.Errorf("wiremap file does not exist: %s", wiremap)
		}
	}

	return nil
}

// applyHubConfig reads instrument_hub_config.yaml and populates device-config,
// wiremap, and nats-url from it if those flags were not explicitly provided.
func applyHubConfig() error {
	if hubconfig == "" {
		return nil
	}

	data, err := os.ReadFile(hubconfig)
	if err != nil {
		return fmt.Errorf("failed to read hub config %s: %w", hubconfig, err)
	}

	var cfg struct {
		Wiremap          string `yaml:"wiremap"`
		QuantumDotConfig string `yaml:"quantum-dot-config"`
		NATSUrl          string `yaml:"nats-url"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse hub config: %w", err)
	}

	if wiremap == "" && cfg.Wiremap != "" {
		wiremap = cfg.Wiremap
		log.Printf("hub config: wiremap = %s", wiremap)
	}
	if deviceconfig == "" && cfg.QuantumDotConfig != "" {
		deviceconfig = cfg.QuantumDotConfig
		log.Printf("hub config: device-config = %s", deviceconfig)
	}
	if natsurl == "" && cfg.NATSUrl != "" {
		natsurl = cfg.NATSUrl
		log.Printf("hub config: nats-url = %s", natsurl)
	}

	return nil
}

// buildEnvWithLibPath returns os.Environ() with extra prepended to LD_LIBRARY_PATH.
func buildEnvWithLibPath(extra string) []string {
	env := os.Environ()
	if extra == "" {
		return env
	}
	for i, e := range env {
		if strings.HasPrefix(e, "LD_LIBRARY_PATH=") {
			existing := strings.TrimPrefix(e, "LD_LIBRARY_PATH=")
			if existing != "" {
				env[i] = "LD_LIBRARY_PATH=" + extra + ":" + existing
			} else {
				env[i] = "LD_LIBRARY_PATH=" + extra
			}
			return env
		}
	}
	return append(env, "LD_LIBRARY_PATH="+extra)
}

// startISSDaemon launches instrument-script-server daemon start in the background.
// Returns the OS process on success so the caller can track it.
func startISSDaemon() (*os.Process, error) {
	if _, err := os.Stat(issBinary); os.IsNotExist(err) {
		return nil, fmt.Errorf("instrument-script-server binary not found at %s", issBinary)
	}

	cmd := exec.Command(issBinary, "daemon", "start")
	cmd.Env = buildEnvWithLibPath(issLibPath)

	// Append to log file
	logFile, err := os.OpenFile("/tmp/iss-daemon.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start instrument-script-server: %w", err)
	}

	// Give the daemon a moment to initialize
	time.Sleep(500 * time.Millisecond)

	return cmd.Process, nil
}

// stopISSDaemon sends a stop command to the instrument-script-server daemon.
func stopISSDaemon() {
	cmd := exec.Command(issBinary, "daemon", "stop")
	cmd.Env = buildEnvWithLibPath(issLibPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("warning: instrument-script-server daemon stop returned: %v", err)
	}
}

func main() {

fmt.Println(`
   ._________________.            _____              _                                        _     
   |.---------------.|           |_   _|            | |                                      | |    
   ||               ||             | |   _ __   ___ | |_  _ __  _   _  _ __ ___    ___  _ __ | |_   
   ||     .--.      ||             | |  | '_ \ / __|| __|| '__|| | | || '_ ` + "`" + ` _ \  / _ \| '__|| __|
   ||    |o_o |     ||            _| |_ | | | |\__ \| |_ | |   | |_| || | | | | ||  __/| |   | |_   		
   ||    |:_/ |     ||           |_____||_| |_||___/ \__||_|    \__,_||_| |_| |_| \___||_|    \__|  
   ||   //   \ \    ||                                                                              
   ||  (|     | )   ||             _   _         _     
   || /'\_   _/` + "`" + `\   ||            | | | |       | |   
   || \___)=(___/   ||            | |_| | _   _ | |__ 
   ||_______________||            |  _  || | | || '_ \
   /.-.-.-.-.-.-.-.-.\            | | | || |_| || |_) |
  /.-.-.-.-.-.-.-.-.-.\           \_| |_/ \__,_||_.__/
 /.-.-.-.-.-.-.-.-.-.-.\                                                       
/______/__________\___o_\
\_______________________/
		||            
		||  connection
		||      
	.--------.  
	| DEVICE |
	|  INST  |
	'--------'
`)

	rootCmd := &cobra.Command{
		Use:   "instrument-hub",
		Short: "falcon instrument hub",
		Long:  "falcon instrument hub — orchestrates NATS, instrument-script-server, and measurement handlers",
	}
	rootCmd.AddCommand(startCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
