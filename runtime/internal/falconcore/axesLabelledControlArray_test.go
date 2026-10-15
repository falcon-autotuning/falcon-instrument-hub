package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledcontrolarray"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAxesLabelledControlArrayReader struct {
	items []*labelledcontrolarray.Handle
	err   error

	itemsCalls int
}

func (m *mockAxesLabelledControlArrayReader) Items() (
	[]*labelledcontrolarray.Handle,
	error,
) {
	m.itemsCalls++

	if m.err != nil {
		return nil, m.err
	}

	return m.items, nil
}

func TestAxesLabelledControlArray_NewFalconAxesLabelledControlArray_Error(
	t *testing.T,
) {
	arr := AxesLabelledControlArray{
		{
			AcquisitionContext: testAcquisitionContext(),
			ControlArray: ControlArray{
				Data:  []float64{1},
				Shape: []int{-1},
			},
		},
	}

	handle, err := arr.NewFalconAxesLabelledControlArray()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestAxesLabelledControlArrayFromFalcon_ItemsError(
	t *testing.T,
) {
	expectedErr := errors.New("items failed")

	reader := &mockAxesLabelledControlArrayReader{
		err: expectedErr,
	}

	out, err := AxesLabelledControlArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, AxesLabelledControlArray{}, out)
	assert.Equal(t, 1, reader.itemsCalls)
}

func TestAxesLabelledControlArrayFromFalcon_LabelledControlArrayError(
	t *testing.T,
) {
	badHandle := &labelledcontrolarray.Handle{}

	reader := &mockAxesLabelledControlArrayReader{
		items: []*labelledcontrolarray.Handle{badHandle},
	}

	out, err := AxesLabelledControlArrayFromFalcon(reader)

	require.Error(t, err)
	assert.Equal(t, AxesLabelledControlArray{}, out)

	assert.Equal(t, 1, reader.itemsCalls)
}

func TestAxesLabelledControlArrayFromFalcon_Success(
	t *testing.T,
) {
	first, err := (LabelledControlArray{
		AcquisitionContext: testAcquisitionContext(),
		ControlArray: ControlArray{
			Data:  []float64{1, 2},
			Shape: []int{2},
		},
	}).NewFalconLabelledControlArray()
	require.NoError(t, err)
	defer first.Close()

	second, err := (LabelledControlArray{
		AcquisitionContext: AcquisitionContext{
			Connection: Connection{
				Name: "B1",
				Type: BarrierGate,
			},
			Unit:           Volt,
			InstrumentType: testAcquisitionContext().InstrumentType,
		},
		ControlArray: ControlArray{
			Data:  []float64{3, 4},
			Shape: []int{2},
		},
	}).NewFalconLabelledControlArray()
	require.NoError(t, err)
	defer second.Close()

	reader := &mockAxesLabelledControlArrayReader{
		items: []*labelledcontrolarray.Handle{
			first,
			second,
		},
	}

	out, err := AxesLabelledControlArrayFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		AxesLabelledControlArray{
			{
				AcquisitionContext: testAcquisitionContext(),
				ControlArray: ControlArray{
					Data:  []float64{1, 2},
					Shape: []int{2},
				},
			},
			{
				AcquisitionContext: AcquisitionContext{
					Connection: Connection{
						Name: "B1",
						Type: BarrierGate,
					},
					Unit:           Volt,
					InstrumentType: testAcquisitionContext().InstrumentType,
				},
				ControlArray: ControlArray{
					Data:  []float64{3, 4},
					Shape: []int{2},
				},
			},
		},
		out,
	)

	assert.Equal(t, 1, reader.itemsCalls)
}

func TestAxesLabelledControlArray_NewFalconAxesLabelledControlArray_Success(
	t *testing.T,
) {
	arr := AxesLabelledControlArray{
		{
			AcquisitionContext: testAcquisitionContext(),
			ControlArray: ControlArray{
				Data:  []float64{1, 2},
				Shape: []int{2},
			},
		},
		{
			AcquisitionContext: AcquisitionContext{
				Connection: Connection{
					Name: "B1",
					Type: BarrierGate,
				},
				Unit:           Volt,
				InstrumentType: testAcquisitionContext().InstrumentType,
			},
			ControlArray: ControlArray{
				Data:  []float64{3, 4},
				Shape: []int{2},
			},
		},
	}

	handle, err := arr.NewFalconAxesLabelledControlArray()

	require.NoError(t, err)
	require.NotNil(t, handle)

	items, err := handle.Items()
	require.NoError(t, err)

	assert.Len(t, items, 2)

	assert.NoError(t, handle.Close())
}
