package falconcore

import "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/domain"

type Domain struct {
	LesserBound           float64
	GreaterBound          float64
	LesserBoundContained  bool
	GreaterBoundContained bool
}

func (d Domain) NewFalconDomain() (*domain.Handle, error) {
	return domain.New(d.LesserBound, d.GreaterBound, d.LesserBoundContained, d.GreaterBoundContained)
}

type DomainReader interface {
	LesserBound() (float64, error)
	GreaterBound() (float64, error)
	LesserBoundContained() (bool, error)
	GreaterBoundContained() (bool, error)
}

var _ DomainReader = (*domain.Handle)(nil)

func DomainFromFalcon(d DomainReader) (Domain, error) {
	lesser, err := d.LesserBound()
	if err != nil {
		return Domain{}, err
	}
	greater, err := d.GreaterBound()
	if err != nil {
		return Domain{}, err
	}
	lesserBound, err := d.LesserBoundContained()
	if err != nil {
		return Domain{}, err
	}
	greaterBound, err := d.GreaterBoundContained()
	if err != nil {
		return Domain{}, err
	}
	return Domain{LesserBound: lesser, GreaterBound: greater, LesserBoundContained: lesserBound, GreaterBoundContained: greaterBound}, nil
}
