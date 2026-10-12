package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportquantity"
)

type SettingResponse struct {
	Getters map[InstrumentPort]Quantity
	Message string
}

func (s SettingResponse) NewFalconSettingResponse() (*settingresponse.Handle, error) {
	out, err := mapinstrumentportquantity.NewEmpty()
	if err != nil {
		return nil, err
	}
	for key, value := range s.Getters {
		port, err := key.NewFalconInstrumentPort()
		if err != nil {
			return nil, err
		}
		quan, err := value.NewFalconQuantity()
		if err != nil {
			return nil, err
		}
		err = out.Insert(port, quan)
		if err != nil {
			return nil, err
		}
	}
	return settingresponse.New(s.Message, out)
}

type SettingResponseReader interface {
	Getters() (*mapinstrumentportquantity.Handle, error)
	Message() (string, error)
}

var _ SettingResponseReader = (*settingresponse.Handle)(nil)

func SettingResponseFromFalcon(s SettingResponseReader) (SettingResponse, error) {
	getters, err := s.Getters()
	if err != nil {
		return SettingResponse{}, err
	}
	defer getters.Close()
	list, err := getters.Items()
	if err != nil {
		return SettingResponse{}, err
	}
	defer list.Close()
	reallist, err := list.Items()
	if err != nil {
		return SettingResponse{}, err
	}
	out := make(map[InstrumentPort]Quantity, len(reallist))
	for _, pair := range reallist {
		key, err := pair.First()
		if err != nil {
			return SettingResponse{}, err
		}
		port, err := InstrumentPortFromFalcon(key)
		if err != nil {
			return SettingResponse{}, err
		}
		key.Close()
		value, err := pair.Second()
		if err != nil {
			return SettingResponse{}, err
		}
		quan, err := QuantityFromFalcon(value)
		if err != nil {
			return SettingResponse{}, err
		}
		value.Close()
		out[port] = quan
		pair.Close()
	}
	message, err := s.Message()
	if err != nil {
		return SettingResponse{}, err
	}
	return SettingResponse{Getters: out, Message: message}, nil
}
