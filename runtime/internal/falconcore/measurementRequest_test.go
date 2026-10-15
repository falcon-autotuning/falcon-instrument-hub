package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listwaveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/pairinstrumentportporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/port-transforms/porttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/waveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretespace"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/labelleddomain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMeasurementRequestReader struct {
	message    string
	messageErr error

	waveforms    *listwaveform.Handle
	waveformsErr error

	getters    *ports.Handle
	gettersErr error

	meterTransforms    *mapinstrumentportporttransform.Handle
	meterTransformsErr error

	timeDomain    *labelleddomain.Handle
	timeDomainErr error
}

func (m *mockMeasurementRequestReader) Message() (string, error) {
	return m.message, m.messageErr
}

func (m *mockMeasurementRequestReader) Waveforms() (
	*listwaveform.Handle,
	error,
) {
	return m.waveforms, m.waveformsErr
}

func (m *mockMeasurementRequestReader) Getters() (
	*ports.Handle,
	error,
) {
	return m.getters, m.gettersErr
}

func (m *mockMeasurementRequestReader) MeterTransforms() (
	*mapinstrumentportporttransform.Handle,
	error,
) {
	return m.meterTransforms, m.meterTransformsErr
}

func (m *mockMeasurementRequestReader) TimeDomain() (
	*labelleddomain.Handle,
	error,
) {
	return m.timeDomain, m.timeDomainErr
}

type mockWaveformsReader struct {
	items    []*waveform.Handle
	itemsErr error
}

func (m *mockWaveformsReader) Items() (
	[]*waveform.Handle,
	error,
) {
	return m.items, m.itemsErr
}

func buildMeasurementRequestTestDiscreteSpaceHandle(
	t *testing.T,
) *discretespace.Handle {
	t.Helper()

	ds := DiscreteSpace{
		Space: UnitSpace{
			Axes: []Discretizer{
				{
					Type:  CartesianDiscretization,
					Delta: 0.5,
				},
			},
			Domain: Domain{
				LesserBound:  0,
				GreaterBound: 10,
			},
		},
		Axes: AxesCoupledLabelledDomain{
			{
				LabelledDomain{
					InstrumentPort: testDiscretePort(),
					Domain: Domain{
						LesserBound:  0,
						GreaterBound: 10,
					},
				},
			},
		},
		Increasing: Increasing{
			{
				string(testDiscretePort().DefaultName): true,
			},
		},
	}

	out, err := ds.NewFalconDiscreteSpace()
	require.NoError(t, err)

	return out
}

func validWaveformList(
	t *testing.T,
) *listwaveform.Handle {
	t.Helper()

	port := testDiscretePort()

	rawPort, err := port.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort.Close()

	transform, err := porttransform.NewIdentityTransform(
		rawPort,
	)
	require.NoError(t, err)
	defer transform.Close()

	rawTransforms, err := listporttransform.New(
		[]*porttransform.Handle{
			transform,
		},
	)
	require.NoError(t, err)
	defer rawTransforms.Close()

	rawWaveform, err := waveform.New(
		buildMeasurementRequestTestDiscreteSpaceHandle(t),
		rawTransforms,
	)
	require.NoError(t, err)

	list, err := listwaveform.New(
		[]*waveform.Handle{
			rawWaveform,
		},
	)
	require.NoError(t, err)

	return list
}

func validPorts(
	t *testing.T,
) *ports.Handle {
	t.Helper()

	port := testDiscretePort()

	rawPort, err := port.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort.Close()

	out, err := ports.New(
		[]*instrumentport.Handle{
			rawPort,
		},
	)
	require.NoError(t, err)

	return out
}

func validMeterTransforms(
	t *testing.T,
) *mapinstrumentportporttransform.Handle {
	t.Helper()

	port := testDiscretePort()

	rawPort, err := port.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort.Close()

	transform, err := porttransform.NewIdentityTransform(
		rawPort,
	)
	require.NoError(t, err)
	defer transform.Close()

	pair, err := pairinstrumentportporttransform.New(
		rawPort,
		transform,
	)
	require.NoError(t, err)
	defer pair.Close()

	out, err := mapinstrumentportporttransform.New(
		[]*pairinstrumentportporttransform.Handle{
			pair,
		},
	)
	require.NoError(t, err)

	return out
}

func validLabelledDomain(
	t *testing.T,
) *labelleddomain.Handle {
	t.Helper()

	out, err := (LabelledDomain{
		InstrumentPort: InstrumentPort{
			DefaultName:    "instrument.1.characteristic",
			InstrumentName: "instrument",
			InstrumentType: instrument.Clock,
			Unit:           Second,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
		},
		Domain: Domain{
			LesserBound:  0,
			GreaterBound: 10,
		},
	}).NewFalconLabelledDomain()

	require.NoError(t, err)

	return out
}

