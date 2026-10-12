package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportquantity"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
)

type SettingRequest struct {
	Getters Ports
	Setters map[InstrumentPort]Quantity
	Message string
}

func (s SettingRequest) NewFalconSettingRequest() (*settingrequest.Handle, error) {
	setters, err := mapinstrumentportquantity.NewEmpty()
	if err != nil {
		return nil, err
	}
	for key, value := range s.Setters {
		port, err := key.NewFalconInstrumentPort()
		if err != nil {
			return nil, err
		}
		quan, err := value.NewFalconQuantity()
		if err != nil {
			return nil, err
		}
		err = setters.Insert(port, quan)
		if err != nil {
			return nil, err
		}
	}
	getters, err := s.Getters.NewFalconPorts()
	if err != nil {
		return nil, err
	}
	return settingrequest.New(s.Message, getters, setters)
}

type SettingRequestReader interface {
	Getters() (*ports.Handle, error)
	Setters() (*mapinstrumentportquantity.Handle, error)
	Message() (string, error)
}

var _ SettingRequestReader = (*settingrequest.Handle)(nil)

func SettingRequestFromFalcon(s SettingRequestReader) (SettingRequest, error) {
	setters, err := s.Setters()
	if err != nil {
		return SettingRequest{}, err
	}
	defer setters.Close()
	list, err := setters.Items()
	if err != nil {
		return SettingRequest{}, err
	}
	defer list.Close()
	reallist, err := list.Items()
	if err != nil {
		return SettingRequest{}, err
	}
	out := make(map[InstrumentPort]Quantity, len(reallist))
	for _, pair := range reallist {
		key, err := pair.First()
		if err != nil {
			return SettingRequest{}, err
		}
		port, err := InstrumentPortFromFalcon(key)
		if err != nil {
			return SettingRequest{}, err
		}
		key.Close()
		value, err := pair.Second()
		if err != nil {
			return SettingRequest{}, err
		}
		quan, err := QuantityFromFalcon(value)
		if err != nil {
			return SettingRequest{}, err
		}
		value.Close()
		out[port] = quan
		pair.Close()
	}
	message, err := s.Message()
	if err != nil {
		return SettingRequest{}, err
	}
	rawgetters, err := s.Getters()
	if err != nil {
		return SettingRequest{}, err
	}
	getters, err := PortsFromFalcon(rawgetters)
	if err != nil {
		return SettingRequest{}, err
	}
	return SettingRequest{Setters: out, Getters: getters, Message: message}, nil
}
