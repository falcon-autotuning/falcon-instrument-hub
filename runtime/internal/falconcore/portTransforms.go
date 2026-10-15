package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/port-transforms/porttransforms"
)

type PortTransforms []PortTransform

type PortTransformsReader interface {
	Items() (*listporttransform.Handle, error)
}

var _ PortTransformsReader = (*porttransforms.Handle)(nil)

func PortTransformsFromFalcon(
	p PortTransformsReader,
) (PortTransforms, error) {
	list, err := p.Items()
	if err != nil {
		return nil, err
	}
	defer list.Close()

	rawItems, err := list.Items()
	if err != nil {
		return nil, err
	}

	out := make(
		PortTransforms,
		0,
		len(rawItems),
	)

	for _, item := range rawItems {
		transform, err := PortTransformFromFalcon(item)
		if err != nil {
			return nil, err
		}

		out = append(out, transform)
	}

	return out, nil
}
