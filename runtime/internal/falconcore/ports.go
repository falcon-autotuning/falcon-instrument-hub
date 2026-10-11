package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listinstrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
)

type Ports []InstrumentPort

func (p Ports) NewFalconPorts() (*ports.Handle, error) {
	out, err := ports.NewEmpty()
	if err != nil {
		return nil, err
	}
	for _, port := range p {
		realport, err := port.NewFalconInstrumentPort()
		if err != nil {
			return nil, err
		}
		err = out.PushBack(realport)
		realport.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

type PortsReader interface {
	Ports() (*listinstrumentport.Handle, error)
}

var _ PortsReader = (*ports.Handle)(nil)

func PortsFromFalcon(p PortsReader) (Ports, error) {
	list, err := p.Ports()
	if err != nil {
		return Ports{}, err
	}
	defer list.Close()
	size, err := list.Size()
	if err != nil {
		return Ports{}, err
	}
	out := make(Ports, 0, size)

	reallist, err := list.Items()
	if err != nil {
		return Ports{}, err
	}

	for _, port := range reallist {
		realport, err := InstrumentPortFromFalcon(port)
		defer port.Close()
		if err != nil {
			return Ports{}, err
		}
		out = append(out, realport)

	}
	return out, nil
}
