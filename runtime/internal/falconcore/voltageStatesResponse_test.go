package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/voltage-states/devicevoltagestates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeVoltageStatesResponseReader struct {
	states    *devicevoltagestates.Handle
	statesErr error

	message    string
	messageErr error
}

func (f *fakeVoltageStatesResponseReader) States() (*devicevoltagestates.Handle, error) {
	return f.states, f.statesErr
}

func (f *fakeVoltageStatesResponseReader) Message() (string, error) {
	return f.message, f.messageErr
}

var _ VoltageStatesResponseReader = (*fakeVoltageStatesResponseReader)(nil)

func TestVoltageStatesResponseFromFalcon(t *testing.T) {
	states := DeviceVoltageStates{
		"P1": {
			Connection: Connection{
				Name: "P1",
				Type: PlungerGate,
			},
			Quantity: Quantity{
				Value: 1.23,
				Unit:  Volt,
			},
		},
	}

	handle, err := states.NewFalconDeviceVoltageStates()
	require.NoError(t, err)
	defer handle.Close()

	resp, err := VoltageStatesResponseFromFalcon(
		&fakeVoltageStatesResponseReader{
			states:  handle,
			message: "hello",
		},
	)

	require.NoError(t, err)

	assert.Equal(t, "hello", resp.Message)
	assert.Len(t, resp.States, 1)
}

func TestVoltageStatesResponseFromFalcon_StatesError(t *testing.T) {
	_, err := VoltageStatesResponseFromFalcon(
		&fakeVoltageStatesResponseReader{
			statesErr: errors.New("states error"),
		},
	)

	require.Error(t, err)
}

func TestVoltageStatesResponseFromFalcon_MessageError(t *testing.T) {
	states := DeviceVoltageStates{
		"P1": {
			Connection: Connection{
				Name: "P1",
				Type: PlungerGate,
			},
			Quantity: Quantity{
				Value: 1,
				Unit:  Volt,
			},
		},
	}

	handle, err := states.NewFalconDeviceVoltageStates()
	require.NoError(t, err)
	defer handle.Close()

	_, err = VoltageStatesResponseFromFalcon(
		&fakeVoltageStatesResponseReader{
			states:     handle,
			messageErr: errors.New("message error"),
		},
	)

	require.Error(t, err)
}

func TestVoltageStatesResponse_NewFalconVoltageStateResponse(t *testing.T) {
	original := VoltageStatesResponse{
		Message: "test message",
		States: DeviceVoltageStates{
			"P1": {
				Connection: Connection{
					Name: "P1",
					Type: PlungerGate,
				},
				Quantity: Quantity{
					Value: 3.14,
					Unit:  Volt,
				},
			},
		},
	}

	handle, err := original.NewFalconVoltageStateResponse()
	require.NoError(t, err)
	defer handle.Close()

	out, err := VoltageStatesResponseFromFalcon(handle)

	require.NoError(t, err)
	assert.Equal(t, original.Message, out.Message)
	assert.Len(t, out.States, 1)
}

func TestVoltageStatesResponse_NewFalconVoltageStateResponse_StateError(t *testing.T) {
	_, err := VoltageStatesResponse{
		Message: "test",
		States: DeviceVoltageStates{
			"bad": {
				Quantity: Quantity{
					Value: 1,
					Unit:  Unit(999),
				},
			},
		},
	}.NewFalconVoltageStateResponse()

	require.Error(t, err)
}
