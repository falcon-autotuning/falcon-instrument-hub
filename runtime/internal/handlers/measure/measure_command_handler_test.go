package measurehandlers

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

type mockBusyManager struct {
	calls []bool
}

func (m *mockBusyManager) SetIsBusy(busy bool) {
	m.calls = append(m.calls, busy)
}

type mockRequest struct {
	closeCalls int
}

func (m *mockRequest) Close() error {
	m.closeCalls++
	return nil
}

type mockResponse struct {
	json string
	err  error

	closeCalls int
	jsonCalls  int
}

func (m *mockResponse) ToJSON() (string, error) {
	m.jsonCalls++

	if m.err != nil {
		return "", m.err
	}

	return m.json, nil
}

func (m *mockResponse) Close() error {
	m.closeCalls++
	return nil
}

type mockRequestFactory struct {
	req FalconRequest
	err error

	callCount int
}

func (m *mockRequestFactory) FromJSON(
	jsonStr string,
) (FalconRequest, error) {
	m.callCount++

	if m.err != nil {
		return nil, m.err
	}

	return m.req, nil
}

type mockRouter struct {
	resp FalconResponse
	err  error

	callCount int
}

func (m *mockRouter) Handle(
	req FalconRequest,
) (FalconResponse, error) {
	m.callCount++

	if m.err != nil {
		return nil, m.err
	}

	return m.resp, nil
}

func newTestHandler(
	logger *logging.Logger,
	busy BusyManager,
	router MeasurementRouter,
	factory FalconRequestFactory,
) *Handler {
	return &Handler{
		logger:         logger,
		busyManager:    busy,
		router:         router,
		requestFactory: factory,
	}
}

func testLogger(t *testing.T) *logging.Logger {
	t.Helper()

	logger, err := logging.NewLogger(t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() {
		logger.Close()
	})

	return logger
}

func TestMeasurementResponseSubject(t *testing.T) {
	assert.Equal(
		t,
		"FALCON.MEASURE_RESPONSE.123",
		measurementResponseSubject(123),
	)
}

func TestHandleMessage_InvalidJSON(t *testing.T) {
	busy := &mockBusyManager{}
	router := &mockRouter{}
	factory := &mockRequestFactory{}

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
		factory,
	)

	handler.handleMessage(
		&nats.Msg{
			Data: []byte("{"),
		},
	)

	assert.Equal(t, 0, factory.callCount)
	assert.Equal(t, 0, router.callCount)
	assert.Empty(t, busy.calls)
}

func TestHandleMessage_EmptyRequest(t *testing.T) {
	busy := &mockBusyManager{}
	router := &mockRouter{}
	factory := &mockRequestFactory{}

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
		factory,
	)

	cmd := api.MeasureCommand{
		Request: "",
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	handler.handleMessage(
		&nats.Msg{
			Data: data,
		},
	)

	assert.Equal(t, 0, factory.callCount)
	assert.Equal(t, 0, router.callCount)
	assert.Empty(t, busy.calls)
}

func TestHandleMessage_RequestFactoryError(
	t *testing.T,
) {
	busy := &mockBusyManager{}

	router := &mockRouter{}

	factory := &mockRequestFactory{
		err: errors.New("parse failure"),
	}

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
		factory,
	)

	cmd := api.MeasureCommand{
		Request: "{}",
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	handler.handleMessage(
		&nats.Msg{
			Data: data,
		},
	)

	assert.Equal(t, 1, factory.callCount)
	assert.Equal(t, 0, router.callCount)

	assert.Equal(
		t,
		[]bool{true, false},
		busy.calls,
	)
}

func TestHandleMessage_RouterError(t *testing.T) {
	busy := &mockBusyManager{}

	router := &mockRouter{
		err: errors.New("routing failed"),
	}

	factory := &mockRequestFactory{
		req: &mockRequest{},
	}

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
		factory,
	)

	cmd := api.MeasureCommand{
		Request: "{}",
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	handler.handleMessage(
		&nats.Msg{
			Data: data,
		},
	)

	assert.Equal(t, 1, factory.callCount)
	assert.Equal(t, 1, router.callCount)

	assert.Equal(
		t,
		[]bool{true, false},
		busy.calls,
	)
}

