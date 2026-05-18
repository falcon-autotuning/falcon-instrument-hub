//go:build cgo && falcon_core
// +build cgo,falcon_core

package config

import (
	"fmt"
	"strings"

	falconconfig "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/config/core/config"
	falcongroup "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/config/core/group"
	falconloader "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/config/loader"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connections"
)

// // wiremapFile is the top-level YAML structure for the new wiremap format.
// type wiremapFile struct {
// 	Wiremap []wiremapEntry `yaml:"wiremap"`
// }

// // wiremapEntry is one entry in the wiremap sequence.
// type wiremapEntry struct {
// 	Name       string            `yaml:"name"`
// 	Instrument wiremapInstrument `yaml:"instrument"`
// }

// // wiremapInstrument holds the instrument channel details for a wiremap entry.
// type wiremapInstrument struct {
// 	Name        string `yaml:"name"`
// 	ChannelName string `yaml:"channel_name"`
// 	Index       int    `yaml:"index"`
// }

// func loadWireMap(path string) (*WireMap, error) {
// 	data, err := os.ReadFile(path)
// 	if err != nil {
// 		return nil, err
// 	}

// 	var raw wiremapFile
// 	if err := yaml.Unmarshal(data, &raw); err != nil {
// 		return nil, err
// 	}

// 	wireMap := make(WireMap, len(raw.Wiremap))
// 	for _, entry := range raw.Wiremap {
// 		// Key format matches the "instrumentName.index" lookup in port_processor.
// 		key := fmt.Sprintf("%s.%d", entry.Instrument.Name, entry.Instrument.Index)
// 		wireMap[InstrumentConnection(key)] = InstrumentConnection(entry.Name)
// 	}
// 	return &wireMap, nil
// }

// LoadConfigCGO loads both the device config (via falcon-core bindings) and the
// wiremap (via the existing YAML parser). It is the CGO equivalent of LoadConfig.
func LoadConfigCGO(deviceConfigPath, wiremapPath string) (*Config, error) {
	cfg := &Config{
		DeviceConfigPath: deviceConfigPath,
		WiremapPath:      wiremapPath,
	}

	deviceConfig, err := loadDeviceConfigCGO(deviceConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load device config: %w", err)
	}
	cfg.DeviceConfig = deviceConfig

	// The wiremap is a flat YAML file not handled by falcon-core; reuse the
	// existing loader.
	wireMap, err := loadWireMap(wiremapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load wiremap: %w", err)
	}
	cfg.WireMap = wireMap

	return cfg, nil
}

// loadDeviceConfigCGO uses the falcon-core Loader binding to parse the device
// config YAML and converts the resulting Config handle into the hub's DeviceConfig
// struct.
func loadDeviceConfigCGO(path string) (*DeviceConfig, error) {
	lh, err := falconloader.New(path)
	if err != nil {
		return nil, fmt.Errorf("falconloader.New: %w", err)
	}
	defer lh.Close()

	ch, err := lh.Config()
	if err != nil {
		return nil, fmt.Errorf("Loader.Config: %w", err)
	}
	defer ch.Close()

	dc := &DeviceConfig{}

	if dc.ScreeningGates, err = connectionsToSemicolon(ch, (*falconconfig.Handle).ScreeningGates); err != nil {
		return nil, fmt.Errorf("ScreeningGates: %w", err)
	}
	if dc.PlungerGates, err = connectionsToSemicolon(ch, (*falconconfig.Handle).PlungerGates); err != nil {
		return nil, fmt.Errorf("PlungerGates: %w", err)
	}
	if dc.Ohmics, err = connectionsToSemicolon(ch, (*falconconfig.Handle).Ohmics); err != nil {
		return nil, fmt.Errorf("Ohmics: %w", err)
	}
	if dc.BarrierGates, err = connectionsToSemicolon(ch, (*falconconfig.Handle).BarrierGates); err != nil {
		return nil, fmt.Errorf("BarrierGates: %w", err)
	}
	if dc.ReservoirGates, err = connectionsToSemicolon(ch, (*falconconfig.Handle).ReservoirGates); err != nil {
		return nil, fmt.Errorf("ReservoirGates: %w", err)
	}

	n, err := ch.NumUniqueChannels()
	if err != nil {
		return nil, fmt.Errorf("NumUniqueChannels: %w", err)
	}
	dc.NumUniqueChannels = int(n)

	if dc.Groups, err = extractGroups(ch); err != nil {
		return nil, fmt.Errorf("groups: %w", err)
	}

	if dc.WiringDC, err = extractWiringDC(ch); err != nil {
		return nil, fmt.Errorf("wiringDC: %w", err)
	}

	return dc, nil
}

// connectionsToSemicolon calls accessor on cfgHandle, then iterates the returned
// connections and joins their names with ";".
func connectionsToSemicolon(
	cfgHandle *falconconfig.Handle,
	accessor func(*falconconfig.Handle) (*connections.Handle, error),
) (string, error) {
	connsHandle, err := accessor(cfgHandle)
	if err != nil {
		return "", err
	}
	defer connsHandle.Close()
	return iterateConnectionNames(connsHandle)
}

