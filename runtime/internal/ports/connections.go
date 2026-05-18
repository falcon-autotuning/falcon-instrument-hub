package ports

import (
	"fmt"
	"strconv"
	"strings"
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
	// Role mirrors PortEntry.Role: "input", "output", or "setting".
	Role string
	// Unit is the physical unit string, e.g. "V".
	Unit string
	// Description is a human-readable description of the io type.
	Description string
}

// IsKnob reports whether this connected port is an output (knob).
func (c ConnectedPort) IsKnob() bool { return c.Role == "output" }

// IsMeter reports whether this connected port is an input (meter).
func (c ConnectedPort) IsMeter() bool { return c.Role == "input" }

// RouteInfo returns the routing information for this connected port.
func (c ConnectedPort) RouteInfo() RouteInfo {
	return RouteInfo{
		InstrumentName: c.InstrumentName,
		ChannelName:    c.ChannelName,
		ChannelIndex:   c.ChannelIndex,
		DeviceName:     c.DeviceName,
	}
}

// ConnectWireMap resolves wiremap entries against the port library, returning
// a ConnectedPort for each (wiremap entry, io type) pair that matches.
//
// wireMap keys must have the form "InstrumentIdentifier.ChannelName.Index"
// (e.g. "Source1.analog.4"); values are device gate names (e.g. "P1").
//
// For each wiremap entry, every port library entry whose Identifier and
// ChannelName match produces a ConnectedPort with that device gate.
func ConnectWireMap(wireMap map[string]string, lib PortLibrary) ([]ConnectedPort, error) {
	var connected []ConnectedPort
	var errs []string

	for key, deviceName := range wireMap {
		// Parse "InstrumentIdentifier.ChannelName.Index"
		parts := strings.SplitN(key, ".", 3)
		if len(parts) != 3 {
			errs = append(errs, fmt.Sprintf(
				"invalid wiremap key %q: expected InstrumentIdentifier.ChannelName.Index", key,
			))
			continue
		}

		instrumentName := parts[0]
		channelName := parts[1]
		idx, err := strconv.Atoi(parts[2])
		if err != nil {
			errs = append(errs, fmt.Sprintf(
				"invalid index in wiremap key %q: %v", key, err,
			))
			continue
		}

		// Find all port library entries matching this instrument + channel.
		for portName, entry := range lib {
			if entry.Identifier == instrumentName && entry.ChannelName == channelName {
				connected = append(connected, ConnectedPort{
					PortName:       portName,
					DeviceName:     deviceName,
					InstrumentName: instrumentName,
					ChannelName:    channelName,
					ChannelIndex:   idx,
					Role:           entry.Role,
					Unit:           entry.Unit,
					Description:    entry.Description,
				})
			}
		}
	}

	if len(errs) > 0 {
		return connected, fmt.Errorf("wiremap connection errors: %s", strings.Join(errs, "; "))
	}
	return connected, nil
}
