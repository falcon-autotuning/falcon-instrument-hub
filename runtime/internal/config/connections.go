package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
)

// PortEntry describes a single port type defined in an instrument API.
type PortEntry struct {
	InstrumentName string
	ChannelGroup   string
	Channel        int
	InstrumentType instrument.Instrument
	Role           porttype.PortType
	Access         access.Access
	Scope          scope.Scope
	Characteristic instrumentcharacteristic.InstrumentCharacteristic
	Unit           string
	Description    string
}

// IsKnob reports whether this port is an output (controllable by falcon).
func (p PortEntry) IsKnob() bool { return p.Role == porttype.PortTypeKnob }

// IsMeter reports whether this port is an input (measured by falcon).
func (p PortEntry) IsMeter() bool { return p.Role == porttype.PortTypeMeter }

// IsSetting reports whether this port is an input (measured by falcon).
func (p PortEntry) IsSetting() bool { return p.Role == porttype.PortTypeSetting }

// PortLibrary maps port names to their definitions.
type PortLibrary map[PortName]PortEntry

func addIOPort(
	library PortLibrary,
	instrumentName string,
	instrumentType instrument.Instrument,
	portName PortName,
	channelGroup string,
	channel int,
	io IoType,
	context string,
) error {
	if channelGroup != "" {
		// Channel-group IOs use suffixes.
		if io.Suffix == "" {
			return fmt.Errorf(
				"%s has a channel IO type with no suffix",
				context,
			)
		}
	} else {
		// Global IOs use names.
		if io.Name == "" {
			return fmt.Errorf(
				"%s has a global IO type with no name",
				context,
			)
		}
	}

	role, err := ParseIORole(io.Role)
	if err != nil {
		id := io.Name
		if id == "" {
			id = io.Suffix
		}

		return fmt.Errorf("%s IO %q: %w", context, id, err)
	}

	portType, portAccess, err := portAttributesFromRole(role)
	if err != nil {
		id := io.Name
		if id == "" {
			id = io.Suffix
		}

		return fmt.Errorf("%s IO %q: %w", context, id, err)
	}

	return addPort(
		library,
		portName,
		PortEntry{
			InstrumentName: instrumentName,
			ChannelGroup:   channelGroup,
			Channel:        channel,
			InstrumentType: instrumentType,
			Role:           portType,
			Access:         portAccess,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
			Unit:           io.Unit,
			Description:    io.Description,
		},
	)
}

func addPort(
	library PortLibrary,
	name PortName,
	entry PortEntry,
) error {
	if _, exists := library[name]; exists {
		return fmt.Errorf("duplicate port definition %q", name)
	}

	library[name] = entry
	return nil
}

type (
	PortName string
	IORole   string
)

func ParseIORole(s string) (IORole, error) {
	switch IORole(s) {
	case IORoleInput,
		IORoleOutput,
		IORoleInOut,
		IORoleTriggerIn,
		IORoleTriggerOut,
		IORoleClockIn,
		IORoleClockOut,
		IORoleSetting:
		return IORole(s), nil

	default:
		return "", fmt.Errorf("unknown IO role %q", s)
	}
}

const (
	IORoleInput      IORole = "input"
	IORoleOutput     IORole = "output"
	IORoleInOut      IORole = "inout"
	IORoleTriggerIn  IORole = "trigger-in"
	IORoleTriggerOut IORole = "trigger-out"
	IORoleClockIn    IORole = "clock-in"
	IORoleClockOut   IORole = "clock-out"
	IORoleSetting    IORole = "setting"
)

func portAttributesFromRole(role IORole) (
	porttype.PortType,
	access.Access,
	error,
) {
	switch role {
	case IORoleOutput:
		return porttype.PortTypeKnob, access.Write, nil

	case IORoleInput:
		return porttype.PortTypeMeter, access.Read, nil

	case IORoleInOut:
		return porttype.PortTypeKnob, access.Readwrite, nil

	case IORoleTriggerIn:
		return porttype.PortTypeMeter, access.Read, nil

	case IORoleTriggerOut:
		return porttype.PortTypeKnob, access.Write, nil

	case IORoleClockIn:
		return porttype.PortTypeMeter, access.Read, nil

	case IORoleClockOut:
		return porttype.PortTypeKnob, access.Write, nil

	case IORoleSetting:
		return porttype.PortTypeSetting, access.Readwrite, nil

	default:
		return 0, 0, fmt.Errorf("unsupported IO role %q", role)
	}
}

