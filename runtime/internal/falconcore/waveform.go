package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/waveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretespace"
)

type Waveform struct {
	Space      DiscreteSpace
	Transforms PortTransforms
}

type WaveformReader interface {
	Space() (*discretespace.Handle, error)

	Transforms() (*listporttransform.Handle, error)
}

var _ WaveformReader = (*waveform.Handle)(nil)

type waveformPortTransformsReader struct {
	list *listporttransform.Handle
}

func (w *waveformPortTransformsReader) Items() (
	*listporttransform.Handle,
	error,
) {
	return w.list, nil
}

func WaveformFromFalcon(
	w WaveformReader,
) (Waveform, error) {
	rawSpace, err := w.Space()
	if err != nil {
		return Waveform{}, err
	}
	defer rawSpace.Close()

	space, err := DiscreteSpaceFromFalcon(
		rawSpace,
	)
	if err != nil {
		return Waveform{}, err
	}

	rawTransforms, err := w.Transforms()
	if err != nil {
		return Waveform{}, err
	}
	defer rawTransforms.Close()

	transforms, err := PortTransformsFromFalcon(
		&waveformPortTransformsReader{
			list: rawTransforms,
		},
	)
	if err != nil {
		return Waveform{}, err
	}

	return Waveform{
		Space:      space,
		Transforms: transforms,
	}, nil
}
