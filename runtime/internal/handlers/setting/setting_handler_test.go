package settinghandler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/quantity"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/testutil"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingResponseSubject(t *testing.T) {
	assert.Equal(
		t,
		"FALCON.SETTING_RESPONSE.123",
		settingResponseSubject(123),
	)
}

type mockBusyManager struct {
	calls []bool
}

func (m *mockBusyManager) SetIsBusy(
	busy bool,
) {
	m.calls = append(
		m.calls,
		busy,
	)
}

func newTestHandler(
	logger *logging.Logger,
	busy BusyManager,
	router SettingRouter,
) *Handler {
	return &Handler{
		logger:      logger,
		busyManager: busy,
		router:      router,
	}
}

func TestHandleMessage_InvalidJSON(
	t *testing.T,
) {
	busy := &mockBusyManager{}

	handler := newTestHandler(
		testutil.TestLogger(t),
		busy,
		&mockRouter{},
	)

	handler.handleMessage(
		&nats.Msg{
			Data: []byte("{"),
		},
	)

	assert.Empty(
		t,
		busy.calls,
	)
}

func TestHandleMessage_EmptyRequest(
	t *testing.T,
) {
	busy := &mockBusyManager{}

	handler := newTestHandler(
		testutil.TestLogger(t),
		busy,
		&mockRouter{},
	)

	data, err := json.Marshal(
		api.SettingCommand{
			Request: "",
		},
	)
	require.NoError(t, err)

	handler.handleMessage(
		&nats.Msg{
			Data: data,
		},
	)

	assert.Empty(
		t,
		busy.calls,
	)
}

func TestHandleMessage_BusyStateAlwaysReset(
	t *testing.T,
) {
	busy := &mockBusyManager{}

	handler := newTestHandler(
		testutil.TestLogger(t),
		busy,
		&mockRouter{},
	)

	handler.handleMessage(
		&nats.Msg{
			Data: []byte(`{
				"request":"invalid"
			}`),
		},
	)

	require.Len(
		t,
		busy.calls,
		2,
	)

	assert.True(
		t,
		busy.calls[0],
	)

	assert.False(
		t,
		busy.calls[1],
	)
}

func TestSubscribeAndUnsubscribe(
	t *testing.T,
) {
	server := testutil.RunNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(
		server.ClientURL(),
	)
	require.NoError(t, err)
	defer nc.Close()

	handler := newTestHandler(
		testutil.TestLogger(t),
		&mockBusyManager{},
		&mockRouter{},
	)

	require.NoError(
		t,
		handler.Subscribe(nc),
	)

	assert.NotNil(
		t,
		handler.subscription,
	)

	assert.NotNil(
		t,
		handler.js,
	)

	require.NoError(
		t,
		handler.Unsubscribe(),
	)

	assert.Nil(
		t,
		handler.subscription,
	)
}

func TestPublishSettingResponse(
	t *testing.T,
) {
	server := testutil.RunNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(
		server.ClientURL(),
	)
	require.NoError(t, err)
	defer nc.Close()

	handler := &Handler{
		logger: testutil.TestLogger(t),
		nc:     nc,
	}

	responseCh := make(
		chan *nats.Msg,
		1,
	)

	sub, err := nc.Subscribe(
		settingResponseSubject(123),
		func(msg *nats.Msg) {
			responseCh <- msg
		},
	)
	require.NoError(t, err)
	defer sub.Unsubscribe()

	ok := handler.publishSettingmentResponse(
		api.SettingCommand{
			Timestamp: 123,
			Hash:      999,
		},
		settingResponseSubject(123),
		`{"ok":true}`,
	)

	require.True(t, ok)

	select {
	case msg := <-responseCh:

		var resp api.SettingResponse

		require.NoError(
			t,
			json.Unmarshal(
				msg.Data,
				&resp,
			),
		)

		assert.Equal(
			t,
			`{"ok":true}`,
			resp.Response,
		)

		assert.EqualValues(
			t,
			123,
			resp.Timestamp,
		)

		assert.EqualValues(
			t,
			999,
			resp.Hash,
		)

	case <-time.After(
		2 * time.Second,
	):
		t.Fatal(
			"timeout waiting for response",
		)
	}
}

func TestNewSettingCommandHandler(
	t *testing.T,
) {
	handler := newSettingCommandHandler(
		testutil.TestLogger(t),
		&mockBusyManager{},
		&mockRouter{},
		nil,
		nil,
		dispatcher.MeasurementDispatcher{},
	)

	require.NotNil(
		t,
		handler,
	)

	require.NotNil(
		t,
		handler.router,
	)
}

