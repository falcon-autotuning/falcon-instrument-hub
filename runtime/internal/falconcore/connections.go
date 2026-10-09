package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listconnection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connections"
)

type Connections []Connection

func (c Connections) NewFalconConnections() (*connections.Handle, error) {
	out, err := connections.NewEmpty()
	if err != nil {
		return nil, err
	}
	for _, conn := range c {
		realconn, err := conn.NewFalconConnection()
		if err != nil {
			return nil, err
		}
		out.PushBack(realconn)
	}
	return out, nil
}

type ConnectionsReader interface {
	Items() (*listconnection.Handle, error)
}

var _ ConnectionsReader = (*connections.Handle)(nil)

func ConnectionsFromFalcon(c ConnectionsReader) (Connections, error) {
	list, err := c.Items()
	if err != nil {
		return Connections{}, err
	}
	reallist, err := list.Items()
	if err != nil {
		return Connections{}, err
	}
	size, err := list.Size()
	if err != nil {
		return Connections{}, err
	}

	out := make(Connections, 0, size)

	for _, conn := range reallist {
		realconn, err := ConnectionFromFalcon(conn)
		if err != nil {
			return Connections{}, err
		}
		out = append(out, realconn)

	}
	return out, nil
}
