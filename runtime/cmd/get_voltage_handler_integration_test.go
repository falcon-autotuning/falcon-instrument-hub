//go:build cgo && falcon_core

package main

import (
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listwaveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	falconports "github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/labelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/interpreter"
	hubports "github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type getVoltageExecutor struct {
	scriptPath string
	variables  []instrumentserver.MeasureVariable
}

func (e *getVoltageExecutor) Measure(
	scriptPath string,
	variables []instrumentserver.MeasureVariable,
) ([]instrumentserver.CallResult, error) {
	e.scriptPath = scriptPath
	e.variables = variables
	return []instrumentserver.CallResult{{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    4,
		Verb:       "GET_VOLTAGE",
		Return: []instrumentserver.ReturnValue{{
			Name:  "measured_voltage",
			Value: instrumentserver.VariableValue{Value: 0.125},
			Unit:  "V",
		}},
	}}, nil
}

type getVoltageBufferRegistrar struct{}

func (*getVoltageBufferRegistrar) RegisterBuffer(string, string) error { return nil }

func getVoltageRequest(t *testing.T) *interpreter.FalconMeasurementRequest {
	t.Helper()

	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)
	defer conn.Close()
	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer unit.Close()
	port, err := instrumentport.NewMeter(
		"Source1.analog",
		conn,
		"voltmeter",
		unit,
		"test getter",
	)
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

	request, err := measurementrequest.New(
		"test measurement",
		"get_voltage",
		waveforms,
		getters,
		transforms,
		domain,
	)
	require.NoError(t, err)
	defer request.Close()
	serialized, err := request.ToJSON()
	require.NoError(t, err)

	falconRequest, err := interpreter.NewFalconMeasurementRequestFromJSON(serialized)
	require.NoError(t, err)
	return falconRequest
}

func TestGetVoltageRoutesSerializedCallStackToLua(t *testing.T) {
	executor := &getVoltageExecutor{}
	dispatcher := interpreter.NewMeasurementDispatcher(
		executor,
		&getVoltageBufferRegistrar{},
		"/scripts",
	)
	connected := &hubports.ConnectedPorts{
		AllConnections: []hubports.ConnectedPort{{
			DeviceName:     "P1",
			InstrumentName: "Source1",
			ChannelName:    "analog",
			ChannelIndex:   4,
			IoTypeName:     "measured_voltage",
			InstrumentType: "voltmeter",
			Role:           "input",
			Unit:           "V",
		}},
	}
	router := interpreter.NewRouter(dispatcher, nil, connected)
	request := getVoltageRequest(t)
	defer request.Close()

	response, err := router.Handle(request)
	require.NoError(t, err)
	require.NotNil(t, response)
	defer response.Close()
	responseJSON, err := response.ToJSON()
	require.NoError(t, err)
	assert.NotEmpty(t, responseJSON)

	assert.Equal(t, filepath.Join("/scripts", "get_voltage.lua"), executor.scriptPath)
	require.Len(t, executor.variables, 1)
	assert.Equal(t, "getter", executor.variables[0].Name)
	stack, ok := executor.variables[0].Value.Value.(instrumentserver.CallStack)
	require.True(t, ok)
	assert.Equal(t, instrumentserver.CallStack("Source1|analog|4|GET_VOLTAGE"), stack)
}