func buildExpandedIONames(
	api *InstrumentAPI,
) map[string]struct{} {
	names := make(map[string]struct{})

	for _, group := range api.ChannelGroups {
		for channel := group.ChannelParameter.Min; channel <= group.ChannelParameter.Max; channel++ {
			for _, io := range group.IoTypes {
				names[fmt.Sprintf(
					"%s%d_%s",
					group.Name,
					channel,
					io.Suffix,
				)] = struct{}{}
			}
		}
	}

	return names
}

func channelParameters(
	api *InstrumentAPI,
) map[string]struct{} {
	result := make(map[string]struct{})

	for _, group := range api.ChannelGroups {
		result[group.ChannelParameter.Name] = struct{}{}
	}

	return result
}

func parameterLocation(
	api *InstrumentAPI,
	name string,
) (
	scope.Scope,
	string,
	error,
) {
	for _, group := range api.ChannelGroups {
		if group.ChannelParameter.Name == name {
			return scope.Local, group.Name, nil
		}
	}

	return scope.Global, "", nil
}

func commandLocation(
	api *InstrumentAPI,
	commandName string,
) (
	scope.Scope,
	string,
	error,
) {
	cmd, ok := api.Commands[CommandName(commandName)]
	if !ok {
		return 0, "", fmt.Errorf(
			"command %q not found",
			commandName,
		)
	}

	if cmd.ChannelGroup != "" {
		return scope.Local, cmd.ChannelGroup, nil
	}

	return scope.Global, "", nil
}

func characteristicAccess(
	c InstrumentCharacteristic,
) access.Access {
	hasRead :=
		c.ReadCommand.Command != "" ||
			c.ReadCommand.ParameterName != ""

	hasWrite :=
		c.WriteCommand.Command != "" ||
			c.WriteCommand.ParameterName != ""

	switch {
	case hasRead && hasWrite:
		return access.Readwrite
	case hasRead:
		return access.Read
	case hasWrite:
		return access.Write
	default:
		return access.Readwrite // should never happen after validation
	}
}

func characteristicLocation(
	api *InstrumentAPI,
	c InstrumentCharacteristic,
) (
	scope.Scope,
	string, // channel group
	error,
) {
	if c.ReadCommand.Command != "" {
		return commandLocation(
			api,
			c.ReadCommand.Command,
		)
	}

	if c.WriteCommand.Command != "" {
		return commandLocation(
			api,
			c.WriteCommand.Command,
		)
	}

	if c.ReadCommand.ParameterName != "" {
		return parameterLocation(
			api,
			c.ReadCommand.ParameterName,
		)
	}

	if c.WriteCommand.ParameterName != "" {
		return parameterLocation(
			api,
			c.WriteCommand.ParameterName,
		)
	}

	return 0, "", fmt.Errorf(
		"unable to determine scope",
	)
}

func findChannelGroup(
	api *InstrumentAPI,
	name string,
) (*ChannelGroup, error) {
	for i := range api.ChannelGroups {
		if api.ChannelGroups[i].Name == name {
			return &api.ChannelGroups[i], nil
		}
	}

	return nil, fmt.Errorf(
		"unknown channel group %q",
		name,
	)
}