func buildMockResponse(
	t *testing.T,
) *settingrouter.FalconSettingResponse {
	t.Helper()

	falconPort := testutil.NewTestKnobPort(t)
	defer falconPort.Close()

	q := testutil.NewVoltQuantity(t, 7)
	defer q.Close()

	resp, err := settingrouter.NewFalconSettingResponse(
		map[settingrouter.ConnectedPort]settingrouter.Quantity{
			{
				ConnectedPort: config.ConnectedPort{
					DeviceName: "P1",
				},
				FalconPort: falconPort,
			}: {
				Handle: q,
			},
		},
	)
	require.NoError(t, err)

	return resp
}

type mockRouter struct {
	resp *settingrouter.FalconSettingResponse
	err  error

	callCount int
}

func (m *mockRouter) Handle(
	_ *settingrouter.FalconSettingRequest,
) (*settingrouter.FalconSettingResponse, error) {
	m.callCount++

	if m.err != nil {
		return nil, m.err
	}

	return m.resp, nil
}

func TestHandler_ReceivesSettingCommand(
	t *testing.T,
) {
	server := testutil.RunNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(
		server.ClientURL(),
	)
	require.NoError(t, err)
	defer nc.Close()

	router := &mockRouter{
		resp: buildMockResponse(t),
	}
	defer router.resp.Close()

	portsCatalog := testutil.BuildTestConnectedPorts(t)

	cmd := api.SettingCommand{
		Request:   testutil.BuildRealFalconRequest(t),
		Timestamp: 999,
		Hash:      123,
	}

	handler := newSettingCommandHandler(
		testutil.TestLogger(t),
		&mockBusyManager{},
		router,
		nil,
		portsCatalog,
		dispatcher.MeasurementDispatcher{},
	)

	require.NoError(
		t,
		handler.Subscribe(nc),
	)
	defer handler.Unsubscribe()

	responseCh := make(
		chan *api.SettingResponse,
		1,
	)

	sub, err := nc.Subscribe(
		settingResponseSubject(999),
		func(msg *nats.Msg) {
			var resp api.SettingResponse

			if json.Unmarshal(
				msg.Data,
				&resp,
			) == nil {
				responseCh <- &resp
			}
		},
	)
	require.NoError(t, err)
	defer sub.Unsubscribe()

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	require.NoError(
		t,
		nc.Publish(
			SettingCommandSubject,
			data,
		),
	)

	require.NoError(
		t,
		nc.Flush(),
	)

	var resp *api.SettingResponse

	require.Eventually(
		t,
		func() bool {
			select {
			case resp = <-responseCh:
				return true
			default:
				return false
			}
		},
		time.Second,
		10*time.Millisecond,
	)

	assert.EqualValues(
		t,
		999,
		resp.Timestamp,
	)

	assert.EqualValues(
		t,
		123,
		resp.Hash,
	)

	assert.NotEmpty(
		t,
		resp.Response,
	)

	assert.Equal(
		t,
		1,
		router.callCount,
	)
}

type recordingSetter struct {
	calls int

	lastPort  settingrouter.ConnectedPort
	lastValue settingrouter.Quantity
}

func (r *recordingSetter) Name() string { return "test" }

func (r *recordingSetter) CanSet(
	settingrouter.ConnectedPort,
	settingrouter.Quantity,
) bool {
	return true
}

func (r *recordingSetter) Set(
	port settingrouter.ConnectedPort,
	value settingrouter.Quantity,
	_ *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	_ *config.ConnectedPorts,
) error {
	r.calls++
	r.lastPort = port
	copied, err := quantity.Copy(
		value.Handle,
	)
	if err != nil {
		return err
	}

	if r.lastValue.Handle != nil {
		_ = r.lastValue.Close()
	}

	r.lastValue = settingrouter.Quantity{
		Handle: copied,
	}

	return nil
}

type recordingGetter struct {
	result settingrouter.Quantity
}

func (r *recordingGetter) Name() string { return "test" }

func (r *recordingGetter) CanGet(
	settingrouter.ConnectedPort,
) bool {
	return true
}

func (r *recordingGetter) Get(
	settingrouter.ConnectedPort,
	*dispatcher.MeasurementDispatcher,
	config.WireMap,
	*config.ConnectedPorts,
) (settingrouter.Quantity, error) {
	return r.result, nil
}

