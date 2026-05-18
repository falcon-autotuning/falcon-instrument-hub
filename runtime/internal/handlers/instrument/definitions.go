package instrument

import (
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
	"github.com/nats-io/nats.go"
)

// NATS subject and command name constants.
const (
	HandlerName              = "INSTRUMENT_HANDLER"
	SetupInstrumentCommand   = "SETUP_INSTRUMENT"
	SetupInstrumentSubject   = "SETUP_INSTRUMENT.external.*"
)

// Name is the unique identifier for a registered instrument.
type Name string

// ID is a numeric identifier used for measurement process IDs.
type ID int64

// MeasurementID combines a process ID and chunk ID for a single measurement.
type MeasurementID struct {
	ProcessId ID
	ChunkId   ID
}

// InstrumentProcess tracks the state of a registered instrument.
type InstrumentProcess struct {
	Name        Name
	Initialized bool
}

// Handler manages registered instruments and their port connections.
type Handler struct {
	logger        *logging.Logger
	Log           *LogWrapper
	natsURL       string
	nc            *nats.Conn
	Instruments   map[Name]*InstrumentProcess
	mutex         sync.RWMutex
	subscriptions []*nats.Subscription
	// PortConnections is the set of wired ports built from the API YAML files
	// and wiremap at startup. Each ConnectedPort maps a physical device gate to
	// a specific instrument channel io type.
	PortConnections []ports.ConnectedPort
}

// LogWrapper provides a named logger scoped to a handler.
type LogWrapper struct {
	logger *logging.Logger
	name   string
}

// NewLogWrapper creates a LogWrapper for the given handler name.
func NewLogWrapper(logger *logging.Logger, name string) *LogWrapper {
	return &LogWrapper{logger: logger, name: name}
}

// Debug logs a debug-level message.
func (l *LogWrapper) Debug(msg string) {
	l.logger.Debug(l.name, msg)
}

// Info logs an info-level message.
func (l *LogWrapper) Info(msg string) {
	l.logger.Info(l.name, msg)
}

// Error logs an error-level message.
func (l *LogWrapper) Error(msg string) {
	l.logger.Error(l.name, msg)
}

// CollectPortProperties partitions PortConnections into knobs (outputs) and
// meters (inputs). Settings are excluded.
func (h *Handler) CollectPortProperties() (knobs, meters []ports.ConnectedPort) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	for _, cp := range h.PortConnections {
		switch {
		case cp.IsKnob():
			knobs = append(knobs, cp)
		case cp.IsMeter():
			meters = append(meters, cp)
		}
	}
	return
}

// BuildConfigurations returns a routing map from device gate name to
// RouteInfo for all connected ports. This is passed to the ISS as
// ProcessRequest.Configurations.
func (h *Handler) BuildConfigurations() (map[string]ports.RouteInfo, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	configs := make(map[string]ports.RouteInfo, len(h.PortConnections))
	for _, cp := range h.PortConnections {
		configs[cp.DeviceName] = cp.RouteInfo()
	}
	return configs, nil
}
