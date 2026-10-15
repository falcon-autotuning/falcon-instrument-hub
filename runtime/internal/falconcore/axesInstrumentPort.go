package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesinstrumentport"
)

type AxesInstrumentPort []InstrumentPort

func (a AxesInstrumentPort) NewFalconAxesInstrumentPort() (*axesinstrumentport.Handle, error) {
	if len(a) == 0 {
		return axesinstrumentport.NewEmpty()
	}
	out := make([]*instrumentport.Handle, 0, len(a))
	for _, port := range a {
		realport, err := port.NewFalconInstrumentPort()
		if err != nil {
			return nil, err
		}
		defer realport.Close()
		out = append(out, realport)
	}
	return axesinstrumentport.New(out)
}

type AxesInstrumentPortReader interface {
	Items() ([]*instrumentport.Handle, error)
}

var _ AxesInstrumentPortReader = (*axesinstrumentport.Handle)(nil)

func AxesInstrumentPortFromFalcon(a AxesInstrumentPortReader) (AxesInstrumentPort, error) {
	list, err := a.Items()
	if err != nil {
		return AxesInstrumentPort{}, err
	}
	inner := make(AxesInstrumentPort, 0, len(list))
	for _, port := range list {
		carr, err := InstrumentPortFromFalcon(port)
		if err != nil {
			return AxesInstrumentPort{}, err
		}
		inner = append(inner, carr)
		port.Close()
	}
	return inner, nil
}
