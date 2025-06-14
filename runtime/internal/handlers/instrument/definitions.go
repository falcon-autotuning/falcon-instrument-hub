package instrument

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

const (
	HandlerName                string         = "INSTRUMENT_HANDLER"
	Knob                       port           = "Knob"
	Meter                      port           = "Meter"
	Port                       port           = "InstrumentPort"
	ScriptsDir                 string         = "scripts"
	LaunchInstrumentScriptName string         = "launch_instrument_daemon.py"
	GracefulShutdownTimeout    int64          = 5 // seconds
	ScreeningGate              connectionType = "ScreeningGate"
	BarrierGate                connectionType = "BarrierGate"
	ReservoirGate              connectionType = "ReservoirGate"
	PlungerGate                connectionType = "PlungerGate"
	Ohmic                      connectionType = "Ohmic"
	screeningModule            string         = "screening_gate"
	barrierModule              string         = "barrier_gate"
	reservoirModule            string         = "reservoir_gate"
	plungerModule              string         = "plunger_gate"
	ohmicModule                string         = "ohmic"
	falconCoreModuleTemplate   string         = "falcon_core.physics.device_structures."
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

var connectionToModule = map[connectionType]module{
	ScreeningGate: module(falconCoreModuleTemplate + screeningModule),
	PlungerGate:   module(falconCoreModuleTemplate + plungerModule),
	BarrierGate:   module(falconCoreModuleTemplate + barrierModule),
	ReservoirGate: module(falconCoreModuleTemplate + reservoirModule),
	Ohmic:         module(falconCoreModuleTemplate + ohmicModule),
}

// InstrumentProcess represents a running instrument daemon
type InstrumentProcess struct {
	Name          Name
	Cmd           *exec.Cmd
	Cancel        context.CancelFunc
	Ports         propertyIndexedPorts
	Configuration map[PropertyName]map[Index]PortConfiguration
	Initialized   bool
	StartTime     time.Time
	Stdout        *bytes.Buffer
	Stderr        *bytes.Buffer
	Completed     bool
	CompletedAt   time.Time
	ExitError     error
}

// Handler handles instrument setup and destruction
type Handler struct {
	logger            *logging.Logger
	Log               *LogWrapper
	natsURL           string
	nc                *nats.Conn
	Instruments       map[Name]*InstrumentProcess
	mutex             sync.RWMutex
	subscriptions     []*nats.Subscription
	portProcessor     *PortProcessor
	pythonInterpreter string
	cleanupStop       chan struct{}
}

// subscriptionConfig represents a subscription configuration
type subscriptionConfig struct {
	subject string
	handler nats.MsgHandler
	name    string
}

// a name for an insturment
type Name string

// ConnectionType represents the type of connection
type (
	connectionType         string
	module                 string
	port                   string
	PropertyName           string
	Index                  string
	propertyIndexedPorts   map[PropertyName]map[Index]JsonPort
	instrumentIndexedPorts map[Name]propertyIndexedPorts
	JsonPort               string
	PortConfiguration      map[string]any
)

// PsuedoName represents a pythonic name that falcon understands
type PsuedoName struct {
	Class  connectionType              `json:"__class__"`
	Module module                      `json:"__module__"`
	Name   config.InstrumentConnection `json:"name"`
}

// units represents the pythonic units for a port
type (
	units       map[string]any
	defaultName string
)

// PortObject represents a generic port (knob or meter)
type PortObject struct {
	Class          port        `json:"__class__"`
	Module         module      `json:"__module__"`
	DefaultName    defaultName `json:"default_name"`
	PseudoName     PsuedoName  `json:"pseudo_name"`
	InstrumentType string      `json:"instrument_type"`
	Units          units       `json:"units"`
	Description    string      `json:"description"`
}

// IsKnob returns true if this port is a knob
func (p *PortObject) IsKnob() bool {
	return p.Class == Knob
}

// IsMeter returns true if this port is a meter
func (p *PortObject) IsMeter() bool {
	return p.Class == Meter
}

// IsPort return true if this port is a port
func (p *PortObject) IsPort() bool {
	return p.Class == Knob
}

// FromInterface unmarshals from interface{} (string or map) into PortObject
func (p *PortObject) FromInterface(portValue JsonPort) error {
	return json.Unmarshal([]byte(portValue), p)
}

// ToInterface converts PortObject back to interface{} (matching original
// format)
func (p *PortObject) ToInterface() (JsonPort, error) {
	portBytes, err := json.Marshal(p)
	return JsonPort(string(portBytes)), err
}

func (jp JsonPort) Contains(other string) bool {
	// Check if the JsonPort contains the other string
	return strings.Contains(jp.String(), other)
}

func (jp JsonPort) String() string {
	return string(jp)
}

// CollectPortProperties collects port properties from all active instruments
func (h *Handler) CollectPortProperties() (knobs, meters []JsonPort) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.portProcessor != nil {
		return h.portProcessor.CollectPortProperties(h.Instruments)
	}
	return nil, nil
}

