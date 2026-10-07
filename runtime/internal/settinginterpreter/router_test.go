//go:build cgo

package settinginterpreter

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportquantity"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/quantity"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSetterHandler struct {
	name string

	canSetResult bool
	setErr       error

	canSetCalls int
	setCalls    int

	lastPort  ConnectedPort
	lastValue Quantity
}

func (m *mockSetterHandler) Name() string {
	return m.name
}

func (m *mockSetterHandler) CanSet(
	port ConnectedPort,
	value Quantity,
) bool {
	m.canSetCalls++
	m.lastPort = port
	m.lastValue = value

	return m.canSetResult
}

func (m *mockSetterHandler) Set(
	port ConnectedPort,
	value Quantity,
	_ *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	_ *config.ConnectedPorts,
) error {
	m.setCalls++
	m.lastPort = port
	m.lastValue = value

	return m.setErr
}

type mockGetterHandler struct {
	name string

	canGetResult bool

	getResult Quantity
	getErr    error

	canGetCalls int
	getCalls    int

	lastPort ConnectedPort
}

func (m *mockGetterHandler) Name() string {
	return m.name
}

func (m *mockGetterHandler) CanGet(
	port ConnectedPort,
) bool {
	m.canGetCalls++
	m.lastPort = port

	return m.canGetResult
}

func (m *mockGetterHandler) Get(
	port ConnectedPort,
	_ *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	_ *config.ConnectedPorts,
) (Quantity, error) {
	m.getCalls++
	m.lastPort = port

	return m.getResult, m.getErr
}

func TestRouter_Set_FirstMatchingHandlerWins(
	t *testing.T,
) {
	port := ConnectedPort{}
	value := Quantity{}

	first := &mockSetterHandler{
		name:         "first",
		canSetResult: false,
	}

	second := &mockSetterHandler{
		name:         "second",
		canSetResult: true,
	}

	third := &mockSetterHandler{
		name:         "third",
		canSetResult: true,
	}

	router := &Router{
		setters: []SetterHandler{
			first,
			second,
			third,
		},
	}

	err := router.set(port, value)

	require.NoError(t, err)

	assert.Equal(t, 1, first.canSetCalls)
	assert.Equal(t, 0, first.setCalls)

	assert.Equal(t, 1, second.canSetCalls)
	assert.Equal(t, 1, second.setCalls)

	assert.Equal(t, 0, third.canSetCalls)
	assert.Equal(t, 0, third.setCalls)
}

func TestRouter_Set_NoMatchingHandler(
	t *testing.T,
) {
	router := &Router{
		setters: []SetterHandler{},
	}

	err := router.set(
		ConnectedPort{},
		Quantity{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"no setter handler",
	)
}

func TestRouter_Get_FirstMatchingHandlerWins(
	t *testing.T,
) {
	expected := Quantity{}

	first := &mockGetterHandler{
		name:         "first",
		canGetResult: false,
	}

	second := &mockGetterHandler{
		name:         "second",
		canGetResult: true,
		getResult:    expected,
	}

	router := &Router{
		getters: []GetterHandler{
			first,
			second,
		},
	}

	result, err := router.get(
		ConnectedPort{},
	)

	require.NoError(t, err)

	assert.Equal(t, expected, result)

	assert.Equal(t, 1, first.canGetCalls)
	assert.Equal(t, 0, first.getCalls)

	assert.Equal(t, 1, second.canGetCalls)
	assert.Equal(t, 1, second.getCalls)
}

func TestRouter_Get_NoMatchingHandler(
	t *testing.T,
) {
	router := &Router{}

	_, err := router.get(
		ConnectedPort{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"no getter handler",
	)
}

func TestRouter_Set_HandlerError(
	t *testing.T,
) {
	expectedErr := errors.New(
		"set failed",
	)

	handler := &mockSetterHandler{
		canSetResult: true,
		setErr:       expectedErr,
	}

	router := &Router{
		setters: []SetterHandler{
			handler,
		},
	}

	err := router.set(
		ConnectedPort{},
		Quantity{},
	)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expectedErr,
	)
}

func TestRouter_Get_HandlerError(
	t *testing.T,
) {
	expectedErr := errors.New(
		"get failed",
	)

	handler := &mockGetterHandler{
		canGetResult: true,
		getErr:       expectedErr,
	}

	router := &Router{
		getters: []GetterHandler{
			handler,
		},
	}

	_, err := router.get(
		ConnectedPort{},
	)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expectedErr,
	)
}

func mustFalconPort(
	t *testing.T,
) *instrumentport.Handle {
	t.Helper()

	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)

	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)

	port, err := instrumentport.NewKnob(
		"P1",
		"Source1",
		conn,
		instrument.DcVoltageSource,
		unit,
		"test port",
	)
	require.NoError(t, err)

	return port
}

