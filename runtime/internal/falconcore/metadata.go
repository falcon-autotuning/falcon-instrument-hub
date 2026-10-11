package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listpairstringstring"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringstring"
)

type Metadata map[string]string

func (m Metadata) NewFalconMetadata() (*mapstringstring.Handle, error) {
	out, err := mapstringstring.NewEmpty()
	if err != nil {
		return nil, err
	}
	for key, value := range m {
		err := out.Insert(key, value)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

type MetadataReader interface {
	Items() (*listpairstringstring.Handle, error)
}

var _ MetadataReader = (*mapstringstring.Handle)(nil)

func MetadataFromFalcon(m MetadataReader) (Metadata, error) {
	list, err := m.Items()
	if err != nil {
		return Metadata{}, err
	}
	defer list.Close()
	rawlist, err := list.Items()
	if err != nil {
		return Metadata{}, err
	}
	out := make(Metadata, len(rawlist))
	for _, pair := range rawlist {
		key, err := pair.First()
		if err != nil {
			return Metadata{}, err
		}
		value, err := pair.Second()
		if err != nil {
			return Metadata{}, err
		}
		out[key] = value
		pair.Close()
	}
	return out, nil
}
