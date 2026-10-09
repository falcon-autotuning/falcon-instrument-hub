package falconcore

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
)

type ConnectionType int

const (
	BarrierGate ConnectionType = iota
	PlungerGate
	ReservoirGate
	ScreeningGate
	Ohmic
)

type ConnectionName string

type Connection struct {
	Name ConnectionName
	Type ConnectionType
}

func (c Connection) NewFalconConnection() (*connection.Handle, error) {
	switch c.Type {
	case BarrierGate:
		return connection.NewBarrierGate(string(c.Name))
	case PlungerGate:
		return connection.NewPlungerGate(string(c.Name))
	case ReservoirGate:
		return connection.NewReservoirGate(string(c.Name))
	case ScreeningGate:
		return connection.NewScreeningGate(string(c.Name))
	case Ohmic:
		return connection.NewOhmic(string(c.Name))
	default:
		return nil, fmt.Errorf("unsupported connection type: %d", c.Type)
	}
}

type ConnectionReader interface {
	Name() (string, error)
	IsPlungerGate() (bool, error)
	IsBarrierGate() (bool, error)
	IsScreeningGate() (bool, error)
	IsReservoirGate() (bool, error)
	IsOhmic() (bool, error)
}

var _ ConnectionReader = (*connection.Handle)(nil)

func ConnectionFromFalcon(
	c ConnectionReader,
) (Connection, error) {
	name, err := c.Name()
	if err != nil {
		return Connection{}, err
	}
	if t, err := c.IsPlungerGate(); err == nil && t {
		return Connection{ConnectionName(name), PlungerGate}, nil
	}

	if t, err := c.IsBarrierGate(); err == nil && t {
		return Connection{ConnectionName(name), BarrierGate}, nil
	}

	if t, err := c.IsReservoirGate(); err == nil && t {
		return Connection{ConnectionName(name), ReservoirGate}, nil
	}

	if t, err := c.IsScreeningGate(); err == nil && t {
		return Connection{ConnectionName(name), ScreeningGate}, nil
	}

	if t, err := c.IsOhmic(); err == nil && t {
		return Connection{ConnectionName(name), Ohmic}, nil
	}
	return Connection{}, fmt.Errorf("unable to select connection type")
}
