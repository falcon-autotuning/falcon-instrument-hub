package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/labelleddomain"
)

type LabelledDomain struct {
	InstrumentPort
	Domain
}

func (l LabelledDomain) NewFalconLabelledDomain() (*labelleddomain.Handle, error) {
	port, err := l.NewFalconInstrumentPort()
	if err != nil {
		return nil, err
	}
	defer port.Close()
	domain, err := l.NewFalconDomain()
	if err != nil {
		return nil, err
	}
	defer domain.Close()
	return labelleddomain.NewFromPortAndDomain(port, domain)
}

type LabelledDomainReader interface {
	DomainReader
	Port() (*instrumentport.Handle, error)
}

var _ LabelledDomainReader = (*labelleddomain.Handle)(nil)

func LabelledDomainFromFalcon(l LabelledDomainReader) (LabelledDomain, error) {
	rawport, err := l.Port()
	if err != nil {
		return LabelledDomain{}, err
	}
	defer rawport.Close()
	port, err := InstrumentPortFromFalcon(rawport)
	if err != nil {
		return LabelledDomain{}, err
	}
	domain, err := DomainFromFalcon(l)
	if err != nil {
		return LabelledDomain{}, err
	}
	return LabelledDomain{port, domain}, nil
}