func TestHandleMessage_ClosesResponse(
	t *testing.T,
) {
	busy := &mockBusyManager{}

	req := &mockRequest{}

	resp := &mockResponse{
		err: errors.New("json failed"),
	}

	router := &mockRouter{
		resp: resp,
	}

	factory := &mockRequestFactory{
		req: req,
	}

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
		factory,
	)

	cmd := api.MeasureCommand{
		Request: "{}",
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	handler.handleMessage(
		&nats.Msg{Data: data},
	)

	assert.Equal(t, 1, req.closeCalls)
	assert.Equal(t, 1, resp.closeCalls)
}

func TestHandleMessage_ClosesRequestOnRouterFailure(
	t *testing.T,
) {
	req := &mockRequest{}

	router := &mockRouter{
		err: errors.New("boom"),
	}

	handler := newTestHandler(
		testLogger(t),
		&mockBusyManager{},
		router,
		&mockRequestFactory{
			req: req,
		},
	)

	cmd := api.MeasureCommand{
		Request: "{}",
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	handler.handleMessage(
		&nats.Msg{Data: data},
	)

	assert.Equal(t, 1, req.closeCalls)
}

func TestHandleMessage_BusyStateAlwaysReset(
	t *testing.T,
) {
	busy := &mockBusyManager{}

	router := &mockRouter{
		err: errors.New("boom"),
	}

	factory := &mockRequestFactory{
		req: &mockRequest{},
	}

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
		factory,
	)

	cmd := api.MeasureCommand{
		Request: "{}",
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	handler.handleMessage(
		&nats.Msg{
			Data: data,
		},
	)

	require.Len(t, busy.calls, 2)

	assert.True(t, busy.calls[0])
	assert.False(t, busy.calls[1])
}

func TestHandleMessage_ToJSONError(
	t *testing.T,
) {
	busy := &mockBusyManager{}

	response := &mockResponse{
		err: errors.New("json failure"),
	}

	router := &mockRouter{
		resp: response,
	}

	factory := &mockRequestFactory{
		req: &mockRequest{},
	}

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
		factory,
	)

	cmd := api.MeasureCommand{
		Request: "{}",
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	handler.handleMessage(
		&nats.Msg{Data: data},
	)

	assert.Equal(t, 1, response.jsonCalls)

	assert.Equal(
		t,
		[]bool{true, false},
		busy.calls,
	)
}

func TestNewMeasureCommandHandler_NilMetadata(
	t *testing.T,
) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()
	handler := newMeasureCommandHandler(
		logger,
		&mockBusyManager{},
		&mockRouter{},
		&mockRequestFactory{},
		nil,
		nil,
	)

	require.NotNil(t, handler)
	require.NotNil(t, handler.router)
	require.NotNil(t, handler.requestFactory)
}

func TestRouterAdapter_WrongRequestType(t *testing.T) {
	router := &routerAdapter{}

	resp, err := router.Handle(
		&mockRequest{},
	)

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(
		t,
		err.Error(),
		"expected *FalconMeasurementRequest",
	)
}

func TestFalconRequestFactory_InvalidJSON(
	t *testing.T,
) {
	factory := falconRequestFactory{}

	req, err := factory.FromJSON(
		"not valid json",
	)

	require.Error(t, err)
	assert.Nil(t, req)
}

func TestUnsubscribe_NoSubscription(
	t *testing.T,
) {
	handler := newTestHandler(
		testLogger(t),
		&mockBusyManager{},
		&mockRouter{},
		&mockRequestFactory{},
	)

	require.NoError(
		t,
		handler.Unsubscribe(),
	)
}

