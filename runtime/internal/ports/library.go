package ports

import "fmt"

// PortName is a dot-separated identifier: "{vendor}.{identifier}.{channel_name}.{io_type_name}"
// e.g. "Mock.Source1.analog.voltage"
type PortName string

// PortEntry describes a single port type defined in an instrument API.
type PortEntry struct {
	Vendor      string
	Identifier  string
	ChannelName string
	IoTypeName  string
	// Role is "input" (meter), "output" (knob), or "setting" (configuration parameter).
	Role        string
	Unit        string
	Description string
}

// IsKnob reports whether this port is an output (controllable by falcon).
func (p PortEntry) IsKnob() bool { return p.Role == "output" }

// IsMeter reports whether this port is an input (measured by falcon).
func (p PortEntry) IsMeter() bool { return p.Role == "input" }

// PortLibrary maps port names to their definitions.
type PortLibrary map[PortName]PortEntry

// BuildPortLibrary constructs a PortLibrary from a set of InstrumentAPI definitions.
// Port names take the form "{vendor}.{identifier}.{channel_name}.{io_type_name}".
func BuildPortLibrary(apis []InstrumentAPI) PortLibrary {
	lib := make(PortLibrary)
	for _, api := range apis {
		for _, cg := range api.ChannelGroups {
			for _, io := range cg.IoTypes {
				name := PortName(fmt.Sprintf(
					"%s.%s.%s.%s",
					api.Instrument.Vendor,
					api.Instrument.Identifier,
					cg.Name,
					io.Name,
				))
				lib[name] = PortEntry{
					Vendor:      api.Instrument.Vendor,
					Identifier:  api.Instrument.Identifier,
					ChannelName: cg.Name,
					IoTypeName:  io.Name,
					Role:        io.Role,
					Unit:        io.Unit,
					Description: io.Description,
				}
			}
		}
	}
	return lib
}

// RouteInfo describes how to route a command to a specific instrument channel.
// It is the output of resolving a PortName + DeviceName through the port
// library and wiremap.
type RouteInfo struct {
	InstrumentName string `json:"instrument_name"`
	ChannelName    string `json:"channel_name"`
	ChannelIndex   int    `json:"channel_index"`
	DeviceName     string `json:"device_name"`
}
