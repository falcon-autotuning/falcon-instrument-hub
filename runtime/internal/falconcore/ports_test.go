package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listinstrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPortsReader struct {
	ports *listinstrumentport.Handle
	err   error

	portsCalls int
}

func (m *mockPortsReader) Ports() (*listinstrumentport.Handle, error) {
	m.portsCalls++

	if m.err != nil {
		return nil, m.err
	}

	return m.ports, nil
}

func testInstrumentPort() InstrumentPort {
	return InstrumentPort{
		DefaultName:    "P1",
		InstrumentName: "DAC1",
		InstrumentType: instrument.Amnmeter,
		Description:    "test port",
		Unit:           Volt,
		Access:         access.Readwrite,
		Scope:          scope.Global,
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
		PortType:       porttype.PortTypeKnob,
	}
}

func TestPorts_NewFalconPorts_Empty(t *testing.T) {
	p := Ports{}

	handle, err := p.NewFalconPorts()

	require.NoError(t, err)
	require.NotNil(t, handle)

	list, err := handle.Ports()
	require.NoError(t, err)

	size, err := list.Size()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), size)

	assert.NoError(t, list.Close())
	assert.NoError(t, handle.Close())
}

func TestPorts_NewFalconPorts_Error(t *testing.T) {
	p := Ports{
		{
			DefaultName:    "P1",
			InstrumentName: "DAC1",
			InstrumentType: instrument.Amnmeter,
			Unit:           Unit(999),
		},
	}

	handle, err := p.NewFalconPorts()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestPorts_NewFalconPorts_Success(t *testing.T) {
	p := Ports{
		testInstrumentPort(),
		testInstrumentPort(),
	}

	handle, err := p.NewFalconPorts()

	require.NoError(t, err)
	require.NotNil(t, handle)

	list, err := handle.Ports()
	require.NoError(t, err)

	size, err := list.Size()
	require.NoError(t, err)

	assert.Equal(t, uint64(2), size)

	assert.NoError(t, list.Close())
	assert.NoError(t, handle.Close())
}

func TestPortsFromFalcon_PortsError(t *testing.T) {
	expectedErr := errors.New("ports failed")

	reader := &mockPortsReader{
		err: expectedErr,
	}

	out, err := PortsFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Empty(t, out)
	assert.Equal(t, 1, reader.portsCalls)
}

func TestPortsFromFalcon_Empty(t *testing.T) {
	handle, err := ports.NewEmpty()
	require.NoError(t, err)
	defer handle.Close()

	out, err := PortsFromFalcon(handle)

	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestPortsFromFalcon_Success(t *testing.T) {
	realPort, err := testInstrumentPort().NewFalconInstrumentPort()
	require.NoError(t, err)
	defer realPort.Close()

	handle, err := ports.NewEmpty()
	require.NoError(t, err)
	defer handle.Close()

	require.NoError(t, handle.PushBack(realPort))

	out, err := PortsFromFalcon(handle)

	require.NoError(t, err)

	require.Len(t, out, 1)

	assert.Equal(t, PortName("P1"), out[0].DefaultName)
	assert.Equal(t, InstrumentName("DAC1"), out[0].InstrumentName)
	assert.Equal(t, Volt, out[0].Unit)
	assert.Equal(t, instrument.Amnmeter, out[0].InstrumentType)
}
