package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/voltagestatesresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/voltage-states/devicevoltagestates"
)

type VoltageStatesResponse struct {
	Message string
	States  DeviceVoltageStates
}

type VoltageStatesResponseReader interface {
	States() (*devicevoltagestates.Handle, error)
	Message() (string, error)
}

var _ VoltageStatesResponseReader = (*voltagestatesresponse.Handle)(nil)

func VoltageStatesResponseFromFalcon(
	v VoltageStatesResponseReader,
) (VoltageStatesResponse, error) {
	states, err := v.States()
	if err != nil {
		return VoltageStatesResponse{}, err
	}
	dstates, err := DeviceVoltageStatesFromFalcon(states)
	if err != nil {
		return VoltageStatesResponse{}, err
	}
	message, err := v.Message()
	if err != nil {
		return VoltageStatesResponse{}, err
	}
	return VoltageStatesResponse{Message: message, States: dstates}, nil
}

func (v VoltageStatesResponse) NewFalconVoltageStateResponse() (*voltagestatesresponse.Handle, error) {
	states, err := v.States.NewFalconDeviceVoltageStates()
	if err != nil {
		return nil, err
	}
	out, err := voltagestatesresponse.New(v.Message, states)
	states.Close()
	if err != nil {
		return nil, err
	}
	return out, nil
}
