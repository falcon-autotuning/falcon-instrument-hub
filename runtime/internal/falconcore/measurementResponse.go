package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
)

type MeasurementResponse struct {
	Arrays  LabelledMeasuredArrays
	Message string
}

type MeasurementResponseReader interface {
	Arrays() (*labelledarrayslabelledmeasuredarray.Handle, error)
	Message() (string, error)
}

var _ MeasurementResponseReader = (*measurementresponse.Handle)(nil)

func MeasurementResponseFromFalcon(
	m MeasurementResponseReader,
) (MeasurementResponse, error) {
	rawArrays, err := m.Arrays()
	if err != nil {
		return MeasurementResponse{}, err
	}
	defer rawArrays.Close()

	arrays, err := LabelledMeasuredArraysFromFalcon(
		rawArrays,
	)
	if err != nil {
		return MeasurementResponse{}, err
	}

	message, err := m.Message()
	if err != nil {
		return MeasurementResponse{}, err
	}

	return MeasurementResponse{
		Arrays:  arrays,
		Message: message,
	}, nil
}

func (m MeasurementResponse) NewFalconMeasurementResponse() (
	*measurementresponse.Handle,
	error,
) {
	rawArrays, err := m.Arrays.
		NewFalconLabelledMeasuredArrays()
	if err != nil {
		return nil, err
	}
	defer rawArrays.Close()

	out, err := measurementresponse.New(
		rawArrays,
	)
	if err != nil {
		return nil, err
	}

	return out, nil
}
