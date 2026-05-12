// Package serverinterpreter provides quantum dot device configuration types.
//
// This file defines types for parsing and working with quantum dot device
// configurations compatible with instrument-script-server schemas.
package serverinterpreter

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// QuantumDotDeviceConfig represents a complete quantum dot device configuration.
// Compatible with instrument-script-server/schemas/quantum_dot_device.schema.json
type QuantumDotDeviceConfig struct {
	Global   GlobalConfig          `yaml:"global" json:"global"`
	Groups   map[string]DotGroup   `yaml:"groups" json:"groups"`
	WiringDC map[string]WiringInfo `yaml:"wiringDC" json:"wiringDC"`
}

// GlobalConfig contains device-wide configuration.
type GlobalConfig struct {
	ScreeningGates    string `yaml:"ScreeningGates" json:"ScreeningGates"`
	PlungerGates      string `yaml:"PlungerGates" json:"PlungerGates"`
	Ohmics            string `yaml:"Ohmics" json:"Ohmics"`
	BarrierGates      string `yaml:"BarrierGates" json:"BarrierGates"`
	ReservoirGates    string `yaml:"ReservoirGates" json:"ReservoirGates"`
	NumUniqueChannels int    `yaml:"num-unique-channels" json:"num-unique-channels"`
}

// DotGroup represents a group of quantum dots with associated gates.
type DotGroup struct {
	Name           string `yaml:"Name" json:"Name"`
	NumDots        int    `yaml:"NumDots" json:"NumDots"`
	ScreeningGates string `yaml:"ScreeningGates" json:"ScreeningGates"`
	ReservoirGates string `yaml:"ReservoirGates" json:"ReservoirGates"`
	PlungerGates   string `yaml:"PlungerGates" json:"PlungerGates"`
	BarrierGates   string `yaml:"BarrierGates" json:"BarrierGates"`
	Order          string `yaml:"Order" json:"Order"`
}

// WiringInfo contains DC wiring parameters for a gate.
type WiringInfo struct {
	Resistance  float64 `yaml:"resistance" json:"resistance"`
	Capacitance float64 `yaml:"capacitance" json:"capacitance"`
}

// LoadQuantumDotDeviceConfig loads a quantum dot device configuration from a YAML file.
func LoadQuantumDotDeviceConfig(path string) (*QuantumDotDeviceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read device config: %w", err)
	}

	var config QuantumDotDeviceConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse device config: %w", err)
	}

	return &config, nil
}

// ParseQuantumDotDeviceConfig parses a quantum dot device configuration from YAML bytes.
func ParseQuantumDotDeviceConfig(data []byte) (*QuantumDotDeviceConfig, error) {
	var config QuantumDotDeviceConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse device config: %w", err)
	}
	return &config, nil
}

// AllPlungerGates returns a list of all plunger gate names.
func (c *QuantumDotDeviceConfig) AllPlungerGates() []string {
	return splitGates(c.Global.PlungerGates)
}

// AllBarrierGates returns a list of all barrier gate names.
func (c *QuantumDotDeviceConfig) AllBarrierGates() []string {
	return splitGates(c.Global.BarrierGates)
}

// AllScreeningGates returns a list of all screening gate names.
func (c *QuantumDotDeviceConfig) AllScreeningGates() []string {
	return splitGates(c.Global.ScreeningGates)
}

// AllOhmics returns a list of all ohmic contact names.
func (c *QuantumDotDeviceConfig) AllOhmics() []string {
	return splitGates(c.Global.Ohmics)
}

// AllReservoirGates returns a list of all reservoir gate names.
func (c *QuantumDotDeviceConfig) AllReservoirGates() []string {
	return splitGates(c.Global.ReservoirGates)
}

// AllGates returns a list of all gate names (plungers, barriers, screening, reservoir).
func (c *QuantumDotDeviceConfig) AllGates() []string {
	var gates []string
	gates = append(gates, c.AllPlungerGates()...)
	gates = append(gates, c.AllBarrierGates()...)
	gates = append(gates, c.AllScreeningGates()...)
	gates = append(gates, c.AllReservoirGates()...)
	return gates
}

// GetGroup returns a specific dot group by name.
func (c *QuantumDotDeviceConfig) GetGroup(name string) (*DotGroup, bool) {
	for _, g := range c.Groups {
		if g.Name == name {
			return &g, true
		}
	}
	return nil, false
}

// GetWiring returns wiring info for a specific gate.
func (c *QuantumDotDeviceConfig) GetWiring(gate string) (*WiringInfo, bool) {
	wiring, ok := c.WiringDC[gate]
	if !ok {
		return nil, false
	}
	return &wiring, true
}

// PlungerGates returns plunger gates for a specific group.
func (g *DotGroup) PlungerGateList() []string {
	return splitGates(g.PlungerGates)
}

// BarrierGates returns barrier gates for a specific group.
func (g *DotGroup) BarrierGateList() []string {
	return splitGates(g.BarrierGates)
}

