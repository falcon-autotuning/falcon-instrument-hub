//go:build cgo

package config

import (
	"fmt"
	"os"

	falconconfig "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/config/core/config"
	falconloader "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/config/loader"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"gopkg.in/yaml.v3"
)

// WireMap is the top-level YAML structure and deprecated
type wireMap struct {
	Contents []WiremapEntry `yaml:"wiremap"`
}

func LoadWiremap(
	wiremapPath string,
	deviceConfigPath string,
) (*wireMap, error) {
	data, err := os.ReadFile(wiremapPath)
	if err != nil {
		return nil, err
	}

	var wiremap wireMap
	if err := yaml.Unmarshal(data, &wiremap); err != nil {
		return nil, err
	}
	err = ResolveWiremap(wiremap.Contents, deviceConfigPath)
	if err != nil {
		return nil, err
	}
	return &wiremap, nil
}

type WireMap []WiremapEntry

type WiremapEntry struct {
	// Logical device connection name, for example P1, B2, or O1
	PhysicalDeviceName string            `yaml:"name" json:"name"`
	Instrument         WiremapInstrument `yaml:"instrument" json:"instrument"`

	// Not serialized. Populated during validation.
	Gate *connection.Handle `yaml:"-" json:"-"`
}

type WiremapInstrument struct {
	// Instrument instance name, for example Source1 or Meter1
	Name string `yaml:"name" json:"name"`
	// Instrument channel group family used i.e. Analog
	ChannelGroup string `yaml:"channel_group" json:"channel_group"`
	// Physical channel index i.e. 1
	Channel int `yaml:"index" json:"index"`
}

func loadConfig(deviceConfigPath string) (*falconconfig.Handle, error) {
	lh, err := falconloader.New(deviceConfigPath)
	if err != nil {
		return nil, fmt.Errorf("falconloader.New: %w", err)
	}

	ch, err := lh.Config()
	if err != nil {
		return nil, fmt.Errorf("Loader.Config: %w", err)
	}

	return ch, nil
}

// ResolveWiremap loads the YAML and validates every wiremap entry
// against the Falcon device configuration. Each entry is linked
// to its resolved Falcon gate object.
func ResolveWiremap(
	entries []WiremapEntry,
	deviceConfigPath string,
) error {
	conf, err := loadConfig(deviceConfigPath)
	if err != nil {
		return err
	}

	connections, err := conf.GetAllConnections()
	if err != nil {
		return err
	}

	rawConnections, err := connections.Items()
	if err != nil {
		return err
	}

	listConnections, err := rawConnections.Items()
	if err != nil {
		return err
	}

	// Build lookup table once.
	gates := make(map[string]*connection.Handle, len(listConnections))

	for _, gate := range listConnections {
		name, err := gate.Name()
		if err != nil {
			return err
		}

		gates[name] = gate
	}

	// Resolve every wiremap entry.
	for i := range entries {
		gate, ok := gates[entries[i].PhysicalDeviceName]
		if !ok {
			return fmt.Errorf(
				"wiremap references unknown gate %q",
				entries[i].PhysicalDeviceName,
			)
		}

		entries[i].Gate = gate
	}

	return nil
}

func LoadDeviceConfig(deviceConfigPath string) (string, error) {
	conf, err := loadConfig(deviceConfigPath)
	if err != nil {
		return "", err
	}

	return conf.ToJSON()
}

type InstrumentConnection string
