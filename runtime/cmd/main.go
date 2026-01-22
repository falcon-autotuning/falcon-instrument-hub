package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/client"
	"github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/config"
	"github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/falcon-instrument-hub/runtime/internal/handlers/measure"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Falcon Instrument Hub Runtime")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded: RPC URL = %s", cfg.GetRPCBaseURL())

	// Create context that listens for interrupt signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Initialize handlers
	instrumentHandler := instrument.NewHandler(cfg)
	instrumentClient := client.NewInstrumentServerClient(cfg.GetRPCBaseURL())
	measureHandler := measure.NewHandler(instrumentClient)

	// Start the instrument-script-server daemon
	log.Println("Starting instrument-script-server daemon...")
	if err := instrumentHandler.StartDaemon(ctx); err != nil {
		log.Fatalf("Failed to start instrument-script-server daemon: %v", err)
	}
	defer func() {
		log.Println("Stopping instrument-script-server daemon...")
		if err := instrumentHandler.StopDaemon(context.Background()); err != nil {
			log.Printf("Error stopping daemon: %v", err)
		}
	}()

	log.Println("Falcon Instrument Hub Runtime is ready")
	log.Println("Press Ctrl+C to stop")

	// Example: List instruments (commented out for now)
	// instruments, err := instrumentHandler.ListInstruments(ctx)
	// if err != nil {
	//     log.Printf("Failed to list instruments: %v", err)
	// } else {
	//     log.Printf("Instruments: %+v", instruments)
	// }

	// TODO: Set up HTTP/gRPC API server to expose these handlers
	// This would provide endpoints for:
	// - POST /api/instruments/start
	// - POST /api/instruments/:name/stop
	// - GET /api/instruments/list
	// - POST /api/measurements/execute
	// - POST /api/measurements/from-script

	// For now, we just keep the daemon running
	// In production, this would be replaced with an API server

	// Example usage (commented out):
	// req := &compiler.MeasurementRequest{
	//     InstrumentName: "DMM",
	//     Command: "MEASURE",
	//     Parameters: map[string]interface{}{
	//         "range": "10V",
	//     },
	// }
	// result, err := measureHandler.ExecuteMeasurement(ctx, req)
	// if err != nil {
	//     log.Printf("Measurement failed: %v", err)
	// } else {
	//     log.Printf("Measurement result: %+v", result)
	// }

	// Wait for interrupt signal
	<-sigChan
	log.Println("Received interrupt signal, shutting down...")

	// Prevent usage warnings by referencing the handlers
	_ = instrumentHandler
	_ = measureHandler

	fmt.Println("Shutdown complete")
}
