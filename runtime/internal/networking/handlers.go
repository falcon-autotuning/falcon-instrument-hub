package networking

import (
	"log"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
	"github.com/nats-io/nats.go"
)

// HandlerManager manages NATS message handlers
type HandlerManager struct {
	natsConn           *nats.Conn
	subscriptions      []*nats.Subscription
	measurementManager *measurements.Manager
}

// NewHandlerManager creates a new handler manager
func NewHandlerManager(natsConn *nats.Conn, measurementManager *measurements.Manager) *HandlerManager {
	return &HandlerManager{
		natsConn:           natsConn,
		subscriptions:      make([]*nats.Subscription, 0),
		measurementManager: measurementManager,
	}
}

// RegisterAllHandlers sets up all NATS message handlers
func (hm *HandlerManager) RegisterAllHandlers() {
	if hm.natsConn == nil {
		log.Println("Warning: Cannot register handlers - NATS connection is nil")
		return
	}

	log.Println("Registering NATS message handlers...")

	// Register API command handlers
	hm.registerHandler("DEVICE_CONFIG_REQUEST", hm.handleDeviceConfigRequest)
	hm.registerHandler("PORT_REQUEST", hm.handlePortRequest)
	hm.registerHandler("STATUS", hm.handleStatusRequest)

	// Register instrument control handlers
	hm.registerHandler("SETUP_INSTRUMENT", hm.handleSetupInstrument)
	hm.registerHandler("DESTROY_INSTRUMENT", hm.handleDestroyInstrument)
	hm.registerHandler("PERFORM_INSTRUMENT_METHOD", hm.handlePerformInstrumentMethod)

	// Register measurement handlers
	hm.registerHandler("PROCESS_REQUEST", hm.handleProcessRequest)
	hm.registerHandler("MEASUREMENT_READY", hm.handleMeasurementReady)
	hm.registerHandler("ALLOCATE_MEASUREMENT_ID", hm.handleAllocateMeasurementID)
	hm.registerHandler("COMPLETE_MEASUREMENT", hm.handleCompleteMeasurement)
	hm.registerHandler("QUERY_MEASUREMENTS", hm.handleQueryMeasurements)

	log.Println("All handlers registered successfully")
}

// registerHandler is a helper to register individual handlers
func (hm *HandlerManager) registerHandler(subject string, handler nats.MsgHandler) {
	sub, err := hm.natsConn.Subscribe(subject, handler)
	if err != nil {
		log.Printf("Error subscribing to %s: %v", subject, err)
		return
	}
	log.Printf("Registered handler for subject: %s", subject)

	// Store subscription for cleanup
	hm.subscriptions = append(hm.subscriptions, sub)
}

// Close unsubscribes from all handlers
func (hm *HandlerManager) Close() {
	log.Println("Unsubscribing from NATS handlers...")
	for _, sub := range hm.subscriptions {
		if sub != nil {
			sub.Unsubscribe()
		}
	}
	hm.subscriptions = nil
}

// Handler methods - implement these based on your API requirements
func (hm *HandlerManager) handleDeviceConfigRequest(msg *nats.Msg) {
	log.Printf("Received DEVICE_CONFIG_REQUEST on subject: %s", msg.Subject)
	// TODO: Implement device config handling
}

func (hm *HandlerManager) handlePortRequest(msg *nats.Msg) {
	log.Printf("Received PORT_REQUEST on subject: %s", msg.Subject)
	// TODO: Implement port request handling
}

func (hm *HandlerManager) handleStatusRequest(msg *nats.Msg) {
	log.Printf("Received STATUS on subject: %s", msg.Subject)
	// TODO: Implement status handling
}

func (hm *HandlerManager) handleSetupInstrument(msg *nats.Msg) {
	log.Printf("Received SETUP_INSTRUMENT on subject: %s", msg.Subject)
	// TODO: Implement instrument setup
}

func (hm *HandlerManager) handleDestroyInstrument(msg *nats.Msg) {
	log.Printf("Received DESTROY_INSTRUMENT on subject: %s", msg.Subject)
	// TODO: Implement instrument destruction
}

func (hm *HandlerManager) handlePerformInstrumentMethod(msg *nats.Msg) {
	log.Printf("Received PERFORM_INSTRUMENT_METHOD on subject: %s", msg.Subject)
	// TODO: Implement instrument method execution
}

func (hm *HandlerManager) handleProcessRequest(msg *nats.Msg) {
	log.Printf("Received PROCESS_REQUEST on subject: %s", msg.Subject)
	// TODO: Implement process request handling
}

func (hm *HandlerManager) handleMeasurementReady(msg *nats.Msg) {
	log.Printf("Received MEASUREMENT_READY on subject: %s", msg.Subject)
	// TODO: Implement measurement ready handling
}

func (hm *HandlerManager) handleAllocateMeasurementID(msg *nats.Msg) {
	log.Printf("Received ALLOCATE_MEASUREMENT_ID on subject: %s", msg.Subject)
	// TODO: Implement measurement ID allocation
	// Example: uniqueID, filePath, err := hm.measurementManager.AllocateMeasurementID(time.Now())
}

func (hm *HandlerManager) handleCompleteMeasurement(msg *nats.Msg) {
	log.Printf("Received COMPLETE_MEASUREMENT on subject: %s", msg.Subject)
	// TODO: Implement measurement completion
	// Example: metadata, err := hm.measurementManager.CompleteMeasurement(uniqueID, title, filePath)
}

func (hm *HandlerManager) handleQueryMeasurements(msg *nats.Msg) {
	log.Printf("Received QUERY_MEASUREMENTS on subject: %s", msg.Subject)
	// TODO: Implement measurement query
	// Example: measurements, err := hm.measurementManager.QueryMeasurements(filters)
}
