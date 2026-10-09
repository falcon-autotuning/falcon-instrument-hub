//go:build cgo

package interpreter

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/devicestate"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHandler struct {
	name string

	canHandleResult bool
	canHandleErr    error

	handleResponse *FalconMeasurementResponse
	handleErr      error

	canHandleCalls int
	handleCalls    int

	updatePort bool

	lastRequest    *FalconMeasurementRequest
	lastDispatcher *dispatcher.MeasurementDispatcher
}

func (m *mockHandler) Name() string {
	return m.name
}

func (m *mockHandler) CanHandle(
	req *FalconMeasurementRequest,
) (bool, error) {
	m.canHandleCalls++
	m.lastRequest = req

	return m.canHandleResult, m.canHandleErr
}

func (m *mockHandler) Handle(
	req *FalconMeasurementRequest,
	dispatcher *dispatcher.MeasurementDispatcher,
	wiremap config.WireMap,
	ports *config.ConnectedPorts,
	tracker StateUpdater,
) (*FalconMeasurementResponse, error) {
	m.handleCalls++
	m.lastRequest = req
	m.lastDispatcher = dispatcher

	if m.updatePort {
		_ = tracker.UpdatePort(
			"P1",
			devicestate.Quantity{},
		)
	}

	return m.handleResponse, m.handleErr
}

func setupDeviceState(t *testing.T) {
	t.Helper()

	devicestate.Close()

	err := devicestate.Startup(
		devicestate.DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
				},
			},
		},
	)
	require.NoError(t, err)

	t.Cleanup(devicestate.Close)
}

func TestRouter_Handle_UniqueMatchingHandlerWins(t *testing.T) {
	setupDeviceState(t)
	dispatcher := &dispatcher.MeasurementDispatcher{}

	req := &FalconMeasurementRequest{}
	resp := &FalconMeasurementResponse{}

	first := &mockHandler{
		name:            "first",
		canHandleResult: false,
	}

	second := &mockHandler{
		name:            "second",
		canHandleResult: true,
		handleResponse:  resp,
		updatePort:      true,
	}

	third := &mockHandler{
		name:            "third",
		canHandleResult: false,
	}

	router := &Router{
		dispatcher: dispatcher,
		wiremap:    []config.WiremapEntry{},
		ports:      &config.ConnectedPorts{},
		handlers: []MeasurementHandler{
			first,
			second,
			third,
		},
	}

	result, err := router.Handle(req)

	require.NoError(t, err)
	assert.Same(t, resp, result)

	assert.Equal(t, 1, first.canHandleCalls)
	assert.Equal(t, 0, first.handleCalls)

	assert.Equal(t, 1, second.canHandleCalls)
	assert.Equal(t, 1, second.handleCalls)

	// Router stops at first match.
	assert.Equal(t, 0, third.canHandleCalls)
	assert.Equal(t, 0, third.handleCalls)

	assert.Same(t, dispatcher, second.lastDispatcher)
	assert.Same(t, req, second.lastRequest)
}

func TestRouter_Handle_FirstMatchWins(t *testing.T) {
	setupDeviceState(t)
	first := &mockHandler{
		name:            "first",
		canHandleResult: true,
		handleResponse:  &FalconMeasurementResponse{},
		updatePort:      true,
	}

	second := &mockHandler{
		name:            "second",
		canHandleResult: true,
	}

	router := &Router{
		handlers: []MeasurementHandler{
			first,
			second,
		},
	}

	_, err := router.Handle(&FalconMeasurementRequest{})

	require.NoError(t, err)

	assert.Equal(t, 1, first.canHandleCalls)
	assert.Equal(t, 1, first.handleCalls)

	assert.Equal(t, 0, second.canHandleCalls)
	assert.Equal(t, 0, second.handleCalls)
}

func TestRouter_Handle_CanHandleError(t *testing.T) {
	dispatcher := &dispatcher.MeasurementDispatcher{}

	expectedErr := errors.New("bad request")

	handler := &mockHandler{
		name:         "failing-handler",
		canHandleErr: expectedErr,
	}

	router := &Router{
		dispatcher: dispatcher,
		handlers: []MeasurementHandler{
			handler,
		},
	}

	resp, err := router.Handle(
		&FalconMeasurementRequest{},
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, expectedErr)

	assert.Equal(t, 1, handler.canHandleCalls)
	assert.Equal(t, 0, handler.handleCalls)
}

