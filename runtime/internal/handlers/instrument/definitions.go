package instrument

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

const (
	Master                     PropertyName   = "master"
	HandlerName                string         = "INSTRUMENT_HANDLER"
	Knob                       port           = "Knob"
	Meter                      port           = "Meter"
	Port                       port           = "InstrumentPort"
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

// PortConfiguration represents the inverted mapping for a port
type PortOptions struct {
	Instrument Name           `json:"instrument"`
	Properties []PropertyName `json:"properties"`
	Index      Index          `json:"index"`
}

// GetPropertyIndices converts PortOptions to a slice of PropertyIndex structs
// Uses the port's index for all properties
func (po *PortOptions) GetPropertyIndices() []PropertyIndex {
	indices := make([]PropertyIndex, len(po.Properties))
	for i, property := range po.Properties {
		indices[i] = PropertyIndex{Property: property, Index: po.Index}
	}
	return indices
}

// GetFirstPropertyIndex returns a PropertyIndex for the first property in
// PortOptions
// Returns nil if no properties exist
func (po *PortOptions) GetFirstPropertyIndex() *PropertyIndex {
	if len(po.Properties) == 0 {
		return nil
	}
	return &PropertyIndex{Property: po.Properties[0], Index: po.Index}
}

type PropertyIndex struct {
	Property PropertyName `json:"property"`
	Index    Index        `json:"index"`
}

// InstrumentProcess represents a registered instrument
type InstrumentProcess struct {
	Name          Name
	Ports         propertyIndexedPorts
	Configuration map[PropertyName]map[Index]PortConfiguration
	Initialized   bool
}

// Handler handles instrument registration and port management
type Handler struct {
	logger        *logging.Logger
	Log           *LogWrapper
	natsURL       string
	nc            *nats.Conn
	Instruments   map[Name]*InstrumentProcess
	mutex         sync.RWMutex
	subscriptions []*nats.Subscription
	portProcessor *PortProcessor
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
	h.Log.Debug(
		"Collecting port properties from %d instruments",
		len(h.Instruments),
	)
	h.mutex.RLock()
	if h.portProcessor != nil {
		knobs, meters := h.portProcessor.CollectPortProperties(h.Instruments)
		h.mutex.RUnlock()
		h.Log.Debug(
			"Collected %d knobs and %d meters",
			len(knobs),
			len(meters),
		)
		return knobs, meters
	}
	h.mutex.RUnlock()
	return nil, nil
}

// GetPortOptions collects the port options from a selected name
func (h *Handler) GetPortOptions(name JsonPort) (PortOptions, error) {
	return h.portProcessor.getCachedOptions(name)
}

// BuildConfigurations creates the configuration mapping by collecting and
// inverting port mappings
func (h *Handler) BuildConfigurations() (map[JsonPort]map[PropertyName]PortConfiguration, error) {
	h.Log.Debug(
		"Building configuration of port properties from %d instruments",
		len(h.Instruments),
	)
	h.mutex.RLock()
	if h.portProcessor != nil {

		config, err := h.portProcessor.BuildConfigurations(h.Instruments)
		h.mutex.RUnlock()
		h.Log.Debug(
			"Port configuration found with %d configurations",
			len(config),
		)
		return config, err
	}

	// Return empty map if no port processor available
	h.mutex.RUnlock()
	return make(map[JsonPort]map[PropertyName]PortConfiguration), nil
}

// BuildPortConfigurations builds the port configurations mapping
// Returns a mapping from port names to their configuration details
func (h *Handler) BuildPortConfigurations() (map[JsonPort]PortOptions, error) {
	h.Log.Debug(
		"Building configuration of port properties from %d instruments",
		len(h.Instruments),
	)
	h.mutex.RLock()
	if h.portProcessor != nil {
		config, err := h.portProcessor.BuildPortConfigurations(h.Instruments)
		h.mutex.RUnlock()
		h.Log.Debug(
			"Port configuration found with %d configurations",
			len(config),
		)
		return config, err
	}

	// Return empty map if no port processor available
	h.mutex.RUnlock()
	return make(map[JsonPort]PortOptions), nil
}

// GetMultiplePortOptions finds configurations for multiple ports efficiently
func (h *Handler) GetMultiplePortOptions(
	names []JsonPort,
) (map[JsonPort]PortOptions, error) {
	h.Log.Debug("Attempting to find port options for %d ports", len(names))

	if len(names) == 0 {
		return make(map[JsonPort]PortOptions), nil
	}

	h.mutex.RLock()
	defer h.mutex.RUnlock()

	results := make(map[JsonPort]PortOptions, len(names))
	var errors []string

	for _, name := range names {
		var data PortObject
		// need to get compact json form:
		if err := json.Unmarshal([]byte(name), &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JsonPort: %v", err)
		}
		compactJSON, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal JsonPort: %v", err)
		}
		compactPortName := JsonPort(compactJSON)
		if portOptions, err := h.portProcessor.getCachedOptions(compactPortName); err != nil {
			errors = append(errors, fmt.Sprintf("port %s: %v", name, err))
		} else {
			results[name] = portOptions
		}
	}

	if len(errors) > 0 {
		return results, fmt.Errorf(
			"failed to find some ports: %s",
			strings.Join(errors, "; "),
		)
	}

	return results, nil
}

