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
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurements"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/serverinterpreter"
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
	results []serverinterpreter.ResolvedCallResult
	err     error
}

func (m *mockDispatcher) RunMeasurement(scriptName string, globals map[string]interface{}, typeManifest map[string]interface{}) ([]serverinterpreter.ResolvedCallResult, error) {
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

	measurementManager, err := measurements.NewManager(
		tempDir+"/data",
		tempDir+"/test.db",
	)
	require.NoError(t, err)
	t.Cleanup(func() { measurementManager.Close() })

	cfg := &config.Config{
		DeviceConfig: &config.DeviceConfig{},
		WireMap:      &config.WireMap{},
	}
	instrumentHandler, err := instrument.NewHandler(
		logger,
		natsServer.ClientURL(),
		nc,
		cfg,
	)
	require.NoError(t, err)

	handler := NewMeasureCommandHandler(
		logger,
		measurementManager,
		instrumentHandler,
		&MockBusyManager{},
		dispatcher,
		nil,
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

