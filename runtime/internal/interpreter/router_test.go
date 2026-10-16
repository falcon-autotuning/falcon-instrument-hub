//go:build cgo

package interpreter

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/devicestate"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDispatcher struct{}

func (m *mockDispatcher) RunAll(
	[]DispatchRequest,
) []DispatchResult {
	return nil
}

var _ MeasurementRunner = (*mockDispatcher)(nil)

type mockHandler struct {
	name string

	canHandleResult bool
	canHandleErr    error

	handleResponse MeasurementResponse
	handleErr      error

	canHandleCalls int
	handleCalls    int

	updatePort bool
	writeDB    bool

	lastRequest MeasurementRequest
}

func (m *mockHandler) Name() string {
	return m.name
}

func (m *mockHandler) CanHandle(
	req MeasurementRequest,
	ports *config.ConnectedPorts,
	scriptDir string,
) (bool, error) {
	m.canHandleCalls++
	m.lastRequest = req

	return m.canHandleResult, m.canHandleErr
}

func (m *mockHandler) Handle(
	req MeasurementRequest,
	dispatcher MeasurementRunner,
	ports *config.ConnectedPorts,
	tracker StateUpdater,
	recorder MeasurementRecorder,
) (MeasurementResponse, error) {
	m.handleCalls++
	m.lastRequest = req

	if m.updatePort {
		_ = tracker.UpdatePort(
			"P1",
			falconcore.Quantity{},
		)
	}

	if m.writeDB {
		_, err := recorder.Insert(
			measurementdb.InsertEntry{
				CoreEntry: measurementdb.CoreEntry{
					RunID:           "run-1",
					MeasurementName: "test",
				},
			},
			measurementdb.DatabaseEntry{},
		)
		if err != nil {
			panic(err)
		}
	}

	return m.handleResponse, m.handleErr
}

var _ MeasurementHandler = (*mockHandler)(nil)

func stubMeasurementInsert(t *testing.T) {
	t.Helper()

	old := measurementInsert

	measurementInsert = func(
		measurementdb.InsertEntry,
		measurementdb.DatabaseEntry,
	) (measurementdb.CatalogEntry, error) {
		return measurementdb.CatalogEntry{}, nil
	}

	t.Cleanup(func() {
		measurementInsert = old
	})
}

