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

// WireMap is the top-level YAML structure for the wiremap format.
type WireMap struct {
	Contents []WiremapEntry `yaml:"wiremap"`
}

type WiremapEntry struct {
	PhysicalDeviceName string            `yaml:"name"`
	Instrument         WiremapInstrument `yaml:"instrument"`

	// Not serialized. Populated during validation.
	Gate *connection.Handle `yaml:"-"`
}

type WiremapInstrument struct {
	Name         string `yaml:"name"`
	ChannelGroup string `yaml:"channel_group"`
	Channel      int    `yaml:"index"`
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

// LoadWiremap loads the YAML and validates every wiremap entry
// against the Falcon device configuration. Each entry is linked
// to its resolved Falcon gate object.
func LoadWiremap(
	wiremapPath string,
	deviceConfigPath string,
) (*WireMap, error) {
	data, err := os.ReadFile(wiremapPath)
	if err != nil {
		return nil, err
	}

	var wiremap WireMap
	if err := yaml.Unmarshal(data, &wiremap); err != nil {
		return nil, err
	}

	conf, err := loadConfig(deviceConfigPath)
	if err != nil {
		return nil, err
	}

	connections, err := conf.GetAllConnections()
	if err != nil {
		return nil, err
	}

	rawConnections, err := connections.Items()
	if err != nil {
		return nil, err
	}

	listConnections, err := rawConnections.Items()
	if err != nil {
		return nil, err
	}

	// Build lookup table once.
	gates := make(map[string]*connection.Handle, len(listConnections))

	for _, gate := range listConnections {
		name, err := gate.Name()
		if err != nil {
			return nil, err
		}

		gates[name] = gate
	}

	// Resolve every wiremap entry.
	for i := range wiremap.Contents {
		entry := &wiremap.Contents[i]

		gate, ok := gates[entry.PhysicalDeviceName]
		if !ok {
			return nil, fmt.Errorf(
				"wiremap references unknown gate %q",
				entry.PhysicalDeviceName,
			)
		}

		entry.Gate = gate
	}

	return &wiremap, nil
}

func LoadConfig(deviceConfigPath string) (string, error) {
	conf, err := loadConfig(deviceConfigPath)
	if err != nil {
		return "", err
	}

	return conf.ToJSON()
}

type InstrumentConnection string
