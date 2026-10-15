package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringbool"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesmapstringbool"
)

type Increasing []map[string]bool

func (i Increasing) NewFalconIncreasing() (
	*axesmapstringbool.Handle,
	error,
) {
	if len(i) == 0 {
		return axesmapstringbool.NewEmpty()
	}

	out := make([]*mapstringbool.Handle, 0, len(i))

	for _, axis := range i {
		m, err := mapstringbool.NewEmpty()
		if err != nil {
			return nil, err
		}

		for k, v := range axis {
			if err := m.InsertOrAssign(k, v); err != nil {
				m.Close()
				return nil, err
			}
		}

		out = append(out, m)
	}

	return axesmapstringbool.New(out)
}

type IncreasingReader interface {
	Items() ([]*mapstringbool.Handle, error)
}

var _ IncreasingReader = (*axesmapstringbool.Handle)(nil)

func IncreasingFromFalcon(
	i IncreasingReader,
) (Increasing, error) {
	rawMaps, err := i.Items()
	if err != nil {
		return nil, err
	}

	out := make(Increasing, 0, len(rawMaps))

	for _, m := range rawMaps {
		pairs, err := m.Items()
		defer m.Close()
		if err != nil {
			return nil, err
		}
		defer pairs.Close()

		rawPairs, err := pairs.Items()
		if err != nil {
			return nil, err
		}

		current := make(map[string]bool)

		for _, p := range rawPairs {
			key, err := p.First()
			if err != nil {
				return nil, err
			}

			value, err := p.Second()
			if err != nil {
				return nil, err
			}

			current[key] = value
		}

		out = append(out, current)
	}

	return out, nil
}
