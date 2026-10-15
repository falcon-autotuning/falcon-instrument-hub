package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/arrays/labelledarrayslabelledmeasuredarray"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockMeasurementResponseReader struct {
	arrays    *labelledarrayslabelledmeasuredarray.Handle
	arraysErr error

	message    string
	messageErr error

	arraysCalls  int
	messageCalls int
}

func (m *mockMeasurementResponseReader) Arrays() (
	*labelledarrayslabelledmeasuredarray.Handle,
	error,
) {
	m.arraysCalls++
	return m.arrays, m.arraysErr
}

func (m *mockMeasurementResponseReader) Message() (
	string,
	error,
) {
	m.messageCalls++
	return m.message, m.messageErr
}

var _ MeasurementResponseReader = (*mockMeasurementResponseReader)(nil)

func testMeasurementResponseArrays(
	t *testing.T,
) LabelledMeasuredArrays {
	t.Helper()

	return LabelledMeasuredArrays{
		testLabelledMeasuredArray("P1"),
		testLabelledMeasuredArray("P2"),
	}
}

func testMeasurementResponseArraysHandle(
	t *testing.T,
) *labelledarrayslabelledmeasuredarray.Handle {
	t.Helper()

	handle, err := testMeasurementResponseArrays(t).
		NewFalconLabelledMeasuredArrays()
	require.NoError(t, err)

	return handle
}

func TestMeasurementResponseFromFalcon_ArraysError(
	t *testing.T,
) {
	expected := errors.New("arrays failed")

	reader := &mockMeasurementResponseReader{
		arraysErr: expected,
	}

	out, err := MeasurementResponseFromFalcon(
		reader,
	)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		MeasurementResponse{},
		out,
	)

	assert.Equal(
		t,
		1,
		reader.arraysCalls,
	)

	assert.Equal(
		t,
		0,
		reader.messageCalls,
	)
}

func TestMeasurementResponseFromFalcon_MessageError(
	t *testing.T,
) {
	arrays := testMeasurementResponseArraysHandle(t)
	defer arrays.Close()

	expected := errors.New("message failed")

	reader := &mockMeasurementResponseReader{
		arrays:     arrays,
		messageErr: expected,
	}

	out, err := MeasurementResponseFromFalcon(
		reader,
	)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)

	assert.Equal(
		t,
		MeasurementResponse{},
		out,
	)

	assert.Equal(
		t,
		1,
		reader.arraysCalls,
	)

	assert.Equal(
		t,
		1,
		reader.messageCalls,
	)
}

func TestMeasurementResponseFromFalcon_Success(
	t *testing.T,
) {
	arrays := testMeasurementResponseArraysHandle(t)
	defer arrays.Close()

	reader := &mockMeasurementResponseReader{
		arrays:  arrays,
		message: "measurement complete",
	}

	out, err := MeasurementResponseFromFalcon(
		reader,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		MeasurementResponse{
			Arrays: LabelledMeasuredArrays{
				testLabelledMeasuredArray("P1"),
				testLabelledMeasuredArray("P2"),
			},
			Message: "measurement complete",
		},
		out,
	)

	assert.Equal(
		t,
		1,
		reader.arraysCalls,
	)

	assert.Equal(
		t,
		1,
		reader.messageCalls,
	)
}

func TestMeasurementResponse_NewFalconMeasurementResponse_Error(
	t *testing.T,
) {
	response := MeasurementResponse{
		Arrays: LabelledMeasuredArrays{
			{
				AcquisitionContext: AcquisitionContext{
					Connection: Connection{
						Name: "P1",
						Type: ConnectionType(999),
					},
				},
			},
		},
	}

	handle, err := response.
		NewFalconMeasurementResponse()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestMeasurementResponse_NewFalconMeasurementResponse_Success(
	t *testing.T,
) {
	response := MeasurementResponse{
		Arrays:  testMeasurementResponseArrays(t),
		Message: "ignored by falcon constructor",
	}

	handle, err := response.
		NewFalconMeasurementResponse()

	require.NoError(t, err)
	require.NotNil(t, handle)

	arrays, err := handle.Arrays()
	require.NoError(t, err)
	defer arrays.Close()

	out, err := LabelledMeasuredArraysFromFalcon(
		arrays,
	)
	require.NoError(t, err)

	assert.Equal(
		t,
		response.Arrays,
		out,
	)

	assert.NoError(
		t,
		handle.Close(),
	)
}

func TestMeasurementResponse_RoundTrip(
	t *testing.T,
) {
	original := MeasurementResponse{
		Arrays:  testMeasurementResponseArrays(t),
		Message: "round trip",
	}

	raw, err := original.
		NewFalconMeasurementResponse()

	require.NoError(t, err)
	defer raw.Close()

	out, err := MeasurementResponseFromFalcon(
		raw,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		original.Arrays,
		out.Arrays,
	)

	// Message is not passed into measurementresponse.New(),
	// so only arrays can be verified round-trip.
}