// BuildConfigurations creates the configuration mapping by collecting and
// inverting port mappings
func (h *Handler) BuildConfigurations() (map[JsonPort]map[PropertyName]PortConfiguration, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.portProcessor != nil {
		return h.portProcessor.BuildConfigurations(h.Instruments)
	}

	// Return empty map if no port processor available
	return make(map[JsonPort]map[PropertyName]PortConfiguration), nil
}

// BuildPortConfigurations builds the port configurations mapping
// Returns a mapping from port names to their configuration details
func (h *Handler) BuildPortConfigurations() (map[JsonPort]PortOptions, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.portProcessor != nil {
		return h.portProcessor.BuildPortConfigurations(h.Instruments)
	}

	// Return empty map if no port processor available
	return make(map[JsonPort]PortOptions), nil
}

// GetPortConfiguration finds the configuration for a specific port
func (h *Handler) GetPortOptions(
	name JsonPort,
) (*PortOptions, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.portProcessor != nil {
		return h.portProcessor.GetPortConfiguration(name, h.Instruments)
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
func (h *Handler) AddInstrument(
	name Name,
	instrument *InstrumentProcess,
) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.Instruments[name] = instrument

	// Invalidate cache when instruments are modified
	if h.portProcessor != nil {
		h.portProcessor.InvalidatePortConfigCache()
	}
}

// RemoveInstrument removes an instrument and invalidates port cache
func (h *Handler) RemoveInstrument(name Name) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	delete(h.Instruments, name)

	// Invalidate cache when instruments are modified
	if h.portProcessor != nil {
		h.portProcessor.InvalidatePortConfigCache()
	}
}

// UpdateInstrumentConfiguration updates an instrument's configuration and
// invalidates cache
func (h *Handler) UpdateInstrumentConfiguration(
	name Name,
	config map[PropertyName]map[Index]PortConfiguration,
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
func (h *Handler) SetInstrumentInitialized(
	name Name,
	initialized bool,
) {
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

type SetInstruction struct {
	Property PropertyName
	Name     JsonPort
	Value    any
}

// SetProperty sends a SET command to the appropriate instrument based on the
// provided property and name.
func (h *Handler) SetProperty(req SetInstruction) {
	// Find the instrument and index by searching through all instrument ports
	var targetInstrument Name
	var targetIndex Index
	found := false
	h.mutex.RLock()

	for instrumentName, process := range h.Instruments {
		if !process.Initialized || process.Ports == nil {
			continue
		}

		// Check if this instrument has the requested property
		if propertyData, exists := process.Ports[req.Property]; exists {
			h.Log.Info(
				"Received %s end it exists %v",
				propertyData,
				exists,
			)
			for index, portName := range propertyData {
				if portName == req.Name {
					targetInstrument = instrumentName
					targetIndex = index
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	h.mutex.RUnlock()

	if !found {
		h.Log.Error(
			"Could not find instrument with property %s and name %s",
			req.Property,
			req.Name,
		)
		return
	}
	realIndex, err := strconv.ParseInt(string(targetIndex), 10, 64)
	if err != nil {
		h.Log.Error(
			"Failed to convert index %s to int64: %v",
			targetIndex,
			err,
		)
	}

	// Create and send the SET command to the target instrument
	setCommand := api.Set{
		Property: string(req.Property),
		Index:    realIndex,
		Value:    req.Value,
	}

	setData, err := json.Marshal(setCommand)
	if err != nil {
		h.Log.Error(
			"Failed to marshal %s command: %v", SetCommand, err,
		)
		return
	}

	// Publish the SET command to the target instrument
	setSubject := fmt.Sprintf("%s.%s", SetCommand, targetInstrument)

	if err := h.nc.Publish(setSubject, setData); err != nil {
		h.Log.Error(
			"Failed to publish %s command to %s: %v",
			SetCommand,
			setSubject,
			err,
		)
		return
	}

	h.Log.Info(
		"Successfully sent %s command to %s: property=%s, index=%s, value=%v",
		SetCommand,
		setSubject,
		req.Property,
		targetIndex,
		req.Value,
	)
}

// SetProperties sets multiple properties on an instrument in order
func (h *Handler) SetProperties(si []SetInstruction) {
	for _, instruction := range si {
		h.SetProperty(instruction)
	}
}

// LogWrapper provides convenient logging with automatic handler name and
// sprintf formatting
type LogWrapper struct {
	logger      *logging.Logger
	handlerName string
}

// NewLogWrapper creates a new log wrapper for the given handler
func NewLogWrapper(logger *logging.Logger, handlerName string) *LogWrapper {
	return &LogWrapper{
		logger:      logger,
		handlerName: handlerName,
	}
}

// Info logs an info message with sprintf formatting
func (l *LogWrapper) Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Info(l.handlerName, msg)
}

// Warn logs a warning message with sprintf formatting
func (l *LogWrapper) Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Warn(l.handlerName, msg)
}

// Error logs an error message with sprintf formatting
func (l *LogWrapper) Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Error(l.handlerName, msg)
}

// Debug logs a debug message with sprintf formatting
func (l *LogWrapper) Debug(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Debug(l.handlerName, msg)
}
