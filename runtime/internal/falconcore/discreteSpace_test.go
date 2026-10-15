package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescoupledlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesinstrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axeslabelledcontrolarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesmapstringbool"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/domain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/unitspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDiscreteSpaceReader struct {
	space    *unitspace.Handle
	spaceErr error

	axes    *axescoupledlabelleddomain.Handle
	axesErr error

	increasing    *axesmapstringbool.Handle
	increasingErr error

	axis    int32
	axisErr error

	domain    *domain.Handle
	domainErr error

	projection    *axeslabelledcontrolarray.Handle
	projectionErr error
}

func (m *mockDiscreteSpaceReader) Space() (*unitspace.Handle, error) {
	return m.space, m.spaceErr
}

func (m *mockDiscreteSpaceReader) Axes() (*axescoupledlabelleddomain.Handle, error) {
	return m.axes, m.axesErr
}

func (m *mockDiscreteSpaceReader) Increasing() (*axesmapstringbool.Handle, error) {
	return m.increasing, m.increasingErr
}

func (m *mockDiscreteSpaceReader) GetAxis(
	*instrumentport.Handle,
) (int32, error) {
	return m.axis, m.axisErr
}

func (m *mockDiscreteSpaceReader) GetDomain(
	*instrumentport.Handle,
) (*domain.Handle, error) {
	return m.domain, m.domainErr
}

func (m *mockDiscreteSpaceReader) GetProjection(
	*axesinstrumentport.Handle,
) (*axeslabelledcontrolarray.Handle, error) {
	return m.projection, m.projectionErr
}

func testDiscreteSpaceUnitSpace(
	t *testing.T,
) *unitspace.Handle {
	t.Helper()

	raw, err := (UnitSpace{
		Axes: []Discretizer{
			{
				Type:  CartesianDiscretization,
				Delta: 0.5,
			},
		},
		Domain: Domain{
			LesserBound:  0,
			GreaterBound: 1,
		},
	}).NewFalconUnitSpace()

	require.NoError(t, err)
	return raw
}

func testDiscreteSpaceAxes(
	t *testing.T,
) *axescoupledlabelleddomain.Handle {
	t.Helper()

	raw, err := (AxesCoupledLabelledDomain{
		testCoupledLabelledDomain(),
	}).NewFalconAxesCoupledLabelledDomain()

	require.NoError(t, err)
	return raw
}

func testDiscreteSpaceIncreasing(
	t *testing.T,
) *axesmapstringbool.Handle {
	t.Helper()

	raw, err := (Increasing{
		{
			"P1": true,
		},
	}).NewFalconIncreasing()

	require.NoError(t, err)
	return raw
}

func testDiscretePort() InstrumentPort {
	return InstrumentPort{
		DefaultName:    "P1",
		InstrumentName: "dac",
		InstrumentType: instrument.Amnmeter,
		Description:    "gate",
		Unit:           Volt,
		PseudoName: Connection{
			Name: "P1",
			Type: PlungerGate,
		},
	}
}