func setupDeviceState(t *testing.T) {
	t.Helper()

	devicestate.Close()

	err := devicestate.Startup(
		falconcore.DeviceVoltageStates{
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

func setupMeasurementDB(t *testing.T) {
	t.Helper()

	measurementdb.Close()

	dir := t.TempDir()

	err := measurementdb.Startup(
		filepath.Join(dir, "catalog.sqlite"),
		filepath.Join(dir, "hdf5"),
		"test",
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = measurementdb.Close()
	})
}

func setupDependencies(t *testing.T) {
	t.Helper()

	setupDeviceState(t)
	setupMeasurementDB(t)
}

func TestRouter_Handle_UniqueMatchingHandlerWins(t *testing.T) {
	stubMeasurementInsert(t)
	setupDependencies(t)

	req := MeasurementRequest{}
	resp := MeasurementResponse{}

	first := &mockHandler{
		name:            "first",
		canHandleResult: false,
	}

	second := &mockHandler{
		name:            "second",
		canHandleResult: true,
		handleResponse:  resp,
		updatePort:      true,
		writeDB:         true,
	}

	third := &mockHandler{
		name:            "third",
		canHandleResult: false,
	}

	router := &Router{
		dispatcher: &mockDispatcher{},
		ports:      &config.ConnectedPorts{},
		handlers: []MeasurementHandler{
			first,
			second,
			third,
		},
	}

	result, err := router.Handle(req)

	require.NoError(t, err)
	assert.Equal(t, resp, result)

	assert.Equal(t, 1, first.canHandleCalls)
	assert.Equal(t, 0, first.handleCalls)

	assert.Equal(t, 1, second.canHandleCalls)
	assert.Equal(t, 1, second.handleCalls)

	assert.Equal(t, 0, third.canHandleCalls)
	assert.Equal(t, 0, third.handleCalls)
}

func TestRouter_Handle_FirstMatchWins(t *testing.T) {
	stubMeasurementInsert(t)
	setupDependencies(t)

	first := &mockHandler{
		name:            "first",
		canHandleResult: true,
		handleResponse:  MeasurementResponse{},
		updatePort:      true,
		writeDB:         true,
	}

	second := &mockHandler{
		name:            "second",
		canHandleResult: true,
	}

	router := &Router{
		dispatcher: &mockDispatcher{},
		ports:      &config.ConnectedPorts{},
		handlers: []MeasurementHandler{
			first,
			second,
		},
	}

	_, err := router.Handle(MeasurementRequest{})

	require.NoError(t, err)

	assert.Equal(t, 1, first.canHandleCalls)
	assert.Equal(t, 1, first.handleCalls)

	assert.Equal(t, 0, second.canHandleCalls)
	assert.Equal(t, 0, second.handleCalls)
}

func TestRouter_Handle_CanHandleError(t *testing.T) {
	expectedErr := errors.New("bad request")

	handler := &mockHandler{
		name:         "failing-handler",
		canHandleErr: expectedErr,
	}

	router := &Router{
		dispatcher: &mockDispatcher{},
		handlers: []MeasurementHandler{
			handler,
		},
	}

	_, err := router.Handle(
		MeasurementRequest{},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)

	assert.Contains(
		t,
		err.Error(),
		`error while evaluating handler "failing-handler"`,
	)

	assert.Equal(t, 1, handler.canHandleCalls)
	assert.Equal(t, 0, handler.handleCalls)
}

func TestRouter_Handle_HandlerError(t *testing.T) {
	setupDependencies(t)

	expectedErr := errors.New("measurement failed")

	handler := &mockHandler{
		name:            "matching-handler",
		canHandleResult: true,
		handleErr:       expectedErr,
	}

	router := &Router{
		dispatcher: &mockDispatcher{},
		handlers: []MeasurementHandler{
			handler,
		},
	}

	_, err := router.Handle(
		MeasurementRequest{},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)

	assert.Equal(t, 1, handler.canHandleCalls)
	assert.Equal(t, 1, handler.handleCalls)
}

func TestRouter_Handle_NoMatchingHandlers(t *testing.T) {
	router := &Router{
		dispatcher: &mockDispatcher{},
		handlers: []MeasurementHandler{
			&mockHandler{
				name:            "handler-1",
				canHandleResult: false,
			},
			&mockHandler{
				name:            "handler-2",
				canHandleResult: false,
			},
		},
	}

	_, err := router.Handle(
		MeasurementRequest{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"no measurement handler matched request",
	)
}

func TestRouter_Handle_HandlerUpdatesState(t *testing.T) {
	stubMeasurementInsert(t)
	setupDependencies(t)

	resp := MeasurementResponse{}

	handler := &mockHandler{
		name:            "handler",
		canHandleResult: true,
		handleResponse:  resp,
		updatePort:      true,
		writeDB:         true,
	}

	router := &Router{
		handlers: []MeasurementHandler{
			handler,
		},
	}

	out, err := router.Handle(
		MeasurementRequest{},
	)

	require.NoError(t, err)
	assert.Equal(t, resp, out)
}

func TestRouter_Handle_NoStateUpdate(t *testing.T) {
	stubMeasurementInsert(t)
	setupDependencies(t)

	handler := &mockHandler{
		name:            "handler",
		canHandleResult: true,
		updatePort:      false,
		writeDB:         true,
	}

	router := &Router{
		handlers: []MeasurementHandler{
			handler,
		},
	}

	_, err := router.Handle(
		MeasurementRequest{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"completed without updating device state",
	)
}

func TestTrackedStateUpdater_UpdatePort(t *testing.T) {
	setupDependencies(t)

	tracker := &trackedStateUpdater{
		manager: devicestate.Manager(),
	}

	err := tracker.UpdatePort(
		"P1",
		falconcore.Quantity{
			Value: 1,
		},
	)

	require.NoError(t, err)

	assert.Equal(t, 1, tracker.updates)
}

func TestRouter_Handle_ManagerNotInitialized(t *testing.T) {
	devicestate.Close()

	handler := &mockHandler{
		name:            "handler",
		canHandleResult: true,
	}

	router := &Router{
		handlers: []MeasurementHandler{
			handler,
		},
	}

	_, err := router.Handle(
		MeasurementRequest{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"device state manager not initialized",
	)

	assert.Equal(t, 1, handler.canHandleCalls)
	assert.Equal(t, 0, handler.handleCalls)
}

func TestRouter_Handle_DatabaseNotInitialized(t *testing.T) {
	setupDeviceState(t)

	measurementdb.Close()

	handler := &mockHandler{
		name:            "handler",
		canHandleResult: true,
	}

	router := &Router{
		handlers: []MeasurementHandler{
			handler,
		},
	}

	_, err := router.Handle(
		MeasurementRequest{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"measurement database not initialized",
	)

	assert.Equal(t, 1, handler.canHandleCalls)
	assert.Equal(t, 0, handler.handleCalls)
}

func TestRouter_Handle_NoDatabaseWrite(t *testing.T) {
	setupDependencies(t)

	handler := &mockHandler{
		name:            "handler",
		canHandleResult: true,
		updatePort:      true,
		writeDB:         false,
	}

	router := &Router{
		handlers: []MeasurementHandler{
			handler,
		},
	}

	_, err := router.Handle(
		MeasurementRequest{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"completed without writing measurement data",
	)
}

func TestTrackedMeasurementRecorder_Insert(
	t *testing.T,
) {
	old := measurementInsert
	defer func() {
		measurementInsert = old
	}()

	measurementInsert = func(
		measurementdb.InsertEntry,
		measurementdb.DatabaseEntry,
	) (measurementdb.CatalogEntry, error) {
		return measurementdb.CatalogEntry{}, nil
	}
	setupMeasurementDB(t)

	recorder := &trackedMeasurementRecorder{}

	_, err := recorder.Insert(
		measurementdb.InsertEntry{
			CoreEntry: measurementdb.CoreEntry{
				RunID:           "run-1",
				MeasurementName: "test",
			},
		},
		measurementdb.DatabaseEntry{},
	)

	require.NoError(t, err)

	assert.Equal(t, 1, recorder.Writes())
}

func TestTrackedMeasurementRecorder_InsertError(
	t *testing.T,
) {
	old := measurementInsert
	defer func() {
		measurementInsert = old
	}()

	measurementInsert = func(
		measurementdb.InsertEntry,
		measurementdb.DatabaseEntry,
	) (measurementdb.CatalogEntry, error) {
		return measurementdb.CatalogEntry{},
			errors.New("insert failed")
	}
	setupMeasurementDB(t)

	recorder := &trackedMeasurementRecorder{}

	_, err := recorder.Insert(
		measurementdb.InsertEntry{},
		measurementdb.DatabaseEntry{},
	)

	require.Error(t, err)

	assert.Equal(t, 0, recorder.Writes())
}

func TestRouter_Handle_CanHandleErrorIncludesHandlerName(
	t *testing.T,
) {
	expected := errors.New("boom")

	handler := &mockHandler{
		name:         "test-handler",
		canHandleErr: expected,
	}

	router := &Router{
		handlers: []MeasurementHandler{handler},
	}

	_, err := router.Handle(
		MeasurementRequest{},
	)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)

	assert.Contains(
		t,
		err.Error(),
		`test-handler`,
	)
}