func buildTestConnectedPorts(
	t *testing.T,
) *config.ConnectedPorts {
	t.Helper()

	return &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			{
				PortEntry: config.PortEntry{
					InstrumentName: "Source1",
					InstrumentType: instrument.DcVoltageSource,
					Role:           porttype.PortTypeKnob,
					Access:         access.Write,
					Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
				},
				PortName:   "Source1.voltage",
				DeviceName: "P1",
			},
			{
				PortEntry: config.PortEntry{
					InstrumentName: "Source1",
					InstrumentType: instrument.DcVoltageSource,
					Role:           porttype.PortTypeMeter,
					Access:         access.Read,
					Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
				},
				PortName:   "Source1.measure_voltage",
				DeviceName: "P1",
			},
		},
	}
}

func newTestKnobPort(
	t *testing.T,
) *instrumentport.Handle {
	t.Helper()

	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)

	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)

	port, err := instrumentport.NewKnob(
		"P1",
		"Source1",
		conn,
		instrument.DcVoltageSource,
		unit,
		"voltage source",
	)
	require.NoError(t, err)

	return port
}

func newTestMeterPort(
	t *testing.T,
) *instrumentport.Handle {
	t.Helper()

	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)

	conn, err := connection.NewPlungerGate("P1")
	require.NoError(t, err)

	port, err := instrumentport.NewMeter(
		"P1",
		"Source1",
		conn,
		instrument.DcVoltageSource,
		unit,
		"voltage meter",
	)
	require.NoError(t, err)

	return port
}

func newVoltQuantity(
	t *testing.T,
	value float64,
) *quantity.Handle {
	t.Helper()

	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)

	q, err := quantity.New(
		value,
		unit,
	)
	require.NoError(t, err)

	return q
}

func TestFalconSettingResponse_RoundTrip(
	t *testing.T,
) {
	falconPort := mustFalconPort(t)
	defer falconPort.Close()

	q := newVoltQuantity(t, 7)
	defer q.Close()

	response, err := NewFalconSettingResponse(
		map[ConnectedPort]Quantity{
			{
				ConnectedPort: config.ConnectedPort{
					DeviceName: "P1",
				},
				FalconPort: falconPort,
			}: {
				Handle: q,
			},
		},
	)
	require.NoError(t, err)
	defer response.Close()

	jsonText, err := response.ToJSON()
	require.NoError(t, err)

	parsed, err := settingresponse.FromJSON(
		jsonText,
	)
	require.NoError(t, err)
	defer parsed.Close()

	getters, err := parsed.Getters()
	require.NoError(t, err)
	defer getters.Close()

	size, err := getters.Size()
	require.NoError(t, err)

	assert.Equal(
		t,
		uint64(1),
		size,
	)

	keys, err := getters.Keys()
	require.NoError(t, err)
	defer keys.Close()

	key, err := keys.At(0)
	require.NoError(t, err)
	defer key.Close()

	value, err := getters.At(key)
	require.NoError(t, err)
	defer value.Close()

	parsedValue, err := value.Value()
	require.NoError(t, err)

	assert.Equal(
		t,
		7.0,
		parsedValue,
	)

	unit, err := value.Unit()
	require.NoError(t, err)
	defer unit.Close()

	symbol, err := unit.Symbol()
	require.NoError(t, err)

	assert.Equal(
		t,
		"V",
		symbol,
	)
}

func TestFalconSettingRequest_RoundTrip(
	t *testing.T,
) {
	knob := newTestKnobPort(t)
	defer knob.Close()

	meter := newTestMeterPort(t)
	defer meter.Close()

	q := newVoltQuantity(t, 5)
	defer q.Close()

	setters, err := mapinstrumentportquantity.NewEmpty()
	require.NoError(t, err)
	defer setters.Close()

	require.NoError(
		t,
		setters.InsertOrAssign(
			knob,
			q,
		),
	)

	getters, err := ports.New(
		[]*instrumentport.Handle{
			meter,
		},
	)
	require.NoError(t, err)
	defer getters.Close()

	request, err := settingrequest.New(
		"integration",
		getters,
		setters,
	)
	require.NoError(t, err)
	defer request.Close()

	jsonText, err := request.ToJSON()
	require.NoError(t, err)

	req, err := NewFalconSettingRequestFromJSON(
		jsonText,
		buildTestConnectedPorts(t),
	)
	require.NoError(t, err)
	defer req.Close()

	parsedSetters, err := req.Setters()
	require.NoError(t, err)

	require.Len(t, parsedSetters, 1)

	for _, q := range parsedSetters {
		value, err := q.Value()
		require.NoError(t, err)

		assert.Equal(t, 5.0, value)

		unit, err := q.UnitSymbol()
		require.NoError(t, err)

		assert.Equal(t, "V", unit)

		_ = q.Close()
	}

	parsedGetters, err := req.Getters()
	require.NoError(t, err)

	require.Len(t, parsedGetters, 1)

	assert.Equal(
		t,
		"Source1",
		parsedGetters[0].InstrumentName,
	)
}