// BuildPortLibrary expands each channel-group IO definition in the parsed
// instrument APIs into one PortEntry per channel.
func BuildPortLibrary(instruments []InstrumentConfig) (PortLibrary, error) {
	library := make(PortLibrary)

	for _, instrument := range instruments {
		config, err := ParseInstrumentConfig(instrument.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to parse instrument config %q: %w",
				instrument.ConfigPath,
				err,
			)
		}
		api, err := ParseInstrumentAPI(
			filepath.Join(filepath.Dir(instrument.ConfigPath), config.API_ref),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to parse instrument API %q: %w",
				instrument.ConfigPath,
				err,
			)
		}
		expandedNames := buildExpandedIONames(api)
		seenExpandedNames := make(map[string]struct{})

		for _, group := range api.ChannelGroups {
			if group.Name == "" {
				return nil, fmt.Errorf(
					"instrument %q has a channel group with no name",
					config.Name,
				)
			}
			if group.ChannelParameter.Min > group.ChannelParameter.Max {
				return nil, fmt.Errorf(
					"instrument %q channel group %q has min channel %d greater than max channel %d",
					config.Name,
					group.Name,
					group.ChannelParameter.Min,
					group.ChannelParameter.Max,
				)
			}

			for channel := group.ChannelParameter.Min; channel <= group.ChannelParameter.Max; channel++ {
				for _, io := range group.IoTypes {

					name := PortName(fmt.Sprintf(
						"%s.%s.%d.%s",
						config.Name,
						group.Name,
						channel,
						io.Suffix,
					))

					if err := addIOPort(
						library,
						config.Name,
						instrument.InstrumentType,
						name,
						group.Name,
						channel,
						io,
						fmt.Sprintf(
							"instrument %q channel group %q",
							config.Name,
							group.Name,
						),
					); err != nil {
						return nil, err
					}
				}
			}
		}
		for _, io := range api.IO {
			if _, ok := expandedNames[io.Name]; ok {
				seenExpandedNames[io.Name] = struct{}{}
				continue
			}

			// real global
			name := PortName(fmt.Sprintf(
				"%s.%s",
				config.Name,
				io.Name,
			))

			// Skip if a channel-generated port already exists.
			if _, exists := library[name]; exists {
				continue
			}

			if err := addIOPort(
				library,
				config.Name,
				instrument.InstrumentType,
				name,
				"",
				0,
				io,
				fmt.Sprintf("instrument %q global IO", config.Name),
			); err != nil {
				return nil, err
			}
		}
		for name := range expandedNames {
			if _, ok := seenExpandedNames[name]; !ok {
				return nil, fmt.Errorf(
					"instrument %q API expansion mismatch: expected IO %q to exist",
					config.Name,
					name,
				)
			}
		}
		for _, characteristic := range instrument.Characteristics {
			scopeType, channelGroup, err := characteristicLocation(
				api,
				characteristic,
			)
			if err != nil {
				return nil, err
			}

			portAccess := characteristicAccess(
				characteristic,
			)

			if scopeType == scope.Global {
				name := PortName(fmt.Sprintf(
					"%s.characteristic.%s",
					config.Name,
					characteristic.Identifier,
				))

				err = addPort(
					library,
					name,
					PortEntry{
						InstrumentName: config.Name,
						InstrumentType: instrument.InstrumentType,
						Role:           porttype.PortTypeSetting,
						Access:         portAccess,
						Scope:          scope.Global,
						Characteristic: characteristic.Characteristic,
					},
				)
				if err != nil {
					return nil, err
				}

				continue
			}

			group, err := findChannelGroup(
				api,
				channelGroup,
			)
			if err != nil {
				return nil, err
			}

			for channel := group.ChannelParameter.Min; channel <= group.ChannelParameter.Max; channel++ {

				name := PortName(fmt.Sprintf(
					"%s.characteristic.%s.%d.%s",
					config.Name,
					channelGroup,
					channel,
					characteristic.Identifier,
				))

				err = addPort(
					library,
					name,
					PortEntry{
						InstrumentName: config.Name,
						InstrumentType: instrument.InstrumentType,
						ChannelGroup:   channelGroup,
						Channel:        channel,
						Role:           porttype.PortTypeSetting,
						Access:         portAccess,
						Scope:          scope.Local,
						Characteristic: characteristic.Characteristic,
					},
				)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return library, nil
}

// This is a PortEntry merged with the contents of the WireMap for falcon indexing
type ConnectedPort struct {
	PortEntry
	// PortName is the fully qualified port name, e.g. "Mock.Source1.analog.voltage".
	PortName PortName
	// DeviceName is the logical device name, e.g. "P1".
	DeviceName string
	Handle     *connection.Handle
}

// ConnectWireMap resolves wiremap entries against the port library, returning
// a ConnectedPort for each (wiremap entry, io type) pair that matches.
//
// wireMap keys must have the form "InstrumentIdentifier.ChannelName.Index"
// (e.g. "Source1.analog.4"); values are device gate names (e.g. "P1").
//
// For each wiremap entry, every port library entry whose Identifier and
// ChannelName match produces a ConnectedPort with that device gate.
func ConnectWireMap(wiremap WireMap, lib PortLibrary) ([]ConnectedPort, error) {
	var connected []ConnectedPort
	var errs []string

	for _, wEntry := range wiremap {
		instrumentName := wEntry.Instrument.Name
		channelName := wEntry.Instrument.ChannelGroup
		channel := wEntry.Instrument.Channel
		handle := wEntry.Gate

		// Find all port library entries matching this instrument + channel.
		for portName, entry := range lib {
			if entry.InstrumentName == instrumentName &&
				entry.ChannelGroup == channelName &&
				channel == entry.Channel {
				connected = append(connected, ConnectedPort{
					PortEntry:  entry,
					PortName:   portName,
					DeviceName: wEntry.PhysicalDeviceName,
					Handle:     handle,
				})
			}
		}
	}

	if len(errs) > 0 {
		return connected, fmt.Errorf(
			"wiremap connection errors: %s",
			strings.Join(errs, "; "),
		)
	}
	return connected, nil
}

type ConnectedPorts struct {
	AllConnections []ConnectedPort
	Knobs          []ConnectedPort
	Meters         []ConnectedPort
	Settings       []ConnectedPort
}

// NewConnectedPortsFromConnections partitions connected ports by Falcon port
// type while retaining the complete catalog for resolution.
func newConnectedPortsFromConnections(ports []ConnectedPort) *ConnectedPorts {
	out := &ConnectedPorts{
		AllConnections: ports,
	}

	for _, cp := range ports {
		switch {
		case cp.IsKnob():
			out.Knobs = append(out.Knobs, cp)

		case cp.IsMeter():
			out.Meters = append(out.Meters, cp)

		case cp.IsSetting():
			out.Settings = append(out.Settings, cp)
		}
	}

	return out
}

func NewConnectedPorts(
	instruments []InstrumentConfig,
	wiremap WireMap,
) (*ConnectedPorts, error) {
	library, err := BuildPortLibrary(instruments)
	if err != nil {
		return nil, fmt.Errorf("failed to build port library: %w", err)
	}

	connected, err := ConnectWireMap(wiremap, library)
	if err != nil {
		return nil, fmt.Errorf("failed to connect wiremap: %w", err)
	}

	return newConnectedPortsFromConnections(connected), nil
}

// ResolveConnectedPort resolves a logical device name plus IO/capability name
// to exactly one connected instrument port.
func (h ConnectedPorts) ResolveConnectedPort(
	port *instrumentport.Handle,
) (ConnectedPort, error) {
	defaultName, err := port.DefaultName()
	if err != nil {
		return ConnectedPort{}, fmt.Errorf(
			"Cannot collect a default name for port: %w",
			err,
		)
	}
	instrumentName, err := port.InstrumentName()
	if err != nil {
		return ConnectedPort{}, fmt.Errorf(
			"Cannot collect an instrument name for port: %w",
			err,
		)
	}
	instrumentType, err := port.InstrumentType()
	if err != nil {
		return ConnectedPort{}, fmt.Errorf(
			"Cannot collect an instrument type for port: %w",
			err,
		)
	}
	role, err := port.Type()
	if err != nil {
		return ConnectedPort{}, fmt.Errorf(
			"Cannot collect a type for port: %w",
			err,
		)
	}
	access, err := port.Access()
	if err != nil {
		return ConnectedPort{}, fmt.Errorf(
			"Cannot collect an access for port: %w",
			err,
		)
	}
	characteristic, err := port.Characteristic()
	if err != nil {
		return ConnectedPort{}, fmt.Errorf(
			"Cannot collect a characteristic for port: %w",
			err,
		)
	}

	var matches []ConnectedPort
	for _, cp := range h.AllConnections {
		if cp.DeviceName == defaultName &&
			cp.InstrumentName == instrumentName &&
			cp.InstrumentType == instrumentType &&
			cp.Role == role &&
			access == cp.Access &&
			characteristic == cp.Characteristic {
			matches = append(matches, cp)
		}
	}
	if len(matches) == 0 {
		return ConnectedPort{}, fmt.Errorf(
			"No connected port for device_name=%q instrument_name=%q instrument_type=%q role=%d access=%d characteristic=%d",
			defaultName,
			instrumentName,
			instrumentType,
			role,
			access,
			characteristic,
		)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return ConnectedPort{}, fmt.Errorf(
		"Ambiguous connected port for device_name=%q instrument_name=%q instrument_type=%q role=%d access=%d characteristic=%d",
		defaultName,
		instrumentName,
		instrumentType,
		role,
		access,
		characteristic,
	)
}
