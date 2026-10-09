package devicestate

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/voltage-states/devicevoltagestate"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

type DeviceVoltageState struct {
	Quantity
	Connection falconcore.Connection
}

func (d DeviceVoltageState) NewFalconDeviceVoltageState() (*devicevoltagestate.Handle, error) {
	conn, err := d.Connection.NewFalconConnection()
	if err != nil {
		return nil, err
	}
	quan, err := d.NewFalconQuantity()
	if err != nil {
		return nil, err
	}
	value, err := quan.Value()
	if err != nil {
		return nil, err
	}
	unit, err := quan.Unit()
	if err != nil {
		return nil, err
	}
	return devicevoltagestate.New(
		conn,
		value,
		unit,
	)
}

type DeviceVoltageStateReader interface {
	QuantityReader
	Connection() (*connection.Handle, error)
}

var _ DeviceVoltageStateReader = (*devicevoltagestate.Handle)(nil)

func DeviceVoltageStateFromFalcon(d DeviceVoltageStateReader) (DeviceVoltageState, error) {
	quantity, err := QuantityFromFalcon(d)
	if err != nil {
		return DeviceVoltageState{}, err
	}
	connection, err := d.Connection()
	if err != nil {
		return DeviceVoltageState{}, err
	}
	realconn, err := falconcore.ConnectionFromFalcon(connection)
	if err != nil {
		return DeviceVoltageState{}, err
	}
	return DeviceVoltageState{quantity, realconn}, nil
}
