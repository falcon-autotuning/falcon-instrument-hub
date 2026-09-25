package handlers

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
)

// MockBusyManager implements BusyManager interface for testing
type MockBusyManager struct {
	isBusy bool
	mutex  sync.RWMutex
}

func (m *MockBusyManager) SetIsBusy(busy bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.isBusy = busy
}

func (m *MockBusyManager) IsBusy() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.isBusy
}

// mockDispatcher implements MeasurementDispatcher for testing.
type mockDispatcher struct {
	results    []ResolvedCallResult
	err        error
	calls      int
	scriptName string
}

func (m *mockDispatcher) RunMeasurement(scriptName string, globals map[string]interface{}, typeManifest map[string]interface{}) ([]ResolvedCallResult, error) {
	m.calls++
	m.scriptName = scriptName
	return m.results, m.err
}

// setupMeasureHandler creates a MeasureCommandHandler wired to an in-process
// NATS server and returns the handler plus a connected NATS client.
func setupMeasureHandler(t *testing.T, dispatcher MeasurementDispatcher) (*MeasureCommandHandler, *nats.Conn) {
	t.Helper()

	natsServer := runNATSServer(t)
	t.Cleanup(func() { natsServer.Shutdown() })

	nc, err := nats.Connect(natsServer.ClientURL())
	require.NoError(t, err)
	t.Cleanup(func() { nc.Close() })

	tempDir := t.TempDir()

	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() { logger.Close() })

	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}
	instrumentHandler, err := instrument.NewHandler(
		logger,
		cfg,
	)
	require.NoError(t, err)

	handler := NewMeasureCommandHandler(
		logger,
		instrumentHandler,
		&MockBusyManager{},
		dispatcher,
		nil,
		defaultMeasurementMetadataRegistry(),
	)
	return handler, nc
}

func TestMeasureCommandHandler_HandleMessage(t *testing.T) {
	handler, nc := setupMeasureHandler(t, &mockDispatcher{})

	err := handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	t.Run("invalid_json", func(t *testing.T) {
		err = nc.Publish(MeasureCommandSubject, []byte("invalid json"))
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("empty_request", func(t *testing.T) {
		cmd := api.MeasureCommand{
			Timestamp: 0,
			Hash:      0,
			Request:   "",
		}
		data, err := json.Marshal(cmd)
		require.NoError(t, err)
		err = nc.Publish(MeasureCommandSubject, data)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	})
}

func TestMeasureCommandHandler_EdgeCases(t *testing.T) {
	handler, nc := setupMeasureHandler(t, &mockDispatcher{})

	t.Run("subscribe_and_unsubscribe", func(t *testing.T) {
		err := handler.Subscribe(nc)
		assert.NoError(t, err, "Should subscribe successfully")

		err = handler.Unsubscribe()
		assert.NoError(t, err, "Should unsubscribe successfully")

		err = handler.Unsubscribe()
		assert.NoError(t, err, "Should handle double unsubscribe gracefully")
	})
}

func TestMeasurementResponseSubject(t *testing.T) {
	assert.Equal(
		t,
		"FALCON.MEASURE_RESPONSE.12345",
		measurementResponseSubject(12345),
	)
}

func TestResolveScriptTargetUsesScriptCapability(t *testing.T) {
	handler := &MeasureCommandHandler{
		instrumentHandler: &instrument.Handler{
			PortConnections: []ports.ConnectedPort{
				{
					DeviceName:     "P1",
					InstrumentName: "Source1",
					ChannelIndex:   4,
					IoTypeName:     "voltage",
					Role:           "output",
				},
				{
					DeviceName:     "P1",
					InstrumentName: "Source1",
					ChannelIndex:   4,
					IoTypeName:     "measured_voltage",
					Role:           "input",
				},
				{
					DeviceName:     "O1",
					InstrumentName: "Meter1",
					ChannelIndex:   1,
					IoTypeName:     "sample_rate",
					Role:           "setting",
				},
			},
		},
	}

	setterTarget, err := handler.resolveScriptTargetForGate(
		"set_voltage",
		"setter",
		"P1",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "Source1", setterTarget.id)
	assert.Equal(t, 4, setterTarget.channel)
	require.NotNil(t, setterTarget.connectedPort)
	assert.Equal(t, "voltage", setterTarget.connectedPort.IoTypeName)
	assert.Equal(t, "output", setterTarget.connectedPort.Role)

	getterTarget, err := handler.resolveScriptTargetForGate(
		"set_sample_rate",
		"getter",
		"O1",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "Meter1", getterTarget.id)
	assert.Equal(t, 1, getterTarget.channel)
	require.NotNil(t, getterTarget.connectedPort)
	assert.Equal(t, "sample_rate", getterTarget.connectedPort.IoTypeName)
	assert.Equal(t, "setting", getterTarget.connectedPort.Role)
}

func TestResolveScriptTargetFallsBackToWireMapForUnknownScript(t *testing.T) {
	handler := &MeasureCommandHandler{}

	target, err := handler.resolveScriptTargetForGate(
		"custom_script",
		"getter",
		"P2",
		map[string]config.InstrumentConnection{
			"P2": "Source1.analog.5",
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "Source1", target.id)
	assert.Equal(t, 5, target.channel)
	assert.Nil(t, target.connectedPort)
}
