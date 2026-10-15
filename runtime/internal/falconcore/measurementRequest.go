package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listwaveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/waveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/labelleddomain"
)

type MeasurementRequest struct {
	Waveforms       Waveforms
	Getters         Ports
	MeterTransforms map[InstrumentPort]PortTransform
	TimeDomain      LabelledDomain
	Message         string
}

type MeasurementRequestReader interface {
	Getters() (*ports.Handle, error)

	Waveforms() (*listwaveform.Handle, error)

	MeterTransforms() (
		*mapinstrumentportporttransform.Handle,
		error,
	)

	TimeDomain() (*labelleddomain.Handle, error)

	Message() (string, error)
}

var _ MeasurementRequestReader = (*measurementrequest.Handle)(nil)

type waveformsReader struct {
	list *listwaveform.Handle
}

func (w *waveformsReader) Items() (
	*listwaveform.Handle,
	error,
) {
	return w.list, nil
}

type Waveforms []Waveform

type WaveformsReader interface {
	Items() ([]*waveform.Handle, error)
}

var _ WaveformsReader = (*listwaveform.Handle)(nil)

func WaveformsFromFalcon(
	w WaveformsReader,
) (Waveforms, error) {
	items, err := w.Items()
	if err != nil {
		return nil, err
	}

	out := make(
		Waveforms,
		0,
		len(items),
	)

	for _, item := range items {
		waveformValue, err := WaveformFromFalcon(
			item,
		)
		if err != nil {
			return nil, err
		}

		out = append(
			out,
			waveformValue,
		)
	}

	return out, nil
}

func MeterTransformsFromFalcon(
	m *mapinstrumentportporttransform.Handle,
) (map[InstrumentPort]PortTransform, error) {
	pairs, err := m.Items()
	if err != nil {
		return nil, err
	}
	rawpairs, err := pairs.Items()
	if err != nil {
		return nil, err
	}

	out := make(
		map[InstrumentPort]PortTransform,
		len(rawpairs),
	)

	for _, pair := range rawpairs {
		rawKey, err := pair.First()
		if err != nil {
			return nil, err
		}

		key, err := InstrumentPortFromFalcon(
			rawKey,
		)
		if err != nil {
			return nil, err
		}

		rawValue, err := pair.Second()
		if err != nil {
			return nil, err
		}

		value, err := PortTransformFromFalcon(
			rawValue,
		)
		if err != nil {
			return nil, err
		}

		out[key] = value
	}
	return out, nil
}

func MeasurementRequestFromFalcon(
	m MeasurementRequestReader,
) (MeasurementRequest, error) {
	message, err := m.Message()
	if err != nil {
		return MeasurementRequest{}, err
	}

	rawWaveforms, err := m.Waveforms()
	if err != nil {
		return MeasurementRequest{}, err
	}
	defer rawWaveforms.Close()

	waveforms, err := WaveformsFromFalcon(
		rawWaveforms,
	)
	if err != nil {
		return MeasurementRequest{}, err
	}

	rawGetters, err := m.Getters()
	if err != nil {
		return MeasurementRequest{}, err
	}
	defer rawGetters.Close()

	getters, err := PortsFromFalcon(
		rawGetters,
	)
	if err != nil {
		return MeasurementRequest{}, err
	}

	rawMeterTransforms, err := m.MeterTransforms()
	if err != nil {
		return MeasurementRequest{}, err
	}
	defer rawMeterTransforms.Close()

	meterTransforms, err := MeterTransformsFromFalcon(
		rawMeterTransforms,
	)
	if err != nil {
		return MeasurementRequest{}, err
	}

	rawTimeDomain, err := m.TimeDomain()
	if err != nil {
		return MeasurementRequest{}, err
	}
	defer rawTimeDomain.Close()

	timeDomain, err := LabelledDomainFromFalcon(
		rawTimeDomain,
	)
	if err != nil {
		return MeasurementRequest{}, err
	}

	return MeasurementRequest{
		Waveforms:       waveforms,
		Getters:         getters,
		MeterTransforms: meterTransforms,
		TimeDomain:      timeDomain,
		Message:         message,
	}, nil
}
