package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/coupledlabelleddomain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCoupledLabelledDomainReader struct {
	list *listlabelleddomain.Handle
	err  error

	itemsCalls int
}

func (m *mockCoupledLabelledDomainReader) Items() (*listlabelleddomain.Handle, error) {
	m.itemsCalls++

	if m.err != nil {
		return nil, m.err
	}

	return m.list, nil
}

func testLabelledDomain() LabelledDomain {
	return LabelledDomain{
		InstrumentPort: InstrumentPort{
			DefaultName:    "P1",
			InstrumentName: "DAC1",
			InstrumentType: instrument.Amnmeter,
			Description:    "test",
			Unit:           Volt,
			Access:         access.Readwrite,
			Scope:          scope.Global,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
			PortType:       porttype.PortTypeKnob,
		},
		Domain: Domain{
			LesserBound:           -1,
			GreaterBound:          1,
			LesserBoundContained:  true,
			GreaterBoundContained: false,
		},
	}
}

func TestCoupledLabelledDomain_NewFalconCoupledLabelledDomain_Empty(
	t *testing.T,
) {
	c := CoupledLabelledDomain{}

	handle, err := c.NewFalconCoupledLabelledDomain()

	require.NoError(t, err)
	require.NotNil(t, handle)

	list, err := handle.Items()
	require.NoError(t, err)

	size, err := list.Size()
	require.NoError(t, err)

	assert.Equal(t, uint64(0), size)

	assert.NoError(t, list.Close())
	assert.NoError(t, handle.Close())
}

func TestCoupledLabelledDomain_NewFalconCoupledLabelledDomain_Error(
	t *testing.T,
) {
	c := CoupledLabelledDomain{
		{
			InstrumentPort: InstrumentPort{
				Unit: Unit(999),
			},
			Domain: Domain{
				LesserBound:  0,
				GreaterBound: 1,
			},
		},
	}

	handle, err := c.NewFalconCoupledLabelledDomain()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestCoupledLabelledDomain_NewFalconCoupledLabelledDomain_Success(
	t *testing.T,
) {
	c := CoupledLabelledDomain{
		testLabelledDomain(),
		testLabelledDomain(),
	}

	handle, err := c.NewFalconCoupledLabelledDomain()

	require.NoError(t, err)
	require.NotNil(t, handle)

	list, err := handle.Items()
	require.NoError(t, err)

	size, err := list.Size()
	require.NoError(t, err)

	assert.Equal(t, uint64(2), size)

	assert.NoError(t, list.Close())
	assert.NoError(t, handle.Close())
}

func TestCoupledLabelledDomainFromFalcon_ItemsError(
	t *testing.T,
) {
	expectedErr := errors.New("items failed")

	reader := &mockCoupledLabelledDomainReader{
		err: expectedErr,
	}

	out, err := CoupledLabelledDomainFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Empty(t, out)

	assert.Equal(t, 1, reader.itemsCalls)
}

func TestCoupledLabelledDomainFromFalcon_Empty(
	t *testing.T,
) {
	handle, err := coupledlabelleddomain.NewEmpty()
	require.NoError(t, err)
	defer handle.Close()

	out, err := CoupledLabelledDomainFromFalcon(handle)

	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestCoupledLabelledDomainFromFalcon_Success(
	t *testing.T,
) {
	first, err := testLabelledDomain().NewFalconLabelledDomain()
	require.NoError(t, err)
	defer first.Close()

	second, err := testLabelledDomain().NewFalconLabelledDomain()
	require.NoError(t, err)
	defer second.Close()

	handle, err := coupledlabelleddomain.NewEmpty()
	require.NoError(t, err)
	defer handle.Close()

	require.NoError(t, handle.PushBack(first))
	require.NoError(t, handle.PushBack(second))

	out, err := CoupledLabelledDomainFromFalcon(handle)

	require.NoError(t, err)

	assert.Equal(t,
		CoupledLabelledDomain{
			testLabelledDomain(),
			testLabelledDomain(),
		},
		out,
	)
}
