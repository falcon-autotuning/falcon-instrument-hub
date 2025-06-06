package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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
	packages     []string
	natsurl      string
	deviceconfig string
	wiremap      string
	workingdir   string
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "start the falcon instrument server",
	Long:  "start the falcon instrument server with the specified configuration",
	RunE:  runStart,
}

func init() {
	startCmd.Flags().
		StringSliceVar(&packages, "packages", []string{}, "python modules containing instrument templates (required)")
	startCmd.Flags().
		StringVar(&natsurl, "nats-url", "", "nats server url (if not provided, starts embedded nats)")
	startCmd.Flags().
		StringVar(&deviceconfig, "device-config", "", "path to device configuration yaml file (required)")
	startCmd.Flags().
		StringVar(&wiremap, "wiremap", "", "path to wiremap yaml file (required)")
	startCmd.Flags().
		StringVar(&workingdir, "working-dir", ".", "working directory for logs and data (default: current directory)")

	// mark required flags
	startCmd.MarkFlagRequired("packages")
	startCmd.MarkFlagRequired("device-config")
	startCmd.MarkFlagRequired("wiremap")
	startCmd.MarkFlagRequired("working-dir")
}

func runStart(cmd *cobra.Command, args []string) error {
	// validate and setup environment
	if err := initializeEnvironment(); err != nil {
		return err
	}

	// setup core services
	services, err := setupCoreServices()
	if err != nil {
		return err
	}
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
	// check device config file
	if _, err := os.Stat(deviceconfig); os.IsNotExist(err) {
		return fmt.Errorf("device config file does not exist: %s", deviceconfig)
	}

	// check wiremap file
	if _, err := os.Stat(wiremap); os.IsNotExist(err) {
		return fmt.Errorf("wiremap file does not exist: %s", wiremap)
	}

	// validate packages (basic check - could be enhanced)
	if len(packages) == 0 {
		return fmt.Errorf("at least one package must be specified")
	}

	return nil
}

func main() {
	fmt.Println("starting up the instrument server....")
	// execute the start command
	if err := startCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
