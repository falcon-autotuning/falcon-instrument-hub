package networking

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// NATSManager handles NATS server and connection management
type NATSManager struct {
	conn    *nats.Conn
	server  *server.Server
	natsURL string
}

// NewNATSManager creates a new NATS manager
func NewNATSManager(conf *config.NatsConfig) (*NATSManager, error) {
	var conn *nats.Conn
	var natsServer *server.Server
	var finalNatsURL string
	var err error
	if conf.Host == "" {
		conf.Host = "127.0.0.1"
	}
	if conf.Port == 0 {
		conf.Port = findAvailablePort()
	}

	// Start embedded NATS server
	log.Printf("Starting embedded NATS server...")
	natsServer, conn, finalNatsURL, err = startEmbeddedNATS(conf.Host, conf.Port)
	if err != nil {
		return nil, fmt.Errorf("failed to start embedded NATS server: %w", err)
	}

	return &NATSManager{
		conn:    conn,
		server:  natsServer,
		natsURL: finalNatsURL,
	}, nil
}

// GetNATSURL returns the NATS server URL
func (nm *NATSManager) GetNATSURL() string {
	return nm.natsURL
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
func startEmbeddedNATS(host string, port int) (*server.Server, *nats.Conn, string, error) {
	storeDir, err := os.MkdirTemp("", "nats-*")
	if err != nil {
		return nil, nil, "", err
	}
	// Configure embedded NATS server with JetStream enabled
	opts := &server.Options{
		Host:      host,
		Port:      port,
		JetStream: true,
		StoreDir:  storeDir,
	}

	// Start the server
	natsServer, err := server.NewServer(opts)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to create NATS server: %w", err)
	}

	// Start the server in a goroutine
	go natsServer.Start()

	// Wait for server to be ready
	if !natsServer.ReadyForConnections(5 * time.Second) {
		return nil, nil, "", fmt.Errorf("NATS server failed to start within timeout")
	}

	// Connect to the embedded server
	natsURL := fmt.Sprintf("nats://%s:%d", host, port)
	natsConn, err := nats.Connect(natsURL)
	if err != nil {
		natsServer.Shutdown()
		return nil, nil, "", fmt.Errorf("failed to connect to embedded NATS server: %w", err)
	}

	log.Printf("Embedded NATS server started at host:%s on port:%d", host, port)
	return natsServer, natsConn, natsURL, nil
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
	ln, err := net.Listen(
		"tcp",
		fmt.Sprintf("127.0.0.1:%d", port),
	)
	if err != nil {
		return false
	}

	_ = ln.Close()

	return true
}
