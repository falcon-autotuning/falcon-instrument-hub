package config

import (
	"fmt"
	"strings"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

type Unit int

const (
	Volt Unit = iota
	Millivolt
	Microvolt
)

func (u Unit) String() string {
	switch u {
	case Volt:
		return "volt"
	case Millivolt:
		return "millivolt"
	case Microvolt:
		return "microvolt"
	default:
		return "unknown"
	}
}

func ParseUnit(s string) (Unit, error) {
	switch strings.ToLower(s) {
	case "volt", "v":
		return Volt, nil
	case "millivolt", "mv":
		return Millivolt, nil
	case "microvolt", "uv", "µv":
		return Microvolt, nil
	default:
		return 0, fmt.Errorf("unknown unit %q", s)
	}
}

type VoltageState struct {
	// The starting voltage value on the device
	Voltage float64 `yaml:"voltage" json:"voltage"`
	// The name of the device connection
	Connection falconcore.ConnectionName `yaml:"name" json:"name"`
	// The unit of the applied voltage (volt, millivolt, microvolt)
	Unit Unit `yaml:"unit" json:"unit"`
}

func (v VoltageState) FindMatchingDeviceName(allDeviceConns []falconcore.Connection) (falconcore.ConnectionName, error) {
	for _, conn := range allDeviceConns {
		if v.Connection == conn.Name {
			return conn.Name, nil
		}
	}
	return falconcore.ConnectionName(""), fmt.Errorf("improper voltage state with name %s not present in the device config", v.Connection)
}

func NewZeroState(DeviceName falconcore.ConnectionName) VoltageState {
	return VoltageState{Voltage: 0.0, Connection: DeviceName, Unit: Volt}
}

type VoltageStates []VoltageState

func (v VoltageStates) FindMatchingDeviceNames(allDeviceConns []falconcore.Connection) ([]falconcore.ConnectionName, error) {
	var found []falconcore.ConnectionName
	for _, state := range v {
		name, err := state.FindMatchingDeviceName(allDeviceConns)
		if err != nil {
			return nil, err
		}
		found = append(found, name)
	}
	return found, nil
}

func (v *VoltageStates) DefaultToZero(
	allDeviceConns falconcore.Connections,
) error {
	matching, err := v.FindMatchingDeviceNames(allDeviceConns)
	if err != nil {
		return err
	}

	matched := make(map[falconcore.ConnectionName]struct{}, len(matching))
	for _, conn := range matching {
		matched[conn] = struct{}{}
	}

	for _, conn := range allDeviceConns {
		if _, ok := matched[conn.Name]; !ok {
			*v = append(*v, NewZeroState(conn.Name))
		}
	}

	return nil
}

// TODO: Need to write tests for this
// Need to connect to main.go. Need to connect to config contents
