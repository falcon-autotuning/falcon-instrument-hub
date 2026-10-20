package cartesianmeasure

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"

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

	for _, setting := range ports.Settings {
		if setting.Characteristic ==
			instrumentcharacteristic.
				InstrumentCharacteristicSourceVoltage {

			out = append(
				out,
				InstrumentPortFromConnectedPort(
					setting,
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

func numberOfSamplesPorts(
	getters []Target,
	getterPorts map[Target]falconcore.InstrumentPort,
	ports *config.ConnectedPorts,
) (
	map[Target]falconcore.InstrumentPort,
	error,
) {
	out := make(
		map[Target]falconcore.InstrumentPort,
	)

	for _, getter := range getters {
		getterPort := getterPorts[getter]

		var local *config.ConnectedPort
		var global *config.ConnectedPort

		for i := range ports.Settings {
			setting := &ports.Settings[i]

			if setting.Characteristic !=
				instrumentcharacteristic.
					InstrumentCharacteristicNumberOfSamples {
				continue
			}

			if setting.InstrumentName !=
				string(getterPort.InstrumentName) {
				continue
			}

			if setting.Scope == scope.Local &&
				setting.ChannelGroup == getter.Group &&
				setting.Channel == getter.Channel {
				local = setting
				break
			}

			if setting.Scope == scope.Global {
				global = setting
			}
		}

		switch {
		case local != nil:
			out[getter] = InstrumentPortFromConnectedPort(
				*local,
			)

		case global != nil:
			out[getter] = InstrumentPortFromConnectedPort(
				*global,
			)

		default:
			return nil, fmt.Errorf(
				"no NumberOfSamples setting found for getter %+v",
				getter,
			)
		}
	}

	return out, nil
}

func (h *CartesianMeasurementHandler) readNumberOfPoints(
	getters []Target,
	getterPorts map[Target]falconcore.InstrumentPort,
	ports *config.ConnectedPorts,
) (NumberOfPoints, error) {
	mapping, err := numberOfSamplesPorts(
		getters,
		getterPorts,
		ports,
	)
	if err != nil {
		return nil, err
	}

	unique := make(
		map[falconcore.InstrumentPort]struct{},
	)

	var requests []falconcore.InstrumentPort

	for _, port := range mapping {
		if _, exists := unique[port]; exists {
			continue
		}

		unique[port] = struct{}{}
		requests = append(
			requests,
			port,
		)
	}

	resp, err := h.settings.Handle(
		falconcore.SettingRequest{
			Getters: requests,
			Message: "capture-number-of-samples",
		},
	)
	if err != nil {
		return nil, err
	}

	out := make(NumberOfPoints)

	for getter, samplePort := range mapping {
		value, ok := resp.Getters[samplePort]
		if !ok {
			return nil, fmt.Errorf(
				"missing sample count for getter %+v",
				getter,
			)
		}

		out[getter] = int(value.Value)
	}
	return out, nil
}
