package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/port-transforms/porttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretespace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWaveformReader struct {
	space    *discretespace.Handle
	spaceErr error

	transforms    *listporttransform.Handle
	transformsErr error
}

func (m *mockWaveformReader) Space() (
	*discretespace.Handle,
	error,
) {
	return m.space, m.spaceErr
}

func (m *mockWaveformReader) Transforms() (
	*listporttransform.Handle,
	error,
) {
	return m.transforms, m.transformsErr
}

var _ WaveformReader = (*mockWaveformReader)(nil)

func testDiscreteSpaceHandle(
	t *testing.T,
) *discretespace.Handle {
	t.Helper()

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
				GreaterBound: 10,
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

	return raw
}

func TestWaveformPortTransformsReader_Items(
	t *testing.T,
) {
	list, err := listporttransform.NewEmpty()
	require.NoError(t, err)
	defer list.Close()

	reader := &waveformPortTransformsReader{
		list: list,
	}

	out, err := reader.Items()

	require.NoError(t, err)
	assert.Same(t, list, out)
}

func TestWaveformFromFalcon_SpaceError(
	t *testing.T,
) {
	expected := errors.New("space failed")

	out, err := WaveformFromFalcon(
		&mockWaveformReader{
			spaceErr: expected,
		},
	)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		Waveform{},
		out,
	)
}

func TestWaveformFromFalcon_TransformsError(
	t *testing.T,
) {
	rawSpace := testDiscreteSpaceHandle(t)
	defer rawSpace.Close()

	expected := errors.New("transforms failed")

	out, err := WaveformFromFalcon(
		&mockWaveformReader{
			space:         rawSpace,
			transformsErr: expected,
		},
	)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		Waveform{},
		out,
	)
}

func TestWaveformFromFalcon_PortTransformsError(
	t *testing.T,
) {
	rawSpace := testDiscreteSpaceHandle(t)
	defer rawSpace.Close()

	badList := &listporttransform.Handle{}

	out, err := WaveformFromFalcon(
		&mockWaveformReader{
			space:      rawSpace,
			transforms: badList,
		},
	)

	require.Error(t, err)

	assert.Equal(
		t,
		Waveform{},
		out,
	)
}

func TestWaveformFromFalcon(
	t *testing.T,
) {
	rawSpace := testDiscreteSpaceHandle(t)
	defer rawSpace.Close()

	port := InstrumentPort{
		InstrumentName: "DAC",
	}

	rawPort, err := port.NewFalconInstrumentPort()
	require.NoError(t, err)
	defer rawPort.Close()

	transform, err := porttransform.NewIdentityTransform(
		rawPort,
	)
	require.NoError(t, err)
	defer transform.Close()

	list, err := listporttransform.New(
		[]*porttransform.Handle{
			transform,
		},
	)
	require.NoError(t, err)
	defer list.Close()

	out, err := WaveformFromFalcon(
		&mockWaveformReader{
			space:      rawSpace,
			transforms: list,
		},
	)

	require.NoError(t, err)

	assert.Len(
		t,
		out.Transforms,
		1,
	)

	assert.Equal(
		t,
		port,
		out.Transforms[0].Port,
	)

	assert.NotNil(
		t,
		out.Space.DiscreteSpaceProjector,
	)
}
