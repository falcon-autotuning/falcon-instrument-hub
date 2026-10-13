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

func PortExists(
	port falconcore.InstrumentPort,
	ports *config.ConnectedPorts,
) bool {
	if ports == nil {
		return false
	}

	defaultConn := falconcore.Connection{}
	for _, p := range ports.AllConnections {
		if !matchesPort(p, port) {
			continue
		}
		if port.PseudoName == defaultConn {
			return true
		}

		return p.Handle == port.PseudoName
	}

	return false
}