func TestWaveformsFromFalcon_ItemsError(
	t *testing.T,
) {
	expected := errors.New("items failed")

	out, err := WaveformsFromFalcon(
		&mockWaveformsReader{
			itemsErr: expected,
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
	assert.Nil(t, out)
}

func TestWaveformsFromFalcon_Empty(
	t *testing.T,
) {
	out, err := WaveformsFromFalcon(
		&mockWaveformsReader{},
	)

	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestMeterTransformsFromFalcon_ItemsError(
	t *testing.T,
) {
	out, err := MeterTransformsFromFalcon(
		&mapinstrumentportporttransform.Handle{},
	)

	require.Error(t, err)
	assert.Nil(t, out)
}

func TestMeterTransformsFromFalcon_Empty(
	t *testing.T,
) {
	raw, err := mapinstrumentportporttransform.NewEmpty()
	require.NoError(t, err)
	defer raw.Close()

	out, err := MeterTransformsFromFalcon(raw)

	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestMeterTransformsFromFalcon_Success(
	t *testing.T,
) {
	port := testDiscretePort()

	rawPort, err := port.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort.Close()

	transform, err := porttransform.NewIdentityTransform(
		rawPort,
	)
	require.NoError(t, err)
	defer transform.Close()

	pair, err := pairinstrumentportporttransform.New(
		rawPort,
		transform,
	)
	require.NoError(t, err)
	defer pair.Close()

	rawMap, err := mapinstrumentportporttransform.New(
		[]*pairinstrumentportporttransform.Handle{
			pair,
		},
	)
	require.NoError(t, err)
	defer rawMap.Close()

	out, err := MeterTransformsFromFalcon(
		rawMap,
	)

	require.NoError(t, err)

	require.Len(t, out, 1)

	value, ok := out[port]
	require.True(t, ok)

	assert.Equal(t, port, value.Port)
}

func TestMeasurementRequestFromFalcon_MessageError(
	t *testing.T,
) {
	expected := errors.New("message failed")

	out, err := MeasurementRequestFromFalcon(
		&mockMeasurementRequestReader{
			messageErr: expected,
		},
	)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, MeasurementRequest{}, out)
}

func TestMeasurementRequestFromFalcon_WaveformsError(
	t *testing.T,
) {
	expected := errors.New("waveforms failed")

	out, err := MeasurementRequestFromFalcon(
		&mockMeasurementRequestReader{
			message:      "hello",
			waveformsErr: expected,
		},
	)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, MeasurementRequest{}, out)
}

func TestMeasurementRequestFromFalcon_GettersError(
	t *testing.T,
) {
	expected := errors.New("getters failed")

	out, err := MeasurementRequestFromFalcon(
		&mockMeasurementRequestReader{
			message:    "hello",
			waveforms:  validWaveformList(t),
			gettersErr: expected,
		},
	)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, MeasurementRequest{}, out)
}

func TestMeasurementRequestFromFalcon_MeterTransformsError(
	t *testing.T,
) {
	expected := errors.New("meter transforms failed")

	out, err := MeasurementRequestFromFalcon(
		&mockMeasurementRequestReader{
			message:            "hello",
			waveforms:          validWaveformList(t),
			getters:            validPorts(t),
			meterTransformsErr: expected,
		},
	)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, MeasurementRequest{}, out)
}

func TestMeasurementRequestFromFalcon_TimeDomainError(
	t *testing.T,
) {
	expected := errors.New("time domain failed")

	out, err := MeasurementRequestFromFalcon(
		&mockMeasurementRequestReader{
			message:         "hello",
			waveforms:       validWaveformList(t),
			getters:         validPorts(t),
			meterTransforms: validMeterTransforms(t),
			timeDomainErr:   expected,
		},
	)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, MeasurementRequest{}, out)
}

func TestMeasurementRequestFromFalcon(
	t *testing.T,
) {
	out, err := MeasurementRequestFromFalcon(
		&mockMeasurementRequestReader{
			message:         "test message",
			waveforms:       validWaveformList(t),
			getters:         validPorts(t),
			meterTransforms: validMeterTransforms(t),
			timeDomain:      validLabelledDomain(t),
		},
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		"test message",
		out.Message,
	)

	assert.NotEmpty(
		t,
		out.Waveforms,
	)

	assert.NotEmpty(
		t,
		out.Getters,
	)

	assert.NotEmpty(
		t,
		out.MeterTransforms,
	)
}
