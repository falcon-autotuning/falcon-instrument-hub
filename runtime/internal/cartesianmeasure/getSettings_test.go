package cartesianmeasure

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

func testNumberOfSamplesPort(
	instrumentName string,
	channelGroup string,
	channel int,
	scopeType scope.Scope,
) config.ConnectedPort {
	return config.ConnectedPort{
		PortName: "samples",
		PortEntry: config.PortEntry{
			InstrumentName: instrumentName,
			InstrumentType: instrument.DcVoltageSource,
			Characteristic: instrumentcharacteristic.
				InstrumentCharacteristicNumberOfSamples,
			Role:         porttype.PortTypeSetting,
			Access:       access.Read,
			Scope:        scopeType,
			Unit:         falconcore.Volt,
			ChannelGroup: channelGroup,
			Channel:      channel,
		},
	}
}

func TestNumberOfSamplesPorts_PrefersLocal(
	t *testing.T,
) {
	getter := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    1,
	}

	getterPorts := map[Target]falconcore.InstrumentPort{
		getter: {
			InstrumentName: "Source1",
		},
	}

	local := testNumberOfSamplesPort(
		"Source1",
		"analog",
		1,
		scope.Local,
	)

	global := testNumberOfSamplesPort(
		"Source1",
		"",
		0,
		scope.Global,
	)

	mapping, err := numberOfSamplesPorts(
		[]Target{getter},
		getterPorts,
		&config.ConnectedPorts{
			Settings: []config.ConnectedPort{
				global,
				local,
			},
		},
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		local.PortName,
		config.PortName(
			mapping[getter].DefaultName,
		),
	)
}

func TestNumberOfSamplesPorts_GlobalFallback(
	t *testing.T,
) {
	getter := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    1,
	}

	getterPorts := map[Target]falconcore.InstrumentPort{
		getter: {
			InstrumentName: "Source1",
		},
	}

	global := testNumberOfSamplesPort(
		"Source1",
		"",
		0,
		scope.Global,
	)

	mapping, err := numberOfSamplesPorts(
		[]Target{getter},
		getterPorts,
		&config.ConnectedPorts{
			Settings: []config.ConnectedPort{
				global,
			},
		},
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		global.PortName,
		config.PortName(
			mapping[getter].DefaultName,
		),
	)
}

func TestNumberOfSamplesPorts_NotFound(
	t *testing.T,
) {
	getter := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    1,
	}

	getterPorts := map[Target]falconcore.InstrumentPort{
		getter: {
			InstrumentName: "Source1",
		},
	}

	_, err := numberOfSamplesPorts(
		[]Target{getter},
		getterPorts,
		&config.ConnectedPorts{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"no NumberOfSamples setting found",
	)
}

func TestReadNumberOfPoints(
	t *testing.T,
) {
	getter := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    1,
	}

	samplePort := InstrumentPortFromConnectedPort(
		testNumberOfSamplesPort(
			"Source1",
			"",
			0,
			scope.Global,
		),
	)

	settings := &mockSettingsReader{
		response: falconcore.SettingResponse{
			Getters: map[falconcore.InstrumentPort]falconcore.Quantity{
				samplePort: {
					Value: 16,
				},
			},
		},
	}

	handler := &CartesianMeasurementHandler{
		settings: settings,
	}

	out, err := handler.readNumberOfPoints(
		[]Target{getter},
		map[Target]falconcore.InstrumentPort{
			getter: {
				InstrumentName: "Source1",
			},
		},
		&config.ConnectedPorts{
			Settings: []config.ConnectedPort{
				testNumberOfSamplesPort(
					"Source1",
					"",
					0,
					scope.Global,
				),
			},
		},
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		16,
		out[getter],
	)
}

func TestReadNumberOfPoints_Deduplicates(
	t *testing.T,
) {
	g1 := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    1,
	}

	g2 := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    2,
	}

	global := testNumberOfSamplesPort(
		"Source1",
		"",
		0,
		scope.Global,
	)

	samplePort := InstrumentPortFromConnectedPort(
		global,
	)

	settings := &mockSettingsReader{
		response: falconcore.SettingResponse{
			Getters: map[falconcore.InstrumentPort]falconcore.Quantity{
				samplePort: {
					Value: 8,
				},
			},
		},
	}

	handler := &CartesianMeasurementHandler{
		settings: settings,
	}

	_, err := handler.readNumberOfPoints(
		[]Target{g1, g2},
		map[Target]falconcore.InstrumentPort{
			g1: {InstrumentName: "Source1"},
			g2: {InstrumentName: "Source1"},
		},
		&config.ConnectedPorts{
			Settings: []config.ConnectedPort{
				global,
			},
		},
	)

	require.NoError(t, err)

	require.Len(
		t,
		settings.lastReq.Getters,
		1,
	)
}

func TestReadNumberOfPoints_HandleError(
	t *testing.T,
) {
	handler := &CartesianMeasurementHandler{
		settings: &mockSettingsReader{
			err: errors.New("boom"),
		},
	}

	getter := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    1,
	}

	_, err := handler.readNumberOfPoints(
		[]Target{getter},
		map[Target]falconcore.InstrumentPort{
			getter: {
				InstrumentName: "Source1",
			},
		},
		&config.ConnectedPorts{
			Settings: []config.ConnectedPort{
				testNumberOfSamplesPort(
					"Source1",
					"",
					0,
					scope.Global,
				),
			},
		},
	)

	require.Error(t, err)
	assert.Equal(t, "boom", err.Error())
}

func TestReadNumberOfPoints_MissingResponse(
	t *testing.T,
) {
	getter := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    1,
	}

	handler := &CartesianMeasurementHandler{
		settings: &mockSettingsReader{
			response: falconcore.SettingResponse{
				Getters: map[falconcore.InstrumentPort]falconcore.Quantity{},
			},
		},
	}

	_, err := handler.readNumberOfPoints(
		[]Target{getter},
		map[Target]falconcore.InstrumentPort{
			getter: {
				InstrumentName: "Source1",
			},
		},
		&config.ConnectedPorts{
			Settings: []config.ConnectedPort{
				testNumberOfSamplesPort(
					"Source1",
					"",
					0,
					scope.Global,
				),
			},
		},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"missing sample count",
	)
}
