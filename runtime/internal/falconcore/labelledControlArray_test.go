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

type mockLabelledArrayReader struct {
	data     []float64
	dataErr  error
	shape    []uint64
	shapeErr error

	conn    *connection.Handle
	connErr error

	unit    *symbolunit.Handle
	unitErr error

	instrumentType instrument.Instrument
	instErr        error
}

func (m *mockLabelledArrayReader) Data() ([]float64, error) {
	if m.dataErr != nil {
		return nil, m.dataErr
	}
	return m.data, nil
}

func (m *mockLabelledArrayReader) Shape() ([]uint64, error) {
	if m.shapeErr != nil {
		return nil, m.shapeErr
	}
	return m.shape, nil
}

func (m *mockLabelledArrayReader) Connection() (*connection.Handle, error) {
	return m.conn, m.connErr
}

func (m *mockLabelledArrayReader) Units() (*symbolunit.Handle, error) {
	return m.unit, m.unitErr
}

func (m *mockLabelledArrayReader) InstrumentType() (instrument.Instrument, error) {
	return m.instrumentType, m.instErr
}

func testAcquisitionContext() AcquisitionContext {
	return AcquisitionContext{
		Connection: Connection{
			Name: "P1",
			Type: PlungerGate,
		},
		Unit:           Volt,
		InstrumentType: instrument.Amnmeter,
	}
}

func testControlArray() ControlArray {
	return ControlArray{
		Data:  []float64{1, 2, 3, 4},
		Shape: []int{1, 4},
	}
}

func TestLabelledControlArray_NewFalconLabelledControlArray_Success(
	t *testing.T,
) {
	l := LabelledControlArray{
		AcquisitionContext: testAcquisitionContext(),
		ControlArray:       testControlArray(),
	}

	handle, err := l.NewFalconLabelledControlArray()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestLabelledControlArray_NewFalconLabelledControlArray_ContextError(
	t *testing.T,
) {
	l := LabelledControlArray{
		AcquisitionContext: AcquisitionContext{
			Connection: Connection{
				Name: "P1",
				Type: ConnectionType(999),
			},
			Unit:           Volt,
			InstrumentType: instrument.Amnmeter,
		},
		ControlArray: testControlArray(),
	}

	handle, err := l.NewFalconLabelledControlArray()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestLabelledControlArray_NewFalconLabelledControlArray_ArrayError(
	t *testing.T,
) {
	l := LabelledControlArray{
		AcquisitionContext: testAcquisitionContext(),
		ControlArray: ControlArray{
			Data:  []float64{1},
			Shape: []int{-1},
		},
	}

	handle, err := l.NewFalconLabelledControlArray()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestLabelledControlArrayFromFalcon_ControlArrayError(
	t *testing.T,
) {
	expectedErr := errors.New("data failed")

	reader := &mockLabelledArrayReader{
		dataErr: expectedErr,
	}

	out, err := LabelledControlArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, LabelledControlArray{}, out)
}

func TestLabelledControlArrayFromFalcon_AcquisitionContextError(
	t *testing.T,
) {
	expectedErr := errors.New("connection failed")

	reader := &mockLabelledArrayReader{
		data:  []float64{1, 2, 3, 4},
		shape: []uint64{2, 2},

		connErr: expectedErr,
	}

	out, err := LabelledControlArrayFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, LabelledControlArray{}, out)
}

func TestLabelledControlArrayFromFalcon_Success(
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

	out, err := LabelledControlArrayFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(t,
		LabelledControlArray{
			AcquisitionContext: testAcquisitionContext(),
			ControlArray: ControlArray{
				Data:  []float64{1, 2, 3, 4},
				Shape: []int{2, 2},
			},
		},
		out,
	)
}