// OrderList returns the gate order as a slice.
func (g *DotGroup) OrderList() []string {
	return splitGates(g.Order)
}

// splitGates splits a semicolon-separated gate list into individual names.
func splitGates(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// GateChannelMapping maps gate names to DAC instrument/channel pairs.
type GateChannelMapping struct {
	Gates map[string]InstrumentTarget
}

// NewGateChannelMapping creates a mapping with default QDAC channels.
// Gates are assigned channels sequentially: P1->1, P2->2, B1->3, etc.
func NewGateChannelMappingFromConfig(config *QuantumDotDeviceConfig, dacID string) *GateChannelMapping {
	mapping := &GateChannelMapping{
		Gates: make(map[string]InstrumentTarget),
	}

	channel := 1
	for _, gate := range config.AllGates() {
		mapping.Gates[gate] = InstrumentTarget{
			Id:      dacID,
			Channel: channel,
		}
		channel++
	}

	return mapping
}

// Get returns the instrument target for a gate name.
func (m *GateChannelMapping) Get(gate string) (InstrumentTarget, bool) {
	target, ok := m.Gates[gate]
	return target, ok
}

// Set assigns an instrument target to a gate name.
func (m *GateChannelMapping) Set(gate string, target InstrumentTarget) {
	m.Gates[gate] = target
}

// MeasurementChannel represents a measurement instrument channel.
type MeasurementChannel struct {
	Name       string           // User-friendly name (e.g., "I_O1")
	Instrument InstrumentTarget // DMM or current meter target
}

// QuantumDotMeasurementSetup combines device config with instrument mappings.
type QuantumDotMeasurementSetup struct {
	Device              *QuantumDotDeviceConfig
	GateMapping         *GateChannelMapping
	MeasurementChannels []MeasurementChannel
}

// NewQuantumDotMeasurementSetup creates a measurement setup from device config.
func NewQuantumDotMeasurementSetup(
	config *QuantumDotDeviceConfig,
	dacID string,
	dmmID string,
) *QuantumDotMeasurementSetup {
	setup := &QuantumDotMeasurementSetup{
		Device:              config,
		GateMapping:         NewGateChannelMappingFromConfig(config, dacID),
		MeasurementChannels: make([]MeasurementChannel, 0),
	}

	// Add measurement channels for each group
	channel := 0
	for _, group := range config.Groups {
		setup.MeasurementChannels = append(setup.MeasurementChannels, MeasurementChannel{
			Name: group.Name,
			Instrument: InstrumentTarget{
				Id:      dmmID,
				Channel: channel,
			},
		})
		channel++
	}

	return setup
}

// BuildSetVoltageRequests builds set voltage requests for the specified gates.
func (s *QuantumDotMeasurementSetup) BuildSetVoltageRequests(
	voltages map[string]float64,
) []SetVoltageRequest {
	var requests []SetVoltageRequest

	for gate, voltage := range voltages {
		if target, ok := s.GateMapping.Get(gate); ok {
			requests = append(requests, SetVoltageRequest{
				Setter:     target,
				SetVoltage: voltage,
			})
		}
	}

	return requests
}

// BuildGetVoltageRequests builds get voltage requests for all measurement channels.
func (s *QuantumDotMeasurementSetup) BuildGetVoltageRequests() []GetVoltageRequest {
	var requests []GetVoltageRequest

	for _, ch := range s.MeasurementChannels {
		requests = append(requests, GetVoltageRequest{
			Getter: ch.Instrument,
		})
	}

	return requests
}

// Build1DSweepData builds sweep data for a 1D voltage sweep on a specific gate.
func (s *QuantumDotMeasurementSetup) Build1DSweepData(
	sweepGate string,
	startVoltage, stopVoltage float64,
	numPoints int,
	staticVoltages map[string]float64,
	settlingTimeMs float64,
) (*Sweep1DScriptData, error) {
	sweepTarget, ok := s.GateMapping.Get(sweepGate)
	if !ok {
		return nil, fmt.Errorf("unknown sweep gate: %s", sweepGate)
	}

	// Build static setters (exclude sweep gate)
	var staticSetters []SetVoltageRequest
	for gate, voltage := range staticVoltages {
		if gate == sweepGate {
			continue
		}
		if target, ok := s.GateMapping.Get(gate); ok {
			staticSetters = append(staticSetters, SetVoltageRequest{
				Setter:     target,
				SetVoltage: voltage,
			})
		}
	}

	return &Sweep1DScriptData{
		MeasurementName:    fmt.Sprintf("sweep_%s", sweepGate),
		SweepGate:          sweepGate,
		SweepSetter:        sweepTarget,
		StartVoltage:       startVoltage,
		StopVoltage:        stopVoltage,
		NumPoints:          numPoints,
		SettlingTimeMs:     settlingTimeMs,
		StaticSetters:      staticSetters,
		GetVoltageRequests: s.BuildGetVoltageRequests(),
	}, nil
}
