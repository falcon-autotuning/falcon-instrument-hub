package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testMeasuredArray() MeasuredArray {
	return MeasuredArray{
		Data:  []float64{1, 2, 3, 4},
		Shape: []int{2, 2},
	}
}

func TestLabelledMeasuredArray_NewFalconLabelledMeasuredArray_Success(
	t *testing.T,
) {
	l := LabelledMeasuredArray{
		AcquisitionContext: testAcquisitionContext(),
		MeasuredArray:      testMeasuredArray(),
	}

	handle, err := l.NewFalconLabelledMeasuredArray()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestLabelledMeasuredArray_NewFalconLabelledMeasuredArray_ContextError(
	t *testing.T,
) {
	l := LabelledMeasuredArray{
		AcquisitionContext: AcquisitionContext{
			Connection: Connection{
				Name: "P1",
				Type: ConnectionType(999),
			},
			Unit:           Volt,
			InstrumentType: instrument.Amnmeter,
		},
		MeasuredArray: testMeasuredArray(),
	}

	handle, err := l.NewFalconLabelledMeasuredArray()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestLabelledMeasuredArray_NewFalconLabelledMeasuredArray_ArrayError(
	t *testing.T,
) {
	l := LabelledMeasuredArray{
		AcquisitionContext: testAcquisitionContext(),
		MeasuredArray: MeasuredArray{
			Data:  []float64{1},
			Shape: []int{-1},
		},
	}

	handle, err := l.NewFalconLabelledMeasuredArray()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestLabelledMeasuredArrayFromFalcon_MeasuredArrayError(
	t *testing.T,
) {
	expectedErr := errors.New("data failed")

	reader := &mockLabelledArrayReader{
		dataErr: expectedErr,
	}

	out, err := LabelledMeasuredArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, LabelledMeasuredArray{}, out)
}

func TestLabelledMeasuredArrayFromFalcon_AcquisitionContextError(
	t *testing.T,
) {
	expectedErr := errors.New("connection failed")

	reader := &mockLabelledArrayReader{
		data:  []float64{1, 2, 3, 4},
		shape: []uint64{2, 2},

		connErr: expectedErr,
	}

	out, err := LabelledMeasuredArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, LabelledMeasuredArray{}, out)
}

func TestLabelledMeasuredArrayFromFalcon_Success(
	t *testing.T,
) {
	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer conn.Close()

	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer unit.Close()

	reader := &mockLabelledArrayReader{
		data: []float64{1, 2, 3, 4},
		shape: []uint64{
			2,
			2,
		},

		conn: conn,
		unit: unit,

		instrumentType: instrument.Amnmeter,
	}

	out, err := LabelledMeasuredArrayFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(t,
		LabelledMeasuredArray{
			AcquisitionContext: testAcquisitionContext(),
			MeasuredArray: MeasuredArray{
				Data:  []float64{1, 2, 3, 4},
				Shape: []int{2, 2},
			},
		},
		out,
	)
}
