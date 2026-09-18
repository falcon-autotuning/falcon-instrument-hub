//go:build cgo && falcon_core

package handlers

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listwaveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	falconports "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/labelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeasurementNameIsRequiredBeforeDispatch(t *testing.T) {
	for _, name := range []string{"", " \t\n", " get_voltage "} {
		t.Run(name, func(t *testing.T) {
			logger, err := logging.NewLogger(t.TempDir())
			require.NoError(t, err)
			t.Cleanup(func() { logger.Close() })
			dispatcher := &mockDispatcher{err: errors.New("stop after observing dispatch")}
			busy := &MockBusyManager{}
			handler := NewMeasureCommandHandler(logger, &instrument.Handler{
				PortConnections: []ports.ConnectedPort{{
					DeviceName: "P1", InstrumentName: "Source1", ChannelIndex: 1,
					IoTypeName: "measured_voltage", Role: "input",
				}},
			}, busy, dispatcher, nil, defaultMeasurementMetadataRegistry())
			cmd := api.MeasureCommand{Request: measurementRequestJSONForName(t, name)}
			data, err := json.Marshal(cmd)
			require.NoError(t, err)
			handler.handleMessage(&nats.Msg{Data: data})
			assert.False(t, busy.IsBusy())
			require.NoError(t, logger.Close())
			log, err := os.ReadFile(logger.GetLogPath())
			require.NoError(t, err)
			if name == " get_voltage " {
				assert.Equal(t, 1, dispatcher.calls)
				assert.Equal(t, "get_voltage", dispatcher.scriptName)
				assert.NotContains(t, string(log), "measurement_name is required")
			} else {
				assert.Zero(t, dispatcher.calls)
				assert.Contains(t, string(log), "measurement_name is required")
			}
		})
	}
}

// Use the same typed request serialization consumed by controller measurements.
func measurementRequestJSONForName(t *testing.T, name string) string {
	t.Helper()
	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer conn.Close()
	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer unit.Close()
	port, err := instrumentport.NewMeter("Source1.analog", conn, "voltmeter", unit, "test getter")
	require.NoError(t, err)
	defer port.Close()
	getters, err := falconports.New([]*instrumentport.Handle{port})
	require.NoError(t, err)
	defer getters.Close()
	waveforms, err := listwaveform.NewEmpty()
	require.NoError(t, err)
	defer waveforms.Close()
	transforms, err := mapinstrumentportporttransform.NewEmpty()
	require.NoError(t, err)
	defer transforms.Close()
	clock, err := instrumentport.NewExecutionClock()
	require.NoError(t, err)
	defer clock.Close()
	domain, err := labelleddomain.NewFromPort(0, 1, clock, true, true)
	require.NoError(t, err)
	defer domain.Close()
	request, err := measurementrequest.New("test measurement", name, waveforms, getters, transforms, domain)
	require.NoError(t, err)
	defer request.Close()
	serialized, err := request.ToJSON()
	require.NoError(t, err)
	return serialized
}
