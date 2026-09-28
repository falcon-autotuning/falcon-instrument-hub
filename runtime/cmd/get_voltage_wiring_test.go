//go:build cgo && falcon_core

package main

import (
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	measure "github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/measure"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRuntime_AttachesISSAndPassesInstrumentAPIPaths(t *testing.T) {
	client := &FakeISSClient{status: true}
	handler := &FakeHandlerManager{}
	apiPaths := []string{"source-api.yml", "meter-api.yml"}

	cfg := &HubConfig{
		NATSURL:                "nats://localhost:4222",
		InstrumentAPIPaths:     apiPaths,
		UserMeasurementLuasDir: "/tmp/scripts",
		RuntimePaths: RuntimePaths{
			Logs: t.TempDir(),
		},
		InstrumentServer: InstrumentServerConfig{
			AutoStart: false,
			RPCPort:   8555,
		},
	}

	var dispatcher measure.MeasurementClient
	var passedAPIPaths []string
	deps := RuntimeDependencies{
		newISSClient: func(string, int, string) ISSClient {
			return client
		},
		newNATSManager: func(string) (NATSManager, error) {
			return &FakeNATSManager{}, nil
		},
		newLogger: func(string) (*logging.Logger, error) {
			return logging.NewLogger(t.TempDir())
		},
		newConfig: func(string) (string, error) {
			return "{}", nil
		},
		newWiremap: func(string, string) (*config.WireMap, error) {
			return &config.WireMap{}, nil
		},
		newHandlerManager: func(
			_ string,
			_ *config.WireMap,
			instrumentAPIPaths []string,
			_ string,
			_ *logging.Logger,
			_ *nats.Conn,
			measurementClient measure.MeasurementClient,
		) HandlerManager {
			passedAPIPaths = append([]string{}, instrumentAPIPaths...)
			dispatcher = measurementClient
			return handler
		},
	}

	runtime, err := deps.NewRuntime(cfg)
	require.NoError(t, err)
	require.NotNil(t, runtime)

	assert.Equal(t, apiPaths, passedAPIPaths)
	assert.Same(t, client, dispatcher)
	assert.Nil(t, runtime.issProcess)
	assert.True(t, handler.coreStarted)
	assert.True(t, handler.statusStarted)

	runtime.Close()
	assert.True(t, client.closeCalled)
	assert.False(t, client.stopDaemonCalled)
}
