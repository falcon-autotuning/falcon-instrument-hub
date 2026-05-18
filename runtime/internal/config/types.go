package config

import "strings"

// DeviceConfig represents the complete device configuration
type DeviceConfig struct {
	// Region 1: Global name categorization
	ScreeningGates    string `yaml:"ScreeningGates"`
	PlungerGates      string `yaml:"PlungerGates"`
	Ohmics            string `yaml:"Ohmics"`
	BarrierGates      string `yaml:"BarrierGates"`
	ReservoirGates    string `yaml:"ReservoirGates"`
	NumUniqueChannels int    `yaml:"num-unique-channels"`

	// Region 2: Specific channel registration
	Groups map[string]Group `yaml:"groups"`

	// Region 3: DC wiring
	WiringDC map[InstrumentConnection]WiringSpec `yaml:"wiringDC"`
}

type Group struct {
	Name           string `yaml:"Name"`
	NumDots        int    `yaml:"NumDots"`
	ScreeningGates string `yaml:"ScreeningGates"`
	ReservoirGates string `yaml:"ReservoirGates"`
	PlungerGates   string `yaml:"PlungerGates"`
	BarrierGates   string `yaml:"BarrierGates"`
	Order          string `yaml:"Order"`
}

type WiringSpec struct {
	Resistance  float64 `yaml:"resistance"`
	Capacitance float64 `yaml:"capacitance"`
}

// WireMap represents the wire mapping configuration
type WireMap map[InstrumentConnection]InstrumentConnection

type InstrumentConnection string

func (ic InstrumentConnection) String() string {
	return string(ic)
}

func (ic InstrumentConnection) Contains(other string) bool {
	// Check if the connection contains the other string
	return strings.Contains(ic.String(), other)
}

// Config holds all configuration data and file paths
type Config struct {
	DeviceConfigPath    string
	WiremapPath         string
	DeviceConfig        *DeviceConfig
	WireMap             *WireMap
	// InstrumentAPIPaths is the list of paths to instrument API YAML files.
	InstrumentAPIPaths []string
}
