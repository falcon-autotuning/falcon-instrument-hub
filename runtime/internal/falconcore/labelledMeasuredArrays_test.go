package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listlabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledmeasuredarray"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLabelledMeasuredArraysReader struct {
	list *listlabelledmeasuredarray.Handle
	err  error

	calls int
}

func (m *mockLabelledMeasuredArraysReader) Arrays() (*listlabelledmeasuredarray.Handle, error) {
	m.calls++

	if m.err != nil {
		return nil, m.err
	}

	return m.list, nil
}

func testLabelledMeasuredArray(name string) LabelledMeasuredArray {
	return LabelledMeasuredArray{
		AcquisitionContext: AcquisitionContext{
			Connection: Connection{
				Name: ConnectionName(name),
				Type: PlungerGate,
			},
			Unit:           Volt,
			InstrumentType: instrument.Amnmeter,
		},
		MeasuredArray: MeasuredArray{
			Data:  []float64{1, 2, 3, 4},
			Shape: []int{2, 2},
		},
	}
}

func TestLabelledMeasuredArrays_NewFalconLabelledMeasuredArrays_Error(
	t *testing.T,
) {
	arrays := LabelledMeasuredArrays{
		{
			AcquisitionContext: AcquisitionContext{
				Connection: Connection{
					Name: "P1",
					Type: ConnectionType(999),
				},
				Unit:           Volt,
				InstrumentType: instrument.Amnmeter,
			},
		},
	}

	handle, err := arrays.NewFalconLabelledMeasuredArrays()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestLabelledMeasuredArrays_NewFalconLabelledMeasuredArrays_Success(
	t *testing.T,
) {
	arrays := LabelledMeasuredArrays{
		testLabelledMeasuredArray("P1"),
		testLabelledMeasuredArray("P2"),
	}

	handle, err := arrays.NewFalconLabelledMeasuredArrays()

	require.NoError(t, err)
	require.NotNil(t, handle)

	list, err := handle.Arrays()
	require.NoError(t, err)

	items, err := list.Items()
	require.NoError(t, err)

	assert.Len(t, items, 2)

	assert.NoError(t, list.Close())
	assert.NoError(t, handle.Close())
}

func TestLabelledMeasuredArraysFromFalcon_ArraysError(
	t *testing.T,
) {
	expectedErr := errors.New("arrays failed")

	reader := &mockLabelledMeasuredArraysReader{
		err: expectedErr,
	}

	out, err := LabelledMeasuredArraysFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, LabelledMeasuredArrays{}, out)

	assert.Equal(t, 1, reader.calls)
}

func TestLabelledMeasuredArraysFromFalcon_Success(
	t *testing.T,
) {
	first, err := testLabelledMeasuredArray("P1").
		NewFalconLabelledMeasuredArray()
	require.NoError(t, err)
	defer first.Close()

	second, err := testLabelledMeasuredArray("P2").
		NewFalconLabelledMeasuredArray()
	require.NoError(t, err)
	defer second.Close()

	handle, err := labelledarrayslabelledmeasuredarray.New(
		[]*labelledmeasuredarray.Handle{
			first,
			second,
		},
	)
	require.NoError(t, err)
	defer handle.Close()

	out, err := LabelledMeasuredArraysFromFalcon(handle)

	require.NoError(t, err)

	assert.Equal(
		t,
		LabelledMeasuredArrays{
			testLabelledMeasuredArray("P1"),
			testLabelledMeasuredArray("P2"),
		},
		out,
	)
}
