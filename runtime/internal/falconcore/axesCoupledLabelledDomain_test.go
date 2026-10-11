package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/coupledlabelleddomain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAxesCoupledLabelledDomainReader struct {
	items []*coupledlabelleddomain.Handle
	err   error

	calls int
}

func (m *mockAxesCoupledLabelledDomainReader) Items() ([]*coupledlabelleddomain.Handle, error) {
	m.calls++

	if m.err != nil {
		return nil, m.err
	}

	return m.items, nil
}

func testCoupledLabelledDomain() CoupledLabelledDomain {
	return CoupledLabelledDomain{
		testLabelledDomain(),
		testLabelledDomain(),
	}
}

func TestAxesCoupledLabelledDomain_NewFalconAxesCoupledLabelledDomain_Error(
	t *testing.T,
) {
	axes := AxesCoupledLabelledDomain{
		{
			{
				InstrumentPort: InstrumentPort{
					Unit: Unit(999),
				},
			},
		},
	}

	handle, err := axes.NewFalconAxesCoupledLabelledDomain()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestAxesCoupledLabelledDomain_NewFalconAxesCoupledLabelledDomain_Success(
	t *testing.T,
) {
	axes := AxesCoupledLabelledDomain{
		testCoupledLabelledDomain(),
		testCoupledLabelledDomain(),
	}

	handle, err := axes.NewFalconAxesCoupledLabelledDomain()

	require.NoError(t, err)
	require.NotNil(t, handle)

	items, err := handle.Items()
	require.NoError(t, err)

	assert.Len(t, items, 2)

	assert.NoError(t, handle.Close())
}

func TestAxesCoupledLabelledDomainFromFalcon_ItemsError(
	t *testing.T,
) {
	expectedErr := errors.New("items failed")

	reader := &mockAxesCoupledLabelledDomainReader{
		err: expectedErr,
	}

	out, err := AxesCoupledLabelledDomainFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, AxesCoupledLabelledDomain{}, out)
	assert.Equal(t, 1, reader.calls)
}

func TestAxesCoupledLabelledDomainFromFalcon_Empty(
	t *testing.T,
) {
	reader := &mockAxesCoupledLabelledDomainReader{
		items: []*coupledlabelleddomain.Handle{},
	}

	out, err := AxesCoupledLabelledDomainFromFalcon(reader)

	require.NoError(t, err)

	assert.Empty(t, out)
	assert.Equal(t, 1, reader.calls)
}

func TestAxesCoupledLabelledDomainFromFalcon_Success(
	t *testing.T,
) {
	first, err := testCoupledLabelledDomain().
		NewFalconCoupledLabelledDomain()
	require.NoError(t, err)
	defer first.Close()

	second, err := testCoupledLabelledDomain().
		NewFalconCoupledLabelledDomain()
	require.NoError(t, err)
	defer second.Close()

	reader := &mockAxesCoupledLabelledDomainReader{
		items: []*coupledlabelleddomain.Handle{
			first,
			second,
		},
	}

	out, err := AxesCoupledLabelledDomainFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		AxesCoupledLabelledDomain{
			testCoupledLabelledDomain(),
			testCoupledLabelledDomain(),
		},
		out,
	)

	assert.Equal(t, 1, reader.calls)
}