type fakeSetter struct {
	calls int

	lastPort  ConnectedPort
	lastValue Quantity
}

func (f *fakeSetter) Name() string { return "fake-setter" }

func (f *fakeSetter) CanSet(
	port ConnectedPort,
	value Quantity,
) bool {
	return true
}

func (f *fakeSetter) Set(
	port ConnectedPort,
	value Quantity,
	_ *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	_ *config.ConnectedPorts,
) error {
	f.calls++
	f.lastPort = port
	f.lastValue = value

	return nil
}

type fakeGetter struct {
	result Quantity
}

func (f *fakeGetter) Name() string { return "fake-getter" }

func (f *fakeGetter) CanGet(
	ConnectedPort,
) bool {
	return true
}

func (f *fakeGetter) Get(
	_ ConnectedPort,
	_ *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	_ *config.ConnectedPorts,
) (Quantity, error) {
	return f.result, nil
}

func buildRealFalconRequest(
	t *testing.T,
) string {
	t.Helper()

	knob := newTestKnobPort(t)
	defer knob.Close()

	meter := newTestMeterPort(t)
	defer meter.Close()

	q := newVoltQuantity(t, 5)
	defer q.Close()

	setters, err := mapinstrumentportquantity.NewEmpty()
	require.NoError(t, err)
	defer setters.Close()

	err = setters.InsertOrAssign(
		knob,
		q,
	)
	require.NoError(t, err)

	getters, err := ports.New(
		[]*instrumentport.Handle{
			meter,
		},
	)
	require.NoError(t, err)
	defer getters.Close()

	req, err := settingrequest.New(
		"integration test",
		getters,
		setters,
	)
	require.NoError(t, err)
	defer req.Close()

	jsonText, err := req.ToJSON()
	require.NoError(t, err)

	return jsonText
}

func TestRouter_Handle_EndToEnd(
	t *testing.T,
) {
	portsCatalog := buildTestConnectedPorts(t)

	inputRequest := buildRealFalconRequest(
		t,
	)

	req, err := NewFalconSettingRequestFromJSON(
		inputRequest,
		portsCatalog,
	)
	require.NoError(t, err)
	defer req.Close()

	setter := &fakeSetter{}

	getterResult := Quantity{
		Handle: newVoltQuantity(t, 7),
	}
	defer getterResult.Close()

	router := &Router{
		setters: []SetterHandler{
			setter,
		},
		getters: []GetterHandler{
			&fakeGetter{
				result: getterResult,
			},
		},
		ports: portsCatalog,
	}

	response, err := router.Handle(req)
	require.NoError(t, err)
	defer response.Close()

	assert.Equal(
		t,
		1,
		setter.calls,
	)

	jsonText, err := response.ToJSON()
	require.NoError(t, err)

	parsed, err := settingresponse.FromJSON(
		jsonText,
	)
	require.NoError(t, err)
	defer parsed.Close()

	getters, err := parsed.Getters()
	require.NoError(t, err)
	defer getters.Close()

	assert.Equal(t, 1, setter.calls)

	setterValue, err := setter.lastValue.Value()
	require.NoError(t, err)

	assert.Equal(t, 5.0, setterValue)

	setterUnit, err := setter.lastValue.UnitSymbol()
	require.NoError(t, err)

	assert.Equal(t, "V", setterUnit)

	size, err := getters.Size()
	require.NoError(t, err)

	require.Equal(t, uint64(1), size)

	keys, err := getters.Keys()
	require.NoError(t, err)
	defer keys.Close()

	key, err := keys.At(0)
	require.NoError(t, err)
	defer key.Close()

	value, err := getters.At(key)
	require.NoError(t, err)
	defer value.Close()

	actual, err := value.Value()
	require.NoError(t, err)

	assert.Equal(t, 7.0, actual)

	unit, err := value.Unit()
	require.NoError(t, err)
	defer unit.Close()

	symbol, err := unit.Symbol()
	require.NoError(t, err)

	assert.Equal(t, "V", symbol)

	defaultName, err := key.DefaultName()
	require.NoError(t, err)

	assert.Equal(t, "P1", defaultName)

	instrumentName, err := key.InstrumentName()
	require.NoError(t, err)

	assert.Equal(t, "Source1", instrumentName)
}
