//go:build cgo

package settingrouter

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSetterHandler struct {
	canSetResult bool
	setErr       error

	canSetCalls int
	setCalls    int

	lastPort  falconcore.InstrumentPort
	lastValue falconcore.Quantity
}

func (*mockSetterHandler) Name() string {
	return "setter"
}

func (m *mockSetterHandler) CanSet(
	port falconcore.InstrumentPort,
	value falconcore.Quantity,
	_ *config.ConnectedPorts,
) bool {
	m.canSetCalls++
	m.lastPort = port
	m.lastValue = value
	return m.canSetResult
}

func (m *mockSetterHandler) Set(
	port falconcore.InstrumentPort,
	value falconcore.Quantity,
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
	canGetResult bool
	getResult    falconcore.Quantity
	getErr       error

	canGetCalls int
	getCalls    int

	lastPort falconcore.InstrumentPort
}

func (*mockGetterHandler) Name() string {
	return "getter"
}

func (m *mockGetterHandler) CanGet(
	port falconcore.InstrumentPort,
	_ map[InstrumentName]InstrumentMetadata,
	_ *config.ConnectedPorts,
) bool {
	m.canGetCalls++
	m.lastPort = port
	return m.canGetResult
}

func (m *mockGetterHandler) Get(
	port falconcore.InstrumentPort,
	_ *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	_ *config.ConnectedPorts,
	_ map[InstrumentName]InstrumentMetadata,
) (falconcore.Quantity, error) {
	m.getCalls++
	m.lastPort = port
	return m.getResult, m.getErr
}

func TestRouter_RegisterSetter(
	t *testing.T,
) {
	r := &Router{}

	handler := &mockSetterHandler{}

	out := r.RegisterSetter(handler)

	assert.Same(t, r, out)
	require.Len(t, r.setters, 1)
	assert.Same(t, handler, r.setters[0])
}

func TestRouter_RegisterGetter(
	t *testing.T,
) {
	r := &Router{}

	handler := &mockGetterHandler{}

	out := r.RegisterGetter(handler)

	assert.Same(t, r, out)
	require.Len(t, r.getters, 1)
	assert.Same(t, handler, r.getters[0])
}

func TestNewRouter(
	t *testing.T,
) {
	wiremap := config.WireMap{}
	ports := &config.ConnectedPorts{}

	meta := map[InstrumentName]InstrumentMetadata{
		"dmm": {},
	}

	r := NewRouter(
		nil,
		wiremap,
		ports,
		meta,
	)

	require.NotNil(t, r)

	assert.Equal(t, wiremap, r.wiremap)
	assert.Same(t, ports, r.ports)
	assert.Equal(t, meta, r.instrumentMetadata)
}

func TestRouter_Set_FirstMatchingHandlerWins(
	t *testing.T,
) {
	port := falconcore.InstrumentPort{}
	value := falconcore.Quantity{}

	first := &mockSetterHandler{}
	second := &mockSetterHandler{
		canSetResult: true,
	}
	third := &mockSetterHandler{
		canSetResult: true,
	}

	r := &Router{
		setters: []SetterHandler{
			first,
			second,
			third,
		},
	}

	err := r.set(
		port,
		value,
		&config.ConnectedPorts{},
	)

	require.NoError(t, err)

	assert.Equal(t, 1, first.canSetCalls)
	assert.Equal(t, 0, first.setCalls)

	assert.Equal(t, 1, second.canSetCalls)
	assert.Equal(t, 1, second.setCalls)

	assert.Equal(t, 0, third.canSetCalls)
}

func TestRouter_Set_NoMatchingHandler(
	t *testing.T,
) {
	r := &Router{}

	err := r.set(
		falconcore.InstrumentPort{},
		falconcore.Quantity{},
		&config.ConnectedPorts{},
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"no setter handler",
	)
}

func TestRouter_Set_HandlerError(
	t *testing.T,
) {
	expected := errors.New("boom")

	r := &Router{
		setters: []SetterHandler{
			&mockSetterHandler{
				canSetResult: true,
				setErr:       expected,
			},
		},
	}

	err := r.set(
		falconcore.InstrumentPort{},
		falconcore.Quantity{},
		&config.ConnectedPorts{},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
}

func TestRouter_Get_FirstMatchingHandlerWins(
	t *testing.T,
) {
	expected := falconcore.Quantity{
		Value: 123,
		Unit:  falconcore.Volt,
	}

	first := &mockGetterHandler{}
	second := &mockGetterHandler{
		canGetResult: true,
		getResult:    expected,
	}

	r := &Router{
		ports: &config.ConnectedPorts{},
		getters: []GetterHandler{
			first,
			second,
		},
	}

	actual, err := r.get(
		falconcore.InstrumentPort{},
		&config.ConnectedPorts{},
	)

	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestRouter_Get_NoMatchingHandler(
	t *testing.T,
) {
	r := &Router{}

	_, err := r.get(
		falconcore.InstrumentPort{},
		&config.ConnectedPorts{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"no getter handler",
	)
}

func TestRouter_Get_HandlerError(
	t *testing.T,
) {
	expected := errors.New("boom")

	r := &Router{
		getters: []GetterHandler{
			&mockGetterHandler{
				canGetResult: true,
				getErr:       expected,
			},
		},
	}

	_, err := r.get(
		falconcore.InstrumentPort{},
		&config.ConnectedPorts{},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expected)
}

func TestRouter_Handle(
	t *testing.T,
) {
	setter := &mockSetterHandler{
		canSetResult: true,
	}

	getter := &mockGetterHandler{
		canGetResult: true,
		getResult: falconcore.Quantity{
			Value: 7,
			Unit:  falconcore.Volt,
		},
	}
	ports := &config.ConnectedPorts{}

	port := falconcore.InstrumentPort{
		DefaultName: "P1",
	}

	req := falconcore.SettingRequest{
		Setters: map[falconcore.InstrumentPort]falconcore.Quantity{
			port: {
				Value: 5,
				Unit:  falconcore.Volt,
			},
		},
		Getters: falconcore.Ports{
			port,
		},
	}

	r := &Router{
		ports:   ports,
		setters: []SetterHandler{setter},
		getters: []GetterHandler{getter},
	}

	resp, err := r.Handle(req)

	require.NoError(t, err)

	assert.Equal(t, 1, setter.setCalls)
	assert.Equal(t, 1, getter.getCalls)

	assert.Equal(
		t,
		7.0,
		resp.Getters[port].Value,
	)
}

func TestRouter_Handle_SetError(
	t *testing.T,
) {
	r := &Router{
		setters: []SetterHandler{
			&mockSetterHandler{
				canSetResult: true,
				setErr:       errors.New("boom"),
			},
		},
	}

	_, err := r.Handle(
		falconcore.SettingRequest{
			Setters: map[falconcore.InstrumentPort]falconcore.Quantity{
				{}: {},
			},
		},
	)

	require.Error(t, err)
}

func TestRouter_Handle_GetError(
	t *testing.T,
) {
	r := &Router{
		getters: []GetterHandler{
			&mockGetterHandler{
				canGetResult: true,
				getErr:       errors.New("boom"),
			},
		},
	}

	_, err := r.Handle(
		falconcore.SettingRequest{
			Getters: falconcore.Ports{
				{},
			},
		},
	)

	require.Error(t, err)
}