// InvalidatePortConfigCache invalidates the cached port configurations
// This should be called when instruments are added, removed, or reconfigured
func (h *Handler) InvalidatePortConfigCache() {
	h.Log.Debug("Invalidating the port configuration cache")
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
	h.Log.Debug("Adding an instrument: %s", name)
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
	h.Log.Debug("Removing an instrument: %s", name)
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
	h.Log.Debug("Updating configuration for the instrument %s", name)
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
	h.Log.Debug("Instrument %s initialized %v", name, initialized)
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

func (h *Handler) IsInstrumentMaster(instrumentName Name) (bool, error) {
	h.Log.Debug("Checking if instrument %s is master", instrumentName)
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	instrument, exists := h.Instruments[instrumentName]
	if !exists {
		return false, fmt.Errorf("instrument %s not found", instrumentName)
	}

	// Check if the instrument is initialized and has a master port
	if !instrument.Initialized || instrument.Ports == nil {
		return false, nil
	}

	// Check if any port is marked as master
	for propertyName := range instrument.Configuration {
		if propertyName == Master {
			return true, nil
		}
	}

	return false, nil
}

func (h *Handler) FindMasterInstrument(instruments []Name) (Name, error) {
	h.Log.Debug("Trying to find the master instruments in %+v", instruments)
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for _, instrumentName := range instruments {
		instrument, exists := h.Instruments[instrumentName]
		if !exists {
			continue
		}

		if instrument.Initialized && instrument.Ports != nil {
			if _, isMaster := instrument.Ports[Master]; isMaster {
				return instrumentName, nil
			}
		}
	}

	return "", fmt.Errorf("no master instrument found in the provided list")
}

type (
	ID             int64
	SetInstruction struct {
		Property PropertyName
		Name     JsonPort
		Value    any
	}
	DirectSetInstruction struct {
		InstrumentName Name
		Property       PropertyName
		Index          int64
		Value          any
	}
	MeasurementID struct {
		ProcessId ID
		ChunkId   ID
	}
)

// SetProperty sends a SET command to the appropriate instrument based on the
// provided property and name.
func (h *Handler) SetProperty(req SetInstruction, measurementID MeasurementID) {
	// default value for processId is 0, which means it is not set
	if measurementID.ProcessId == 0 {
		measurementID.ProcessId = -1
	}

	h.Log.Debug("Preparing to SET an instruction")
	options, err := h.portProcessor.getCachedOptions(req.Name)
	if err != nil {
		h.Log.Error(
			"error collecting cached options for port %s: %v",
			req.Name,
			err,
		)
		return
	}
	targetInstrument := options.Instrument
	targetIndex := options.Index

	realIndex, err := strconv.ParseInt(string(targetIndex), 10, 64)
	if err != nil {
		h.Log.Error(
			"Failed to convert index %s to int64: %v",
			targetIndex,
			err,
		)
		return
	}

	// Create DirectSetInstruction and send it
	directInstruction := DirectSetInstruction{
		InstrumentName: targetInstrument,
		Property:       req.Property,
		Index:          realIndex,
		Value:          req.Value,
	}

	h.SendDirectSetInstruction(directInstruction, measurementID)
}

// SendDirectSetInstruction sends a SET command directly to a specific
// instrument
func (h *Handler) SendDirectSetInstruction(
	req DirectSetInstruction,
	measurementID MeasurementID,
) {
	// default value for processId is 0, which means it is not set
	if measurementID.ProcessId == 0 {
		measurementID.ProcessId = -1
	}

	// Create and send the SET command to the target instrument
	setCommand := api.Set{
		Property:  string(req.Property),
		Index:     req.Index,
		Value:     req.Value,
		ProcessId: int64(measurementID.ProcessId),
		ChunkId:   int64(measurementID.ChunkId),
	}
	h.Log.Debug(
		"The processId in the set command is %d for property %s at index %d",
		setCommand.ProcessId,
		setCommand.Property,
		setCommand.Index,
	)

	setData, err := json.Marshal(setCommand)
	if err != nil {
		h.Log.Error(
			"Failed to marshal %s command: %v", SetCommand, err,
		)
		return
	}

	// Publish the SET command to the target instrument
	setSubject := fmt.Sprintf("%s.%s", SetCommand, req.InstrumentName)

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
		"Successfully sent %s command to %s: property=%s, index=%d, value=%v",
		SetCommand,
		setSubject,
		req.Property,
		req.Index,
		req.Value,
	)
}