func runNATSServer(t *testing.T) *server.Server {
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // Use random port
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	s, err := server.NewServer(opts)
	require.NoError(t, err)

	go s.Start()

	// Wait for server to be ready
	if !s.ReadyForConnections(2 * time.Second) {
		t.Fatal("NATS server not ready for connections")
	}

	return s
}

func TestSubscribeAndUnsubscribe(
	t *testing.T,
) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(
		server.ClientURL(),
	)
	require.NoError(t, err)
	defer nc.Close()

	logger := testLogger(t)

	handler := newMeasureCommandHandler(
		logger,
		&mockBusyManager{},
		&mockRouter{},
		&mockRequestFactory{},
		nil,
		nil,
	)

	require.NoError(
		t,
		handler.Subscribe(nc),
	)

	assert.NotNil(t, handler.subscription)
	assert.NotNil(t, handler.js)

	require.NoError(
		t,
		handler.Unsubscribe(),
	)

	assert.Nil(t, handler.subscription)
}

func TestPublishMeasurementResponse(
	t *testing.T,
) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(
		server.ClientURL(),
	)
	require.NoError(t, err)
	defer nc.Close()

	js, err := nc.JetStream()
	require.NoError(t, err)

	_, err = js.AddStream(
		&nats.StreamConfig{
			Name: "FALCON_MEASURE",
			Subjects: []string{
				"FALCON.MEASURE_DATA.*",
			},
		},
	)
	require.NoError(t, err)

	handler := &Handler{
		logger: testLogger(t),
		nc:     nc,
		js:     js,
	}

	responseCh := make(
		chan *nats.Msg,
		1,
	)

	sub, err := nc.Subscribe(
		measurementResponseSubject(123),
		func(msg *nats.Msg) {
			responseCh <- msg
		},
	)
	require.NoError(t, err)
	defer sub.Unsubscribe()

	ok := handler.publishMeasurementResponse(
		api.MeasureCommand{
			Timestamp: 123,
			Hash:      10,
		},
		measurementResponseSubject(123),
		`{"result":"ok"}`,
	)

	assert.True(t, ok)

	select {
	case msg := <-responseCh:

		var resp api.MeasureResponse

		require.NoError(
			t,
			json.Unmarshal(
				msg.Data,
				&resp,
			),
		)

		assert.Equal(
			t,
			`{"result":"ok"}`,
			resp.Response,
		)

		assert.EqualValues(
			t,
			123,
			resp.Timestamp,
		)

	case <-time.After(
		2 * time.Second,
	):
		t.Fatal("timeout waiting for response")
	}
}

func TestHandler_ReceivesMeasureCommand(
	t *testing.T,
) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(
		server.ClientURL(),
	)
	require.NoError(t, err)
	defer nc.Close()

	req := &mockRequest{}

	resp := &mockResponse{
		json: `{"ok":true}`,
	}

	router := &mockRouter{
		resp: resp,
	}

	handler := newMeasureCommandHandler(
		testLogger(t),
		&mockBusyManager{},
		router,
		&mockRequestFactory{
			req: req,
		},
		nil,
		nil,
	)

	require.NoError(
		t,
		handler.Subscribe(nc),
	)

	defer handler.Unsubscribe()

	cmd := api.MeasureCommand{
		Request:   "{}",
		Timestamp: 999,
		Hash:      100,
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	require.NoError(
		t,
		nc.Publish(
			MeasureCommandSubject,
			data,
		),
	)

	require.NoError(
		t,
		nc.Flush(),
	)

	require.Eventually(
		t,
		func() bool {
			return router.callCount > 0
		},
		time.Second,
		10*time.Millisecond,
	)

	assert.Equal(t, 1, router.callCount)
	assert.Equal(t, 1, req.closeCalls)
	assert.Equal(t, 1, resp.closeCalls)
}

func TestRouterAdapter_WrongType(
	t *testing.T,
) {
	adapter := &routerAdapter{}

	resp, err := adapter.Handle(
		&mockRequest{},
	)

	require.Error(t, err)
	assert.Nil(t, resp)
}