// iterateConnectionNames iterates a connections handle and returns the gate
// names joined with ";".
func iterateConnectionNames(connsHandle *connections.Handle) (string, error) {
	size, err := connsHandle.Size()
	if err != nil {
		return "", err
	}
	names := make([]string, 0, size)
	for i := uint64(0); i < size; i++ {
		conn, err := connsHandle.At(i)
		if err != nil {
			return "", fmt.Errorf("connections.At(%d): %w", i, err)
		}
		name, err := conn.Name()
		conn.Close()
		if err != nil {
			return "", fmt.Errorf("connection.Name: %w", err)
		}
		names = append(names, name)
	}
	return strings.Join(names, ";"), nil
}

// extractGroups builds the DeviceConfig.Groups map from the falcon-core config.
func extractGroups(ch *falconconfig.Handle) (map[string]Group, error) {
	gnameList, err := ch.GetAllGnames()
	if err != nil {
		return nil, err
	}
	defer gnameList.Close()

	gnames, err := gnameList.Items()
	if err != nil {
		return nil, err
	}

	groups := make(map[string]Group, len(gnames))
	for _, gnameHandle := range gnames {
		key, err := gnameHandle.Gname()
		if err != nil {
			gnameHandle.Close()
			return nil, fmt.Errorf("gname.Gname: %w", err)
		}

		groupHandle, err := ch.SelectGroup(gnameHandle)
		gnameHandle.Close()
		if err != nil {
			return nil, fmt.Errorf("SelectGroup(%s): %w", key, err)
		}

		g, err := extractGroup(groupHandle)
		groupHandle.Close()
		if err != nil {
			return nil, fmt.Errorf("group %s: %w", key, err)
		}
		groups[key] = g
	}
	return groups, nil
}

// extractGroup converts a single falcon-core group handle into the hub's Group struct.
func extractGroup(gh *falcongroup.Handle) (Group, error) {
	var g Group
	var err error

	nameHandle, err := gh.Name()
	if err != nil {
		return g, fmt.Errorf("group.Name: %w", err)
	}
	defer nameHandle.Close()
	if g.Name, err = nameHandle.Name(); err != nil {
		return g, fmt.Errorf("channel.Name: %w", err)
	}

	numDots, err := gh.NumDots()
	if err != nil {
		return g, fmt.Errorf("group.NumDots: %w", err)
	}
	g.NumDots = int(numDots)

	if g.ScreeningGates, err = extractGroupGates(gh.ScreeningGates); err != nil {
		return g, fmt.Errorf("group.ScreeningGates: %w", err)
	}
	if g.ReservoirGates, err = extractGroupGates(gh.ReservoirGates); err != nil {
		return g, fmt.Errorf("group.ReservoirGates: %w", err)
	}
	if g.PlungerGates, err = extractGroupGates(gh.PlungerGates); err != nil {
		return g, fmt.Errorf("group.PlungerGates: %w", err)
	}
	if g.BarrierGates, err = extractGroupGates(gh.BarrierGates); err != nil {
		return g, fmt.Errorf("group.BarrierGates: %w", err)
	}

	orderHandle, err := gh.Order()
	if err != nil {
		return g, fmt.Errorf("group.Order: %w", err)
	}
	defer orderHandle.Close()
	linearConns, err := orderHandle.LinearArray()
	if err != nil {
		return g, fmt.Errorf("order.LinearArray: %w", err)
	}
	defer linearConns.Close()
	if g.Order, err = iterateConnectionNames(linearConns); err != nil {
		return g, fmt.Errorf("order names: %w", err)
	}

	return g, nil
}

// extractGroupGates calls the provided bound method and converts the result to a
// semicolon string.
func extractGroupGates(accessor func() (*connections.Handle, error)) (string, error) {
	h, err := accessor()
	if err != nil {
		return "", err
	}
	defer h.Close()
	return iterateConnectionNames(h)
}

// extractWiringDC iterates the impedances collection on the config handle and
// produces a DeviceConfig.WiringDC map.
func extractWiringDC(ch *falconconfig.Handle) (map[InstrumentConnection]WiringSpec, error) {
	impHandle, err := ch.WiringDc()
	if err != nil {
		return nil, err
	}
	defer impHandle.Close()

	size, err := impHandle.Size()
	if err != nil {
		return nil, err
	}

	wiringDC := make(map[InstrumentConnection]WiringSpec, size)
	for i := uint64(0); i < size; i++ {
		imp, err := impHandle.At(i)
		if err != nil {
			return nil, fmt.Errorf("impedances.At(%d): %w", i, err)
		}

		conn, err := imp.Connection()
		if err != nil {
			imp.Close()
			return nil, fmt.Errorf("impedance.Connection: %w", err)
		}
		connName, err := conn.Name()
		conn.Close()
		if err != nil {
			imp.Close()
			return nil, fmt.Errorf("connection.Name: %w", err)
		}

		resistance, err := imp.Resistance()
		if err != nil {
			imp.Close()
			return nil, fmt.Errorf("impedance.Resistance: %w", err)
		}
		capacitance, err := imp.Capacitance()
		imp.Close()
		if err != nil {
			return nil, fmt.Errorf("impedance.Capacitance: %w", err)
		}

		wiringDC[InstrumentConnection(connName)] = WiringSpec{
			Resistance:  resistance,
			Capacitance: capacitance,
		}
	}
	return wiringDC, nil
}
