package ports

import (
	"fmt"
	"strings"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumenttypes"
)

// PortName is a dot-separated identifier: "{vendor}.{identifier}.{channel_name}.{io_type_name}"
// e.g. "Mock.Source1.analog.voltage"
type PortName string

// PortEntry describes a single port type defined in an instrument API.
type PortEntry struct {
	Vendor      string
	Identifier  string
	ChannelName string
	IoTypeName  string
	// InstrumentType is the canonical falcon-core instrument type string.
	InstrumentType string
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
					Vendor:         api.Instrument.Vendor,
					Identifier:     api.Instrument.Identifier,
					ChannelName:    cg.Name,
					IoTypeName:     io.Name,
					InstrumentType: inferInstrumentType(api, io),
					Role:           io.Role,
					Unit:           io.Unit,
					Description:    io.Description,
				}
			}
		}
	}
	return lib
}

func inferInstrumentType(api InstrumentAPI, io IoType) string {
	role := strings.ToLower(strings.TrimSpace(io.Role))
	unit := strings.ToLower(strings.TrimSpace(io.Unit))
	protocolType := normalizeInstrumentTypeToken(api.Protocol.Type)
	ioName := normalizeInstrumentTypeToken(io.Name)

	switch role {
	case "output":
		switch {
		case strings.Contains(protocolType, "magnet"):
			return instrumenttypes.Magnet()
		case strings.Contains(protocolType, "currentsource"):
			return instrumenttypes.DCCurrentSource()
		case strings.Contains(protocolType, "voltagesource"):
			return instrumenttypes.DCVoltageSource()
		case isCurrentUnit(unit):
			return instrumenttypes.DCCurrentSource()
		case isVoltageUnit(unit):
			return instrumenttypes.DCVoltageSource()
		default:
			return instrumenttypes.Discrete()
		}

	case "input":
		switch {
		case strings.Contains(protocolType, "lockin"):
			return instrumenttypes.Lockin()
		case strings.Contains(protocolType, "thermometer"):
			return instrumenttypes.Thermometer()
		case strings.Contains(protocolType, "fpga"):
			return instrumenttypes.FPGA()
		case strings.Contains(protocolType, "magnet"):
			return instrumenttypes.Magnet()
		case strings.Contains(protocolType, "multimeter"):
			if isCurrentUnit(unit) || strings.Contains(ioName, "current") {
				return instrumenttypes.Amnmeter()
			}
			return instrumenttypes.Voltmeter()
		case isCurrentUnit(unit) || strings.Contains(ioName, "current"):
			return instrumenttypes.Amnmeter()
		case isVoltageUnit(unit) || strings.Contains(ioName, "voltage") || strings.Contains(ioName, "stream"):
			return instrumenttypes.Voltmeter()
		default:
			return instrumenttypes.Discrete()
		}

	default:
		switch {
		case strings.Contains(protocolType, "currentsource"):
			return instrumenttypes.DCCurrentSource()
		case strings.Contains(protocolType, "voltagesource"):
			return instrumenttypes.DCVoltageSource()
		case strings.Contains(protocolType, "multimeter"):
			return instrumenttypes.Voltmeter()
		default:
			return instrumenttypes.Discrete()
		}
	}
}

func normalizeInstrumentTypeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("-", "", "_", "", " ", "")
	return replacer.Replace(value)
}

func isVoltageUnit(unit string) bool {
	switch unit {
	case "v", "mv", "uv", "μv", "nv", "pv":
		return true
	default:
		return false
	}
}

func isCurrentUnit(unit string) bool {
	switch unit {
	case "a", "ma", "ua", "μa", "na", "pa":
		return true
	default:
		return false
	}
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
