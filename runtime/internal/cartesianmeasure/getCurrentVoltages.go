package cartesianmeasure

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
)

func InstrumentPortFromConnectedPort(
	p config.ConnectedPort,
) falconcore.InstrumentPort {
	return falconcore.InstrumentPort{
		DefaultName: falconcore.PortName(
			p.PortName,
		),
		InstrumentName: falconcore.InstrumentName(
			p.InstrumentName,
		),
		InstrumentType: p.InstrumentType,
		Description:    p.Description,
		Unit:           p.Unit,
		PseudoName:     p.Handle,
		Access:         p.Access,
		Scope:          p.Scope,
		Characteristic: p.Characteristic,
		PortType:       p.Role,
	}
}

func sourceVoltagePorts(
	ports *config.ConnectedPorts,
) []falconcore.InstrumentPort {
	var out []falconcore.InstrumentPort

	for _, knob := range ports.Knobs {
		if knob.Characteristic ==
			instrumentcharacteristic.
				InstrumentCharacteristicSourceVoltage {

			out = append(
				out,
				InstrumentPortFromConnectedPort(
					knob,
				),
			)
		}
	}

	return out
}

type SettingsReader interface {
	Handle(
		falconcore.SettingRequest,
	) (falconcore.SettingResponse, error)
}

var _ SettingsReader = (*settingrouter.Router)(nil)

func (h *CartesianMeasurementHandler) readSourceVoltages(
	ports *config.ConnectedPorts,
) (
	map[falconcore.InstrumentPort]falconcore.Quantity,
	error,
) {
	req := falconcore.SettingRequest{
		Getters: sourceVoltagePorts(ports),
		Message: "capture-source-voltages",
	}

	resp, err := h.settings.Handle(req)
	if err != nil {
		return nil, err
	}

	return resp.Getters, nil
}
