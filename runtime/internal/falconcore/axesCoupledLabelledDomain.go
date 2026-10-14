package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescoupledlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/coupledlabelleddomain"
)

type AxesCoupledLabelledDomain []CoupledLabelledDomain

func (a AxesCoupledLabelledDomain) NewFalconAxesCoupledLabelledDomain() (*axescoupledlabelleddomain.Handle, error) {
	if len(a) == 0 {
		return axescoupledlabelleddomain.NewEmpty()
	}
	out := make([]*coupledlabelleddomain.Handle, 0, len(a))
	for _, dom := range a {
		realdom, err := dom.NewFalconCoupledLabelledDomain()
		if err != nil {
			return nil, err
		}
		defer realdom.Close()
		out = append(out, realdom)
	}
	return axescoupledlabelleddomain.New(out)
}

type AxesCoupledLabelledDomainReader interface {
	Items() ([]*coupledlabelleddomain.Handle, error)
}

var _ AxesCoupledLabelledDomainReader = (*axescoupledlabelleddomain.Handle)(nil)

func AxesCoupledLabelledDomainFromFalcon(a AxesCoupledLabelledDomainReader) (AxesCoupledLabelledDomain, error) {
	list, err := a.Items()
	if err != nil {
		return AxesCoupledLabelledDomain{}, err
	}
	inner := make(AxesCoupledLabelledDomain, 0, len(list))
	for _, dom := range list {
		realdom, err := CoupledLabelledDomainFromFalcon(dom)
		if err != nil {
			return AxesCoupledLabelledDomain{}, err
		}
		inner = append(inner, realdom)
		dom.Close()
	}
	return inner, nil
}