func TestDiscreteSpaceFromFalcon_SpaceError(
	t *testing.T,
) {
	expected := errors.New("space failed")

	out, err := DiscreteSpaceFromFalcon(
		&mockDiscreteSpaceReader{
			spaceErr: expected,
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
	assert.Equal(t, DiscreteSpace{}, out)
}

func TestDiscreteSpaceFromFalcon_AxesError(
	t *testing.T,
) {
	rawSpace := testDiscreteSpaceUnitSpace(t)
	defer rawSpace.Close()

	expected := errors.New("axes failed")

	out, err := DiscreteSpaceFromFalcon(
		&mockDiscreteSpaceReader{
			space:   rawSpace,
			axesErr: expected,
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
	assert.Equal(t, DiscreteSpace{}, out)
}

func TestDiscreteSpaceFromFalcon_IncreasingError(
	t *testing.T,
) {
	rawSpace := testDiscreteSpaceUnitSpace(t)
	defer rawSpace.Close()

	rawAxes := testDiscreteSpaceAxes(t)
	defer rawAxes.Close()

	expected := errors.New("increasing failed")

	out, err := DiscreteSpaceFromFalcon(
		&mockDiscreteSpaceReader{
			space:         rawSpace,
			axes:          rawAxes,
			increasingErr: expected,
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
	assert.Equal(t, DiscreteSpace{}, out)
}

func TestDiscreteSpaceFromFalcon_Success(
	t *testing.T,
) {
	rawSpace := testDiscreteSpaceUnitSpace(t)
	defer rawSpace.Close()

	rawAxes := testDiscreteSpaceAxes(t)
	defer rawAxes.Close()

	rawIncreasing := testDiscreteSpaceIncreasing(t)
	defer rawIncreasing.Close()

	out, err := DiscreteSpaceFromFalcon(
		&mockDiscreteSpaceReader{
			space:      rawSpace,
			axes:       rawAxes,
			increasing: rawIncreasing,
		},
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		Increasing{
			{
				"P1": true,
			},
		},
		out.Increasing,
	)

	assert.NotNil(
		t,
		out.DiscreteSpaceProjector,
	)
}

func TestFalconDiscreteSpaceProjector_GetAxis(
	t *testing.T,
) {
	projector := &falconDiscreteSpaceProjector{
		reader: &mockDiscreteSpaceReader{
			axis: 3,
		},
	}

	out, err := projector.GetAxis(
		testDiscretePort(),
	)

	require.NoError(t, err)
	assert.Equal(t, 3, out)
}

func TestFalconDiscreteSpaceProjector_GetAxis_Error(
	t *testing.T,
) {
	expected := errors.New("axis failed")

	projector := &falconDiscreteSpaceProjector{
		reader: &mockDiscreteSpaceReader{
			axisErr: expected,
		},
	}

	_, err := projector.GetAxis(
		testDiscretePort(),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
}

func TestFalconDiscreteSpaceProjector_GetDomain_Error(
	t *testing.T,
) {
	expected := errors.New("domain failed")

	projector := &falconDiscreteSpaceProjector{
		reader: &mockDiscreteSpaceReader{
			domainErr: expected,
		},
	}

	_, err := projector.GetDomain(
		testDiscretePort(),
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
}

func TestFalconDiscreteSpaceProjector_GetDomain(
	t *testing.T,
) {
	raw, err := domain.New(
		0,
		10,
		false,
		false,
	)
	require.NoError(t, err)
	defer raw.Close()

	projector := &falconDiscreteSpaceProjector{
		reader: &mockDiscreteSpaceReader{
			domain: raw,
		},
	}

	out, err := projector.GetDomain(
		testDiscretePort(),
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		Domain{
			LesserBound:  0,
			GreaterBound: 10,
		},
		out,
	)
}

func TestFalconDiscreteSpaceProjector_GetProjection_Error(
	t *testing.T,
) {
	expected := errors.New("projection failed")

	projector := &falconDiscreteSpaceProjector{
		reader: &mockDiscreteSpaceReader{
			projectionErr: expected,
		},
	}
	projection := AxesInstrumentPort{
		testDiscretePort(),
	}

	_, err := projector.GetProjection(
		projection,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
}

func TestFalconDiscreteSpaceProjector_GetProjection(
	t *testing.T,
) {
	raw, err := (AxesLabelledControlArray{
		{
			AcquisitionContext: testAcquisitionContext(),
			ControlArray: ControlArray{
				Data:  []float64{1, 2},
				Shape: []int{2},
			},
		},
	}).NewFalconAxesLabelledControlArray()
	require.NoError(t, err)
	defer raw.Close()

	projector := &falconDiscreteSpaceProjector{
		reader: &mockDiscreteSpaceReader{
			projection: raw,
		},
	}

	projection := AxesInstrumentPort{
		testDiscretePort(),
	}

	out, err := projector.GetProjection(
		projection,
	)

	require.NoError(t, err)

	require.Len(t, out, 1)

	assert.Equal(
		t,
		[]float64{1, 2},
		out[0].Data,
	)
}

func TestDiscreteSpace_RoundTrip(
	t *testing.T,
) {
	raw, err := (DiscreteSpace{
		Space: UnitSpace{
			Axes: []Discretizer{
				{
					Type:  CartesianDiscretization,
					Delta: 0.5,
				},
			},
			Domain: Domain{
				LesserBound:  0,
				GreaterBound: 1,
			},
		},
		Axes: AxesCoupledLabelledDomain{
			testCoupledLabelledDomain(),
		},
		Increasing: Increasing{
			{
				"P1": true,
			},
		},
	}).NewFalconDiscreteSpace()

	require.NoError(t, err)
	defer raw.Close()

	_, err = DiscreteSpaceFromFalcon(raw)

	require.NoError(t, err)
}

func TestDiscreteSpaceFromFalcon_BadUnitSpace(
	t *testing.T,
) {
	out, err := DiscreteSpaceFromFalcon(
		&mockDiscreteSpaceReader{
			space: &unitspace.Handle{},
		},
	)

	require.Error(t, err)
	assert.Equal(t, DiscreteSpace{}, out)
}

func TestDiscreteSpaceFromFalcon_BadAxes(
	t *testing.T,
) {
	rawSpace := testDiscreteSpaceUnitSpace(t)
	defer rawSpace.Close()

	out, err := DiscreteSpaceFromFalcon(
		&mockDiscreteSpaceReader{
			space: rawSpace,
			axes:  &axescoupledlabelleddomain.Handle{},
		},
	)

	require.Error(t, err)
	assert.Equal(t, DiscreteSpace{}, out)
}

func TestDiscreteSpaceFromFalcon_BadIncreasing(
	t *testing.T,
) {
	rawSpace := testDiscreteSpaceUnitSpace(t)
	defer rawSpace.Close()

	rawAxes := testDiscreteSpaceAxes(t)
	defer rawAxes.Close()

	out, err := DiscreteSpaceFromFalcon(
		&mockDiscreteSpaceReader{
			space:      rawSpace,
			axes:       rawAxes,
			increasing: &axesmapstringbool.Handle{},
		},
	)

	require.Error(t, err)
	assert.Equal(t, DiscreteSpace{}, out)
}
