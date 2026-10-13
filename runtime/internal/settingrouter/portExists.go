package settingrouter

import (
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

func matchesPort(
	cfg config.ConnectedPort,
	port falconcore.InstrumentPort,
) bool {
	return cfg.InstrumentName == string(port.InstrumentName) &&
		cfg.PortName == config.PortName(port.DefaultName) &&
		cfg.Characteristic == port.Characteristic &&
		cfg.InstrumentType == port.InstrumentType &&
		cfg.Role == port.PortType &&
		cfg.Access == port.Access &&
		cfg.Scope == port.Scope &&
		cfg.Unit == port.Unit
}

func ResolvePort(
	port falconcore.InstrumentPort,
	ports *config.ConnectedPorts,
) (*config.ConnectedPort, bool) {
	if ports == nil {
		return nil, false
	}

	defaultConn := falconcore.Connection{}

	for i := range ports.AllConnections {
		p := &ports.AllConnections[i]

		if !matchesPort(*p, port) {
			continue
		}

		if port.PseudoName == defaultConn {
			return p, true
		}

		if p.Handle == port.PseudoName {
			return p, true
		}
	}

	return nil, false
}

func PortExists(
	port falconcore.InstrumentPort,
	ports *config.ConnectedPorts,
) bool {
	_, ok := ResolvePort(
		port,
		ports,
	)

	return ok
}
