package devicestate

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/voltage-states/devicevoltagestate"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/voltage-states/devicevoltagestates"
)

type DeviceVoltageStates map[ConnectionName]DeviceVoltageState

func DeviceVoltageStatesFromFalcon(d *devicevoltagestates.Handle) (DeviceVoltageStates, error) {
	out := make(map[ConnectionName]DeviceVoltageState)
	list, err := d.Items()
	if err != nil {
		return DeviceVoltageStates{}, err
	}
	rawlist, err := list.Items()
	if err != nil {
		return DeviceVoltageStates{}, err
	}
	for _, state := range rawlist {
		realstate, err := DeviceVoltageStateFromFalcon(state)
		if err != nil {
			return DeviceVoltageStates{}, err
		}
		out[realstate.Connection.Name] = realstate
	}
	return DeviceVoltageStates(out), nil
}

func (d DeviceVoltageStates) NewFalconDeviceVoltageStates() (*devicevoltagestates.Handle, error) {
	var out []*devicevoltagestate.Handle
	for _, state := range d {
		realstate, err := state.NewFalconDeviceVoltageState()
		if err != nil {
			return nil, err
		}
		out = append(out, realstate)
	}
	if len(out) == 0 {
		return devicevoltagestates.NewEmpty()
	}
	return devicevoltagestates.New(out)
}
