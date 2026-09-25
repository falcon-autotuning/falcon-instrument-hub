package ports

import (
	"fmt"
	"strings"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
)

// ConnectedPort represents a port type instance wired to a physical device gate.
// It is produced by ConnectWireMap and is the primary type used at runtime.
type ConnectedPort struct {
	// PortName is the fully qualified port name, e.g. "Mock.Source1.analog.voltage".
	PortName PortName
	// DeviceName is the physical gate name from the wiremap, e.g. "P1".
	DeviceName string
	// InstrumentName is the ISS instrument identifier, e.g. "Source1".
	InstrumentName string
	// ChannelName is the channel group name, e.g. "analog".
	ChannelName string
	// ChannelIndex is the 1-based channel index from the wiremap entry.
	ChannelIndex int
	// IoTypeName is the channel IO/capability name, e.g. "voltage" or "sample_rate".
	IoTypeName string
	// InstrumentType is the canonical falcon-core instrument type string.
	InstrumentType string
	// Role mirrors PortEntry.Role: "input", "output", or "setting".
	Role string
	// Unit is the physical unit string, e.g. "V".
	Unit string
	// Description is a human-readable description of the io type.
	Description string
	// The falcon handle for the DeviceName
	Handle *connection.Handle
}

// IsKnob reports whether this connected port is an output (knob).
func (c ConnectedPort) IsKnob() bool { return c.Role == "output" }

// IsMeter reports whether this connected port is an input (meter).
func (c ConnectedPort) IsMeter() bool { return c.Role == "input" }

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
			if entry.Identifier == instrumentName && entry.ChannelName == channelName {
				connected = append(connected, ConnectedPort{
					PortName:       portName,
					DeviceName:     wEntry.PhysicalDeviceName,
					InstrumentName: instrumentName,
					ChannelName:    channelName,
					ChannelIndex:   channel,
					IoTypeName:     entry.IoTypeName,
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
		}
	}

	return out
}

func NewConnectedPorts(instrumentAPIPaths []string, wiremap *config.WireMap) (*ConnectedPorts, error) {
	if wiremap == nil {
		return nil, fmt.Errorf("no wiremap provided; port connections will be empty")
	}
	apis, err := ParseInstrumentAPIs(instrumentAPIPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to load instrument APIs: %w", err)
	}
	lib := BuildPortLibrary(apis)
	connected, err := ConnectWireMap(wiremap, lib)
	if err != nil {
		return nil, fmt.Errorf("wiremap connection warnings: %v", err)
	}
	return newConnectedPorts(connected), nil
}

// ResolveConnectedPort resolves a logical device name plus IO/capability name
// to exactly one connected instrument port.
func (h ConnectedPorts) ResolveConnectedPort(deviceName, ioTypeName, role string) (ConnectedPort, error) {
	var matches []ConnectedPort
	for _, cp := range h.AllConnections {
		if cp.DeviceName == deviceName && cp.IoTypeName == ioTypeName && cp.Role == role {
			matches = append(matches, cp)
		}
	}

	switch len(matches) {
	case 0:
		return ConnectedPort{}, fmt.Errorf(
			"no connected port for device_name=%q io_type_name=%q role=%q",
			deviceName,
			ioTypeName,
			role,
		)
	case 1:
		return matches[0], nil
	default:
		return ConnectedPort{}, fmt.Errorf(
			"ambiguous connected port for device_name=%q io_type_name=%q role=%q: %d matches",
			deviceName,
			ioTypeName,
			role,
			len(matches),
		)
	}
}
