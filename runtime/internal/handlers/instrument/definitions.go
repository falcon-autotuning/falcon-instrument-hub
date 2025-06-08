package instrument

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

// Handler Names
const (
	HandlerName = "INSTRUMENT_HANDLER"
)

// File Paths
const (
	ScriptsDir                 = "./"
	LaunchInstrumentScriptName = "launch_instrument_daemon.py"
	defaultPythonInterpreter   = "/home/daniel/work/wisc/playground/falcon/.venv/bin/python3"
)

// Process Management
const (
	GracefulShutdownTimeout = 5 // seconds
)

//go:embed launch_instrument_daemon.py
var embeddedScript embed.FS

var (
	SetupInstrumentCommand       = api.GetCommandName(api.SetupInstrument{})
	DestroyInstrumentCommand     = api.GetCommandName(api.DestroyInstrument{})
	ConfirmInitializationCommand = api.GetCommandName(
		api.ConfirmInitialization{},
	)
	UpdateDaemonPropertyCommand = api.GetCommandName(
		api.UpdateDaemonProperty{},
	)
	SetCommand                   = api.GetCommandName(api.Set{})
	SetupInstrumentSubject       = SetupInstrumentCommand + ".external.*"
	DestroyInstrumentSubject     = DestroyInstrumentCommand + ".external.*"
	ConfirmInitializationSubject = ConfirmInitializationCommand + ".*"
	UpdateDaemonPropertySubject  = UpdateDaemonPropertyCommand + ".instrument-server"
)

// InstrumentProcess represents a running instrument daemon
type InstrumentProcess struct {
	Name          string
	Process       *os.Process
	Cmd           *exec.Cmd
	Cancel        context.CancelFunc
	Ports         map[string]any
	Configuration map[string]any
	Initialized   bool
}

// Handler handles instrument setup and destruction
type Handler struct {
	logger            *logging.Logger
	natsURL           string
	nc                *nats.Conn
	Instruments       map[string]*InstrumentProcess
	mutex             sync.RWMutex
	subscriptions     []*nats.Subscription
	portProcessor     *PortProcessor
	PythonInterpreter string
}

// subscriptionConfig represents a subscription configuration
type subscriptionConfig struct {
	subject string
	handler nats.MsgHandler
	name    string
}

// CollectPortProperties collects port properties from all active instruments
func (h *Handler) CollectPortProperties() (knobs, meters []string) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.portProcessor != nil {
		return h.portProcessor.CollectPortProperties(h.Instruments)
	}
	return nil, nil
}

// BuildConfigurations creates the configuration mapping by collecting and
// inverting port mappings
func (h *Handler) BuildConfigurations() (map[string]map[string]any, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.portProcessor != nil {
		return h.portProcessor.BuildConfigurations(h.Instruments)
	}

	// Return empty map if no port processor available
	return make(map[string]map[string]any), nil
}

// BuildPortConfigurations builds the port configurations mapping
// Returns a mapping from port names to their configuration details
func (h *Handler) BuildPortConfigurations() (map[string]any, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.portProcessor != nil {
		return h.portProcessor.BuildPortConfigurations(h.Instruments)
	}

	// Return empty map if no port processor available
	return make(map[string]any), nil
}

// GetPortConfiguration finds the configuration for a specific port
func (h *Handler) GetPortConfiguration(
	portName string,
) (*PortConfiguration, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.portProcessor != nil {
		return h.portProcessor.GetPortConfiguration(portName, h.Instruments)
	}

	return nil, fmt.Errorf("no port processor available")
}

// InvalidatePortConfigCache invalidates the cached port configurations
// This should be called when instruments are added, removed, or reconfigured
func (h *Handler) InvalidatePortConfigCache() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if h.portProcessor != nil {
		h.portProcessor.InvalidatePortConfigCache()
	}
}

// AddInstrument adds an instrument and invalidates port cache
func (h *Handler) AddInstrument(name string, instrument *InstrumentProcess) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.Instruments[name] = instrument

	// Invalidate cache when instruments are modified
	if h.portProcessor != nil {
		h.portProcessor.InvalidatePortConfigCache()
	}
}

// RemoveInstrument removes an instrument and invalidates port cache
func (h *Handler) RemoveInstrument(name string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	delete(h.Instruments, name)

	// Invalidate cache when instruments are modified
	if h.portProcessor != nil {
		h.portProcessor.InvalidatePortConfigCache()
	}
}

// UpdateInstrumentPorts updates an instrument's ports and invalidates cache
func (h *Handler) UpdateInstrumentPorts(name string, ports map[string]any) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if instrument, exists := h.Instruments[name]; exists {
		instrument.Ports = ports

		// Invalidate cache when instrument ports are modified
		if h.portProcessor != nil {
			h.portProcessor.InvalidatePortConfigCache()
		}
	}
}

// UpdateInstrumentConfiguration updates an instrument's configuration and
// invalidates cache
func (h *Handler) UpdateInstrumentConfiguration(
	name string,
	config map[string]any,
) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if instrument, exists := h.Instruments[name]; exists {
		instrument.Configuration = config

		// Invalidate cache when instrument configuration is modified
		if h.portProcessor != nil {
			h.portProcessor.InvalidatePortConfigCache()
		}
	}
}

// SetInstrumentInitialized marks an instrument as initialized and invalidates
// cache
func (h *Handler) SetInstrumentInitialized(name string, initialized bool) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if instrument, exists := h.Instruments[name]; exists {
		instrument.Initialized = initialized

		// Invalidate cache when instrument initialization state changes
		if h.portProcessor != nil {
			h.portProcessor.InvalidatePortConfigCache()
		}
	}
}
