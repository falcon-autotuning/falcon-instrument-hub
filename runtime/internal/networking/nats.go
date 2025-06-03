package networking

import (
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// NATSManager handles NATS server and connection management
type NATSManager struct {
	conn   *nats.Conn
	server *server.Server
}

// NewNATSManager creates a new NATS manager
func NewNATSManager(natsURL string) (*NATSManager, error) {
	var conn *nats.Conn
	var natsServer *server.Server
	var err error

	if natsURL != "" {
		// Connect to external NATS server
		log.Printf("Connecting to external NATS server: %s", natsURL)
		conn, err = nats.Connect(natsURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to NATS server: %w", err)
		}
	} else {
		// Start embedded NATS server
		log.Printf("Starting embedded NATS server...")
		natsServer, conn, err = startEmbeddedNATS()
		if err != nil {
			return nil, fmt.Errorf("failed to start embedded NATS server: %w", err)
		}
	}

	return &NATSManager{
		conn:   conn,
		server: natsServer,
	}, nil
}

// GetConnection returns the NATS connection
func (nm *NATSManager) GetConnection() *nats.Conn {
	return nm.conn
}

// Close gracefully shuts down the NATS manager
func (nm *NATSManager) Close() {
	// Close NATS connection
	if nm.conn != nil {
		log.Println("Closing NATS connection...")
		nm.conn.Close()
		nm.conn = nil
	}

	// Shutdown embedded NATS server
	if nm.server != nil {
		log.Println("Shutting down embedded NATS server...")
		nm.server.Shutdown()
		nm.server = nil
	}
}

// startEmbeddedNATS starts an embedded NATS server
func startEmbeddedNATS() (*server.Server, *nats.Conn, error) {
	// Find an available port
	port := findAvailablePort()

	// Configure embedded NATS server
	opts := &server.Options{
		Host: "127.0.0.1",
		Port: port,
	}

	// Start the server
	natsServer, err := server.NewServer(opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create NATS server: %w", err)
	}

	// Start the server in a goroutine
	go natsServer.Start()

	// Wait for server to be ready
	if !natsServer.ReadyForConnections(5 * time.Second) {
		return nil, nil, fmt.Errorf("NATS server failed to start within timeout")
	}

	// Connect to the embedded server
	natsURL := fmt.Sprintf("nats://127.0.0.1:%d", port)
	natsConn, err := nats.Connect(natsURL)
	if err != nil {
		natsServer.Shutdown()
		return nil, nil, fmt.Errorf("failed to connect to embedded NATS server: %w", err)
	}

	log.Printf("Embedded NATS server started on port %d", port)
	return natsServer, natsConn, nil
}

// findAvailablePort finds an available port for the NATS server
func findAvailablePort() int {
	// Simple approach: try a range of ports starting from 4222 (default NATS port)
	for port := 4222; port < 4300; port++ {
		if isPortAvailable(port) {
			return port
		}
	}
	// Fallback to a higher range
	for port := 14222; port < 14300; port++ {
		if isPortAvailable(port) {
			return port
		}
	}
	// Default fallback
	return 4222
}

// isPortAvailable checks if a port is available
func isPortAvailable(port int) bool {
	// Simple check - try to create a test server
	opts := &server.Options{
		Host: "127.0.0.1",
		Port: port,
	}
	testServer, err := server.NewServer(opts)
	if err != nil {
		return false
	}
	defer testServer.Shutdown()

	// If we can create it, the port is likely available
	return true
}
