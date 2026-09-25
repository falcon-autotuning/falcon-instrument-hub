//go:build cgo

package interpreter

import (
	"errors"
	"testing"

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

	lastRequest    *FalconMeasurementRequest
	lastDispatcher *MeasurementDispatcher
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
	dispatcher *MeasurementDispatcher,
) (*FalconMeasurementResponse, error) {
	m.handleCalls++
	m.lastRequest = req
	m.lastDispatcher = dispatcher

	return m.handleResponse, m.handleErr
}

func TestRouter_Handle_FirstMatchingHandlerWins(t *testing.T) {
	dispatcher := &MeasurementDispatcher{}

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
	}

	third := &mockHandler{
		name:            "third",
		canHandleResult: true,
	}

	router := &Router{
		dispatcher: dispatcher,
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

func TestRouter_Handle_CanHandleError(t *testing.T) {
	dispatcher := &MeasurementDispatcher{}

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
	dispatcher := &MeasurementDispatcher{}

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
	dispatcher := &MeasurementDispatcher{}

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
