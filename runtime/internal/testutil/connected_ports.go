package testutil

import (
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
)

func BuildTestConnectedPorts(
	t *testing.T,
) *config.ConnectedPorts {
	t.Helper()

	return &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			{
				PortEntry: config.PortEntry{
					InstrumentName: "Source1",
					InstrumentType: instrument.DcVoltageSource,
					Role:           porttype.PortTypeKnob,
					Access:         access.Write,
					Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
				},
				PortName:   "Source1.voltage",
				DeviceName: "P1",
			},
			{
				PortEntry: config.PortEntry{
					InstrumentName: "Source1",
					InstrumentType: instrument.DcVoltageSource,
					Role:           porttype.PortTypeMeter,
					Access:         access.Read,
					Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
				},
				PortName:   "Source1.measure_voltage",
				DeviceName: "P1",
			},
		},
	}
}
