package ports

import (
	"fmt"
	"strings"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
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

type PortName string

// BuildPortLibrary expands each channel-group IO definition in the parsed
// instrument APIs into one PortEntry per channel.
//
// This is intentionally limited to the current hub API model. It does not
// create entries for top-level or global IO because those are not represented
// by InstrumentAPI yet.
func BuildPortLibrary(apis []InstrumentAPI) (PortLibrary, error) {
	library := make(PortLibrary)

	for _, api := range apis {
		instrumentType, err := instrumentTypeFromAPI(api.Instrument.InstrumentType)
		if err != nil {
			return nil, fmt.Errorf(
				"instrument %q: %w",
				api.Instrument.Identifier,
				err,
			)
		}

		for _, group := range api.ChannelGroups {
			if group.Name == "" {
				return nil, fmt.Errorf(
					"instrument %q has a channel group with no name",
					api.Instrument.Identifier,
				)
			}
			if group.ChannelParameter.Min > group.ChannelParameter.Max {
				return nil, fmt.Errorf(
					"instrument %q channel group %q has min channel %d greater than max channel %d",
					api.Instrument.Identifier,
					group.Name,
					group.ChannelParameter.Min,
					group.ChannelParameter.Max,
				)
			}

			for channel := group.ChannelParameter.Min; channel <= group.ChannelParameter.Max; channel++ {
				for _, io := range group.IoTypes {
					role, portAccess, err := portAttributesFromRole(io.Role)
					if err != nil {
						return nil, fmt.Errorf(
							"instrument %q channel group %q IO %q: %w",
							api.Instrument.Identifier,
							group.Name,
							io.Name,
							err,
						)
					}
					if io.Name == "" {
						return nil, fmt.Errorf(
							"instrument %q channel group %q has an IO type with no name",
							api.Instrument.Identifier,
							group.Name,
						)
					}

					name := PortName(fmt.Sprintf(
						"%s.%s.%d.%s",
						api.Instrument.Identifier,
						group.Name,
						channel,
						io.Name,
					))
					if _, exists := library[name]; exists {
						return nil, fmt.Errorf("duplicate port definition %q", name)
					}

					library[name] = PortEntry{
						InstrumentName: api.Instrument.Identifier,
						ChannelGroup:   group.Name,
						Channel:        channel,
						InstrumentType: instrumentType,
						Role:           role,
						Access:         portAccess,
						Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
						Unit:           io.Unit,
						Description:    io.Description,
					}
				}
			}
		}
	}

	return library, nil
}

func instrumentTypeFromAPI(name string) (instrument.Instrument, error) {
	types := map[string]instrument.Instrument{
		"dc_voltage_source": instrument.DcVoltageSource,
		"amnmeter":          instrument.Amnmeter,
		"magnet":            instrument.Magnet,
		"lockin":            instrument.Lockin,
		"voltage_source":    instrument.VoltageSource,
		"current_source":    instrument.CurrentSource,
		"hf_voltage_source": instrument.HfVoltageSource,
		"dc_current_source": instrument.DcCurrentSource,
		"hf_current_source": instrument.HfCurrentSource,
		"thermometer":       instrument.Thermometer,
		"voltmeter":         instrument.Voltmeter,
		"fpga":              instrument.Fpga,
		"clock":             instrument.Clock,
		"discrete":          instrument.Discrete,
	}

	value, ok := types[name]
	if !ok {
		return 0, fmt.Errorf("unsupported instrument type %q", name)
	}
	return value, nil
}

func portAttributesFromRole(role string) (porttype.PortType, access.Access, error) {
	switch role {
	case "output":
		return porttype.PortTypeKnob, access.Write, nil
	case "input":
		return porttype.PortTypeMeter, access.Read, nil
	case "setting":
		return porttype.PortTypeSetting, access.Readwrite, nil
	default:
		return 0, 0, fmt.Errorf("unsupported IO role %q", role)
	}
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
func ConnectWireMap(wiremap *config.WireMap, lib PortLibrary) ([]ConnectedPort, error) {
	var connected []ConnectedPort
	var errs []string

	for _, wEntry := range wiremap.Contents {
		instrumentName := wEntry.Instrument.Name
		channelName := wEntry.Instrument.ChannelGroup
		channel := wEntry.Instrument.Channel
		handle := wEntry.Gate

		// Find all port library entries matching this instrument + channel.
		for portName, entry := range lib {
			if entry.InstrumentName == instrumentName && entry.ChannelGroup == channelName && channel == entry.Channel {
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
		return connected, fmt.Errorf("wiremap connection errors: %s", strings.Join(errs, "; "))
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
func NewConnectedPortsFromConnections(ports []ConnectedPort) *ConnectedPorts {
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

// ResolveConnectedPort resolves a logical device name plus IO/capability name
// to exactly one connected instrument port.
func (h ConnectedPorts) ResolveConnectedPort(port *instrumentport.Handle) (ConnectedPort, error) {
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
		if cp.DeviceName == defaultName && cp.InstrumentName == instrumentName && cp.InstrumentType == instrumentType && cp.Role == role && access == cp.Access && characteristic == cp.Characteristic {
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