func TestRouter_Handle_HandlerError(t *testing.T) {
	setupDeviceState(t)
	dispatcher := &dispatcher.MeasurementDispatcher{}

	expectedErr := errors.New("measurement failed")

	handler := &mockHandler{
		name:            "matching-handler",
		canHandleResult: true,
		handleErr:       expectedErr,
	}

	router := &Router{
		dispatcher: dispatcher,
		handlers: []MeasurementHandler{
			handler,
		},
	}

	resp, err := router.Handle(
		&FalconMeasurementRequest{},
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, expectedErr)

	assert.Equal(t, 1, handler.canHandleCalls)
	assert.Equal(t, 1, handler.handleCalls)
}

func TestRouter_Handle_NoMatchingHandlers(t *testing.T) {
	dispatcher := &dispatcher.MeasurementDispatcher{}

	handler1 := &mockHandler{
		name:            "handler-1",
		canHandleResult: false,
	}

	handler2 := &mockHandler{
		name:            "handler-2",
		canHandleResult: false,
	}

	router := &Router{
		dispatcher: dispatcher,
		handlers: []MeasurementHandler{
			handler1,
			handler2,
		},
	}

	resp, err := router.Handle(
		&FalconMeasurementRequest{},
	)

	require.Error(t, err)
	assert.Nil(t, resp)

	assert.Contains(
		t,
		err.Error(),
		"no measurement handler matched request",
	)

	assert.Equal(t, 1, handler1.canHandleCalls)
	assert.Equal(t, 1, handler2.canHandleCalls)
}

func TestRouter_Handle_HandlerUpdatesState(t *testing.T) {
	devicestate.Close()

	err := devicestate.Startup(
		devicestate.DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
				},
			},
		},
	)
	require.NoError(t, err)

	defer devicestate.Close()

	resp := &FalconMeasurementResponse{}

	handler := &mockHandler{
		name:            "handler",
		canHandleResult: true,
		handleResponse:  resp,
		updatePort:      true,
	}

	router := &Router{
		handlers: []MeasurementHandler{
			handler,
		},
	}

	out, err := router.Handle(
		&FalconMeasurementRequest{},
	)

	require.NoError(t, err)
	assert.Same(t, resp, out)

	assert.Equal(t, 1, handler.canHandleCalls)
	assert.Equal(t, 1, handler.handleCalls)
}

func TestRouter_Handle_NoStateUpdate(t *testing.T) {
	devicestate.Close()

	err := devicestate.Startup(
		devicestate.DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
				},
			},
		},
	)
	require.NoError(t, err)

	defer devicestate.Close()

	handler := &mockHandler{
		name:            "handler",
		canHandleResult: true,
		handleResponse:  &FalconMeasurementResponse{},
		updatePort:      false,
	}

	router := &Router{
		handlers: []MeasurementHandler{
			handler,
		},
	}

	resp, err := router.Handle(
		&FalconMeasurementRequest{},
	)

	require.Error(t, err)
	assert.Nil(t, resp)

	assert.Contains(
		t,
		err.Error(),
		"completed without updating device state",
	)
}

func TestTrackedStateUpdater_UpdatePort(t *testing.T) {
	devicestate.Close()

	err := devicestate.Startup(
		devicestate.DeviceVoltageStates{
			"P1": {
				Connection: falconcore.Connection{
					Name: "P1",
				},
			},
		},
	)
	require.NoError(t, err)

	defer devicestate.Close()

	tracker := &trackedStateUpdater{
		manager: devicestate.Manager(),
	}

	err = tracker.UpdatePort(
		"P1",
		devicestate.Quantity{
			Value: 1,
		},
	)

	require.NoError(t, err)

	assert.Equal(t, 1, tracker.updates)
}

func TestRouter_Handle_ManagerNotInitialized(t *testing.T) {
	// Ensure no global manager exists.
	devicestate.Close()

	handler := &mockHandler{
		name:            "handler",
		canHandleResult: true,
		handleResponse:  &FalconMeasurementResponse{},
	}

	router := &Router{
		handlers: []MeasurementHandler{
			handler,
		},
	}

	resp, err := router.Handle(
		&FalconMeasurementRequest{},
	)

	require.Error(t, err)
	assert.Nil(t, resp)

	assert.Contains(
		t,
		err.Error(),
		"device state manager not initialized",
	)

	// CanHandle should still have been evaluated.
	assert.Equal(t, 1, handler.canHandleCalls)

	// Handle should never be reached because the router
	// detects the missing manager first.
	assert.Equal(t, 0, handler.handleCalls)
}
