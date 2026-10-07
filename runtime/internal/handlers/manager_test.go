package handlers

import (
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagerOperations(t *testing.T) {
	manager := &Manager{}

	for _, includeStatus := range []bool{false, true} {
		var names []string

		for _, op := range manager.getHandlerOperations(includeStatus) {
			names = append(names, op.name)
		}

		expected := []string{
			"device config handler",
			"measure command handler",
			"port request handler",
			"setting command handler",
		}

		if includeStatus {
			expected = append(expected, "status handler")
		}

		assert.Equal(t, expected, names)
	}
}

func TestManagerBusyState(t *testing.T) {
	m := &Manager{}

	assert.False(t, m.IsBusy())

	m.SetIsBusy(true)
	assert.True(t, m.IsBusy())

	m.SetIsBusy(false)
	assert.False(t, m.IsBusy())
}

func TestManagerStartStatus(t *testing.T) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	logger, err := logging.NewLogger(t.TempDir())
	require.NoError(t, err)
	defer logger.Close()

	m := &Manager{
		logger: logger,
		nc:     nc,
		statusHandler: NewStatusHandler(
			logger,
		),
	}

	require.NoError(t, m.StartStatus())

	require.NoError(t, m.statusHandler.Stop())
}

type mockMeasurementClient struct{}

func (m *mockMeasurementClient) Measure(
	string,
	[]instrumentserver.MeasureVariable,
) ([]instrumentserver.CallResult, error) {
	return nil, nil
}

type mockConnectedPortsBuilder struct{}

func (b *mockConnectedPortsBuilder) NewConnectedPorts(
	instrumentAPIPaths []string,
	wiremap *config.WireMap,
) (*config.ConnectedPorts, error) {
	return nil, nil
}

func (m *mockMeasurementClient) ReleaseBuffer(
	string,
) error {
	return nil
}

func TestManagerStartCoreHandlers(t *testing.T) {
	server := runNATSServer(t)
	defer server.Shutdown()

	nc, err := nats.Connect(server.ClientURL())
	require.NoError(t, err)
	defer nc.Close()

	logger, err := logging.NewLogger(t.TempDir())
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(
		"{}",
		[]config.WiremapEntry{},
		nil,
		t.TempDir(),
		logger,
		nc,
		&mockMeasurementClient{},
		nil,
	)

	require.NoError(t, manager.StartCoreHandlers())

	require.NotNil(t, manager.deviceConfigHandler)
	require.NotNil(t, manager.portRequestHandler)

	require.NoError(t, manager.Stop())
}
