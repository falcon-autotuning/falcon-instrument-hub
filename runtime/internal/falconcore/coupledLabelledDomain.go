package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/coupledlabelleddomain"
)

type CoupledLabelledDomain []LabelledDomain

func (c CoupledLabelledDomain) NewFalconCoupledLabelledDomain() (*coupledlabelleddomain.Handle, error) {
	out, err := coupledlabelleddomain.NewEmpty()
	if err != nil {
		return nil, err
	}
	for _, ldomain := range c {
		realldomain, err := ldomain.NewFalconLabelledDomain()
		if err != nil {
			return nil, err
		}
		err = out.PushBack(realldomain)
		realldomain.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

type CoupledLabelledDomainReader interface {
	Items() (*listlabelleddomain.Handle, error)
}

var _ CoupledLabelledDomainReader = (*coupledlabelleddomain.Handle)(nil)

func CoupledLabelledDomainFromFalcon(c CoupledLabelledDomainReader) (CoupledLabelledDomain, error) {
	list, err := c.Items()
	if err != nil {
		return CoupledLabelledDomain{}, err
	}
	defer list.Close()
	size, err := list.Size()
	if err != nil {
		return CoupledLabelledDomain{}, err
	}
	out := make(CoupledLabelledDomain, 0, size)

	reallist, err := list.Items()
	if err != nil {
		return CoupledLabelledDomain{}, err
	}

	for _, ldomain := range reallist {
		realldomain, err := LabelledDomainFromFalcon(ldomain)
		defer ldomain.Close()
		if err != nil {
			return CoupledLabelledDomain{}, err
		}
		out = append(out, realldomain)
	}
	return out, nil
}