// SetProperties sets multiple properties on an instrument in order, ensuring
// ARM is last
func (h *Handler) SetProperties(
	seti []SetInstruction,
	armi []SetInstruction,
	measurementID MeasurementID,
) {
	// Send regular instructions first
	for _, instruction := range seti {
		h.SetProperty(instruction, measurementID)
	}

	// Send ARM instructions last
	for _, instruction := range armi {
		h.SetProperty(instruction, measurementID)
	}
}

// SetPropertyWithDefaults sets a property with default MeasurementID (-1, 0)
func (h *Handler) SetPropertyWithDefaults(req SetInstruction) {
	defaultMeasurementID := MeasurementID{
		ProcessId: -1,
		ChunkId:   0,
	}
	h.SetProperty(req, defaultMeasurementID)
}

// SetPropertiesWithDefaults sets multiple properties with default MeasurementID
// (-1, 0)
func (h *Handler) SetPropertiesWithDefaults(seti, armi []SetInstruction) {
	defaultMeasurementID := MeasurementID{
		ProcessId: -1,
		ChunkId:   0,
	}
	h.SetProperties(seti, armi, defaultMeasurementID)
}

// FindSetByInstrumentPropertyIndex finds a port using instrument name,
// property, and index
func (h *Handler) FindPortByInstrumentPropertyIndex(
	name Name,
	property PropertyName,
	index Index,
) (JsonPort, error) {
	// Get the instrument directly
	h.Log.Debug(
		"Attempting to find an instrument port with the name of %s and the property %s and the index %s",
		name,
		property,
		index,
	)
	h.mutex.RLock()

	instrumentProcess, exists := h.Instruments[name]
	if !exists {
		h.mutex.RUnlock()
		return "", fmt.Errorf("instrument %s not found", name)
	}

	if !instrumentProcess.Initialized || instrumentProcess.Ports == nil {
		h.mutex.RUnlock()
		return "", fmt.Errorf("instrument %s not initialized", name)
	}

	// Look up the port directly: instrument -> property -> index -> port
	if propertyPorts, exists := instrumentProcess.Ports[property]; exists {
		if port, exists := propertyPorts[index]; exists {
			h.mutex.RUnlock()
			h.Log.Debug("Found port: %s", port)
			return port, nil
		}
	}
	defer h.mutex.RUnlock()
	return "", fmt.Errorf(
		"no port found for instrument %s, property %s, index %s",
		name,
		property,
		index,
	)
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
func (l *LogWrapper) Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Info(l.handlerName, msg)
}

// Warn logs a warning message with sprintf formatting
func (l *LogWrapper) Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Warn(l.handlerName, msg)
}

// Error logs an error message with sprintf formatting
func (l *LogWrapper) Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Error(l.handlerName, msg)
}

// Debug logs a debug message with sprintf formatting
func (l *LogWrapper) Debug(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Debug(l.handlerName, msg)
}
