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
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/settingguard"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/interpreter"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

// TODO: ensure full coverage tests

type mockBusyManager struct {
	calls []bool
}

func (m *mockBusyManager) SetIsBusy(busy bool) {
	m.calls = append(m.calls, busy)
}

type mockRouter struct {
	resp interpreter.MeasurementResponse
	err  error

	callCount int
}

func (m *mockRouter) Handle(
	req interpreter.MeasurementRequest,
) (
	interpreter.MeasurementResponse,
	error,
) {
	m.callCount++

	if m.err != nil {
		return interpreter.MeasurementResponse{}, m.err
	}

	return m.resp, nil
}

func newTestHandler(
	logger *logging.Logger,
	busy BusyManager,
	router MeasurementRouter,
) *Handler {
	return &Handler{
		logger:      logger,
		busyManager: busy,
		router:      router,
		writeGuard:  &settingguard.Guard{},
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

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
	)

	handler.handleMessage(
		&nats.Msg{
			Data: []byte("{"),
		},
	)

	assert.Equal(t, 0, router.callCount)
	assert.Empty(t, busy.calls)
}

func TestHandleMessage_EmptyRequest(t *testing.T) {
	busy := &mockBusyManager{}
	router := &mockRouter{}

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
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

	assert.Equal(t, 0, router.callCount)
	assert.Equal(t, 0, router.callCount)
	assert.Empty(t, busy.calls)
}

func TestHandleMessage_BusyStateAlwaysReset(
	t *testing.T,
) {
	busy := &mockBusyManager{}

	router := &mockRouter{
		err: errors.New("boom"),
	}

	handler := newTestHandler(
		testLogger(t),
		busy,
		router,
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
		nil,
		&settingguard.Guard{},
	)

	require.NotNil(t, handler)
	require.NotNil(t, handler.router)
}

func TestUnsubscribe_NoSubscription(
	t *testing.T,
) {
	handler := newTestHandler(
		testLogger(t),
		&mockBusyManager{},
		&mockRouter{},
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
		nil,
		&settingguard.Guard{},
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

func TestHandleMessage_BusyStateResetOnParseFailure(
	t *testing.T,
) {
	busy := &mockBusyManager{}

	handler := newTestHandler(
		testLogger(t),
		busy,
		&mockRouter{},
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

func TestHandleMessage_ReleasesWriteLockOnSuccess(
	t *testing.T,
) {
	guard := &settingguard.Guard{}

	router := &mockRouter{
		resp: interpreter.MeasurementResponse{},
	}

	handler := &Handler{
		logger:      testLogger(t),
		busyManager: &mockBusyManager{},
		router:      router,
		writeGuard:  guard,
	}

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

	assert.False(
		t,
		guard.WritesLocked(),
	)
}

func TestHandleMessage_ReleasesWriteLockOnFailure(
	t *testing.T,
) {
	guard := &settingguard.Guard{}

	router := &mockRouter{
		err: errors.New("boom"),
	}

	handler := &Handler{
		logger:      testLogger(t),
		busyManager: &mockBusyManager{},
		router:      router,
		writeGuard:  guard,
	}

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

	assert.False(
		t,
		guard.WritesLocked(),
	)
}

type mockMeasurementRunner struct{}

func (m *mockMeasurementRunner) RunAll(
	_ []interpreter.DispatchRequest,
) []interpreter.DispatchResult {
	return nil
}

type mockSettingsReader struct{}

func (m *mockSettingsReader) Handle(
	req falconcore.SettingRequest,
) (falconcore.SettingResponse, error) {
	return falconcore.SettingResponse{}, nil
}

func TestNewMeasureCommandHandler(
	t *testing.T,
) {
	handler := NewMeasureCommandHandler(
		testLogger(t),
		&mockBusyManager{},
		&mockMeasurementRunner{},
		nil,
		&settingguard.Guard{},
		&mockSettingsReader{},
	)

	require.NotNil(t, handler)
	require.NotNil(t, handler.router)
	require.NotNil(t, handler.writeGuard)
}
