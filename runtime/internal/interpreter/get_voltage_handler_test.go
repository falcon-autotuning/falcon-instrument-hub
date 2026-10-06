//go:build cgo && falcon_core

package interpreter

import (
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listwaveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/labelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func scalarMeterRequest(
	t *testing.T,
	instrumentType instrument.Instrument,
	unit *symbolunit.Handle,
) *FalconMeasurementRequest {
	t.Helper()

	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer conn.Close()

	getter, err := instrumentport.NewMeter(
		"P1",
		"Source1",
		conn,
		instrumentType,
		unit,
		"test meter",
	)
	require.NoError(t, err)
	defer getter.Close()

	getters, err := ports.New([]*instrumentport.Handle{getter})
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

	timeDomain, err := labelleddomain.NewFromPort(0, 1, clock, true, true)
	require.NoError(t, err)
	defer timeDomain.Close()

	request, err := measurementrequest.New(
		"descriptive text is not used for routing",
		waveforms,
		getters,
		transforms,
		timeDomain,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, request.Close()) })

	return &FalconMeasurementRequest{handle: request}
}

type getVoltageTestExecutor struct {
	scriptPath string
	variables  []instrumentserver.MeasureVariable
}

func (e *getVoltageTestExecutor) Measure(
	scriptPath string,
	variables []instrumentserver.MeasureVariable,
) ([]instrumentserver.CallResult, error) {
	e.scriptPath = scriptPath
	e.variables = variables
	return []instrumentserver.CallResult{{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    4,
		Verb:       getVoltageCommand,
		Return: []instrumentserver.ReturnValue{{
			Name:  "measured_voltage",
			Value: instrumentserver.VariableValue{Value: 0.125},
			Unit:  "V",
		}},
	}}, nil
}

type getVoltageTestBuffers struct{}

func (*getVoltageTestBuffers) RegisterBuffer(
	string,
	string,
) error {
	return nil
}

func TestGetVoltageCanHandleScalarVoltageMeter(t *testing.T) {
	volts, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer volts.Close()

	request := scalarMeterRequest(t, instrument.DcVoltageSource, volts)
	matches, err := (&getVoltageHandler{}).CanHandle(request)

	require.NoError(t, err)
	assert.True(t, matches)
}

func TestGetVoltageRejectsNonVoltageMeter(t *testing.T) {
	amps, err := symbolunit.NewAmpere()
	require.NoError(t, err)
	defer amps.Close()

	request := scalarMeterRequest(t, instrument.DcCurrentSource, amps)
	matches, err := (&getVoltageHandler{}).CanHandle(request)

	require.NoError(t, err)
	assert.False(t, matches)
}

func TestGetVoltageDispatchesInstrumentTarget(t *testing.T) {
	volts, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer volts.Close()

	request := scalarMeterRequest(t, instrument.DcVoltageSource, volts)
	executor := &getVoltageTestExecutor{}
	dispatcher := NewMeasurementDispatcher(
		executor,
		&getVoltageTestBuffers{},
		"/scripts",
	)
	connected := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{{
			PortEntry: config.PortEntry{
				InstrumentName: "Source1",
				ChannelGroup:   "analog",
				Channel:        4,
				InstrumentType: instrument.DcVoltageSource,
				Role:           porttype.PortTypeMeter,
				Access:         access.Read,
				Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
				Unit:           "V",
			},
			PortName:   "Source1.analog.4.measured_voltage",
			DeviceName: "P1",
		}},
	}

	response, err := (&getVoltageHandler{}).Handle(
		request,
		dispatcher,
		nil,
		connected,
	)
	require.NoError(t, err)
	require.NotNil(t, response)
	defer response.Close()

	assert.Equal(
		t,
		filepath.Join("/scripts", "get_voltage.lua"),
		executor.scriptPath,
	)
	require.Len(t, executor.variables, 1)
	assert.Equal(t, "getter", executor.variables[0].Name)

	serialized, ok := executor.variables[0].Value.Value.(instrumentserver.InstrumentTarget)
	require.True(t, ok)
	expected, err := (instrumenttarget.Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    4,
	}).Serialize()
	require.NoError(t, err)
	assert.Equal(t, expected, string(serialized))
}