func TestHandler_ReceivesSettingCommand_EndToEnd(
	t *testing.T,
) {
	server := testutil.RunNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(
		server.ClientURL(),
	)
	require.NoError(t, err)
	defer nc.Close()

	portsCatalog := testutil.BuildTestConnectedPorts(t)

	setter := &recordingSetter{}
	defer func() {
		if setter.lastValue.Handle != nil {
			_ = setter.lastValue.Close()
		}
	}()

	getterQuantity := settingrouter.Quantity{
		Handle: testutil.NewVoltQuantity(t, 7),
	}
	defer getterQuantity.Close()

	router := settingrouter.NewRouter(
		nil,
		nil,
		portsCatalog,
	)

	router.RegisterSetter(setter)

	router.RegisterGetter(
		&recordingGetter{
			result: getterQuantity,
		},
	)

	busy := &mockBusyManager{}

	handler := newSettingCommandHandler(
		testutil.TestLogger(t),
		busy,
		router,
		nil,
		portsCatalog,
		dispatcher.MeasurementDispatcher{},
	)

	require.NoError(
		t,
		handler.Subscribe(nc),
	)
	defer handler.Unsubscribe()

	responseCh := make(
		chan api.SettingResponse,
		1,
	)

	sub, err := nc.Subscribe(
		settingResponseSubject(999),
		func(msg *nats.Msg) {
			var resp api.SettingResponse

			if err := json.Unmarshal(
				msg.Data,
				&resp,
			); err == nil {
				responseCh <- resp
			}
		},
	)
	require.NoError(t, err)
	defer sub.Unsubscribe()

	cmd := api.SettingCommand{
		Request:   testutil.BuildRealFalconRequest(t),
		Timestamp: 999,
		Hash:      12345,
	}

	data, err := json.Marshal(cmd)
	require.NoError(t, err)

	require.NoError(
		t,
		nc.Publish(
			SettingCommandSubject,
			data,
		),
	)

	require.NoError(
		t,
		nc.Flush(),
	)

	var apiResp api.SettingResponse

	require.Eventually(
		t,
		func() bool {
			select {
			case apiResp = <-responseCh:
				return true
			default:
				return false
			}
		},
		2*time.Second,
		10*time.Millisecond,
	)

	assert.EqualValues(
		t,
		999,
		apiResp.Timestamp,
	)

	assert.EqualValues(
		t,
		12345,
		apiResp.Hash,
	)

	assert.NotEmpty(
		t,
		apiResp.Response,
	)

	assert.Equal(
		t,
		1,
		setter.calls,
	)

	require.Len(
		t,
		busy.calls,
		2,
	)

	assert.True(
		t,
		busy.calls[0],
	)

	assert.False(
		t,
		busy.calls[1],
	)

	setterValue, err := setter.lastValue.Value()
	require.NoError(t, err)

	assert.Equal(
		t,
		5.0,
		setterValue,
	)

	setterUnit, err := setter.lastValue.UnitSymbol()
	require.NoError(t, err)

	assert.Equal(
		t,
		"V",
		setterUnit,
	)

	falconResp, err := settingresponse.FromJSON(
		apiResp.Response,
	)
	require.NoError(t, err)
	defer falconResp.Close()

	getters, err := falconResp.Getters()
	require.NoError(t, err)
	defer getters.Close()

	size, err := getters.Size()
	require.NoError(t, err)

	require.Equal(
		t,
		uint64(1),
		size,
	)

	keys, err := getters.Keys()
	require.NoError(t, err)
	defer keys.Close()

	key, err := keys.At(0)
	require.NoError(t, err)
	defer key.Close()

	value, err := getters.At(key)
	require.NoError(t, err)
	defer value.Close()

	actual, err := value.Value()
	require.NoError(t, err)

	assert.Equal(
		t,
		7.0,
		actual,
	)

	unit, err := value.Unit()
	require.NoError(t, err)
	defer unit.Close()

	symbol, err := unit.Symbol()
	require.NoError(t, err)

	assert.Equal(
		t,
		"V",
		symbol,
	)

	defaultName, err := key.DefaultName()
	require.NoError(t, err)

	assert.Equal(
		t,
		"P1",
		defaultName,
	)

	instrumentName, err := key.InstrumentName()
	require.NoError(t, err)

	assert.Equal(
		t,
		"Source1",
		instrumentName,
	)
}
