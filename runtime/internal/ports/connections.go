package ports

import (
	"fmt"
	"strings"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
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

// This is a PortEntry merged with the contents of the WireMap for falcon indexing
type ConnectedPort struct {
	// PortName is the fully qualified port name, e.g. "Mock.Source1.analog.voltage".
	PortName       PortName
	DeviceName     string
	InstrumentName string
	ChannelName    string
	ChannelIndex   int
	InstrumentType instrument.Instrument
	Role           porttype.PortType
	Access         access.Access
	Characteristic instrumentcharacteristic.InstrumentCharacteristic
	Unit           string
	Description    string
	Handle         *connection.Handle
}

// IsKnob reports whether this connected port is an output (knob).
func (c ConnectedPort) IsKnob() bool { return c.Role == porttype.PortTypeKnob }

// IsMeter reports whether this connected port is an input (meter).
func (c ConnectedPort) IsMeter() bool { return c.Role == porttype.PortTypeMeter }

// IsMeter reports whether this connected port is an input (meter).
func (c ConnectedPort) IsSetting() bool { return c.Role == porttype.PortTypeSetting }

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
					PortName:       portName,
					DeviceName:     wEntry.PhysicalDeviceName,
					InstrumentName: instrumentName,
					ChannelName:    channelName,
					ChannelIndex:   channel,
					InstrumentType: entry.InstrumentType,
					Role:           entry.Role,
					Unit:           entry.Unit,
					Description:    entry.Description,
					Handle:         handle,
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

func newConnectedPorts(ports []ConnectedPort) *ConnectedPorts {
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
