package interpreter

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

type mockSettingsReader struct {
	response falconcore.SettingResponse
	err      error

	callCount int
	lastReq   falconcore.SettingRequest
}

func (m *mockSettingsReader) Handle(
	req falconcore.SettingRequest,
) (falconcore.SettingResponse, error) {
	m.callCount++
	m.lastReq = req

	if m.err != nil {
		return falconcore.SettingResponse{}, m.err
	}

	return m.response, nil
}

func TestInstrumentPortFromConnectedPort(t *testing.T) {
	in := config.ConnectedPort{
		PortName: "gate1",
		PortEntry: config.PortEntry{
			InstrumentName: "dac",
			InstrumentType: instrument.Instrument(123),
			Description:    "gate voltage",
			Unit:           falconcore.Volt,
			Access:         access.Readwrite,
			Scope:          scope.Global,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
			Role:           porttype.PortTypeKnob,
		},
		Handle: falconcore.Connection{Name: "P1", Type: falconcore.PlungerGate},
	}

	out := InstrumentPortFromConnectedPort(in)

	assert.Equal(
		t,
		falconcore.PortName("gate1"),
		out.DefaultName,
	)

	assert.Equal(
		t,
		falconcore.InstrumentName("dac"),
		out.InstrumentName,
	)

	assert.Equal(
		t,
		in.InstrumentType,
		out.InstrumentType,
	)

	assert.Equal(
		t,
		in.Description,
		out.Description,
	)

	assert.Equal(
		t,
		in.Unit,
		out.Unit,
	)

	assert.Equal(
		t,
		in.Handle,
		out.PseudoName,
	)

	assert.Equal(
		t,
		in.Access,
		out.Access,
	)

	assert.Equal(
		t,
		in.Scope,
		out.Scope,
	)

	assert.Equal(
		t,
		in.Characteristic,
		out.Characteristic,
	)

	assert.Equal(
		t,
		in.Role,
		out.PortType,
	)
}

func TestSourceVoltagePorts_Empty(t *testing.T) {
	ports := &config.ConnectedPorts{}

	out := sourceVoltagePorts(ports)

	assert.Empty(t, out)
}

func TestSourceVoltagePorts_FiltersCorrectly(t *testing.T) {
	source := config.ConnectedPort{
		PortName: "gate1",
		PortEntry: config.PortEntry{
			InstrumentName: "dac",
			InstrumentType: instrument.Instrument(123),
			Description:    "gate voltage",
			Unit:           falconcore.Volt,
			Access:         access.Readwrite,
			Scope:          scope.Global,
			Characteristic: instrumentcharacteristic.
				InstrumentCharacteristicSourceVoltage,
			Role: porttype.PortTypeKnob,
		},
		Handle: falconcore.Connection{
			Name: "P1",
			Type: falconcore.PlungerGate,
		},
	}

	notSource := config.ConnectedPort{
		PortName: "meter1",
		PortEntry: config.PortEntry{
			InstrumentName: "dmm",
			InstrumentType: instrument.Instrument(456),
			Description:    "measurement",
			Unit:           falconcore.Volt,
			Access:         access.Read,
			Scope:          scope.Global,
			Characteristic: instrumentcharacteristic.
				InstrumentCharacteristicNumberOfSamples,
			Role: porttype.PortTypeMeter,
		},
		Handle: falconcore.Connection{
			Name: "M1",
			Type: falconcore.Ohmic,
		},
	}

	ports := &config.ConnectedPorts{
		Knobs: []config.ConnectedPort{
			source,
			notSource,
		},
	}

	out := sourceVoltagePorts(ports)

	require.Len(t, out, 1)

	expected := InstrumentPortFromConnectedPort(source)

	assert.Equal(t, expected, out[0])
}

func TestCartesianMeasurementHandler_ReadSourceVoltages(t *testing.T) {
	expected := map[falconcore.InstrumentPort]falconcore.Quantity{
		{
			DefaultName: "gate1",
		}: {
			Value: 1.23,
			Unit:  falconcore.Volt,
		},
	}

	settings := &mockSettingsReader{
		response: falconcore.SettingResponse{
			Getters: expected,
		},
	}

	handler := &CartesianMeasurementHandler{
		settings: settings,
	}

	ports := &config.ConnectedPorts{
		Knobs: []config.ConnectedPort{
			{
				PortName: "gate1",
				PortEntry: config.PortEntry{
					InstrumentName: "dac",
					Characteristic: instrumentcharacteristic.
						InstrumentCharacteristicSourceVoltage,
				},
			},
		},
	}

	out, err := handler.readSourceVoltages(
		ports,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		expected,
		out,
	)

	assert.Equal(
		t,
		1,
		settings.callCount,
	)

	assert.Equal(
		t,
		"capture-source-voltages",
		settings.lastReq.Message,
	)

	require.Len(
		t,
		settings.lastReq.Getters,
		1,
	)
}

func TestCartesianMeasurementHandler_ReadSourceVoltages_Error(
	t *testing.T,
) {
	settings := &mockSettingsReader{
		err: errors.New("boom"),
	}

	handler := &CartesianMeasurementHandler{
		settings: settings,
	}

	out, err := handler.readSourceVoltages(
		&config.ConnectedPorts{},
	)

	require.Error(t, err)

	assert.Nil(t, out)

	assert.Equal(
		t,
		"boom",
		err.Error(),
	)

	assert.Equal(
		t,
		1,
		settings.callCount,
	)
}
