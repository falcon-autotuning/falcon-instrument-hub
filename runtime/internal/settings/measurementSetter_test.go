//go:build cgo

package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSetterPort() falconcore.InstrumentPort {
	return falconcore.InstrumentPort{
		DefaultName:    "P1",
		InstrumentName: "Source1",
		InstrumentType: instrument.VoltageSource,
		Access:         access.Readwrite,
		Scope:          scope.Local,
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
		PortType:       porttype.PortTypeKnob,
		Unit:           falconcore.Volt,
	}
}

func testSetterConnectedPort() config.ConnectedPort {
	return config.ConnectedPort{
		PortEntry: config.PortEntry{
			InstrumentName: "Source1",
			InstrumentType: instrument.VoltageSource,
			Access:         access.Readwrite,
			Scope:          scope.Local,
			Role:           porttype.PortTypeKnob,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
			Unit:           falconcore.Volt,
			ChannelGroup:   "source",
			Channel:        1,
		},
		PortName: "P1",
	}
}

func testSetterConnectedPorts() *config.ConnectedPorts {
	return &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			testSetterConnectedPort(),
		},
	}
}

func TestMeasurementCharacteristicSetter_Name(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	assert.Equal(
		t,
		"MeasurementCharacteristicSetter",
		s.Name(),
	)
}

func TestLookupSetterDefinition_Found(
	t *testing.T,
) {
	def, ok := lookupSetterDefinition(
		instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
	)

	require.True(t, ok)
	require.NotNil(t, def)
}

func TestLookupSetterDefinition_NotFound(
	t *testing.T,
) {
	_, ok := lookupSetterDefinition(
		instrumentcharacteristic.InstrumentCharacteristic(999),
	)

	assert.False(t, ok)
}

func TestCanSet_PortMissing(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	assert.False(
		t,
		s.CanSet(
			testSetterPort(),
			falconcore.Quantity{},
			&config.ConnectedPorts{},
			t.TempDir(),
		),
	)
}

func TestCanSet_ReadOnly(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	port := testSetterPort()
	port.Access = access.Read

	assert.False(
		t,
		s.CanSet(
			port,
			falconcore.Quantity{},
			testSetterConnectedPorts(),
			t.TempDir(),
		),
	)
}

func TestCanSet_UnsupportedCharacteristic(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	port := testSetterPort()
	port.Characteristic = instrumentcharacteristic.InstrumentCharacteristic(999)

	assert.False(
		t,
		s.CanSet(
			port,
			falconcore.Quantity{},
			testSetterConnectedPorts(),
			t.TempDir(),
		),
	)
}

func TestCanSet_MissingScript(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	assert.False(
		t,
		s.CanSet(
			testSetterPort(),
			falconcore.Quantity{},
			testSetterConnectedPorts(),
			t.TempDir(),
		),
	)
}

func TestCanSet_ScriptIsDirectory(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	tmpDir := t.TempDir()

	def, ok := lookupSetterDefinition(
		instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
	)
	require.True(t, ok)

	require.NoError(
		t,
		os.MkdirAll(
			filepath.Join(tmpDir, def.Script),
			0755,
		),
	)

	assert.False(
		t,
		s.CanSet(
			testSetterPort(),
			falconcore.Quantity{},
			testSetterConnectedPorts(),
			tmpDir,
		),
	)
}

func TestCanSet_Success(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	tmpDir := t.TempDir()

	def, ok := lookupSetterDefinition(
		instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
	)
	require.True(t, ok)

	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(tmpDir, def.Script),
			[]byte("-- test"),
			0600,
		),
	)

	assert.True(
		t,
		s.CanSet(
			testSetterPort(),
			falconcore.Quantity{},
			testSetterConnectedPorts(),
			tmpDir,
		),
	)
}

func TestSet_UnsupportedCharacteristic(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	port := testSetterPort()
	port.Characteristic = instrumentcharacteristic.InstrumentCharacteristic(999)

	err := s.Set(
		port,
		falconcore.Quantity{},
		&mockRunner{},
		testSetterConnectedPorts(),
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"unsupported characteristic",
	)
}

func TestSet_PortResolutionFailure(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	err := s.Set(
		testSetterPort(),
		falconcore.Quantity{},
		&mockRunner{},
		&config.ConnectedPorts{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"unable to resolve port",
	)
}

func TestSet_WrongResultCount(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	runner := &mockRunner{}

	err := s.Set(
		testSetterPort(),
		falconcore.Quantity{
			Value: 12,
			Unit:  falconcore.Volt,
		},
		runner,
		testSetterConnectedPorts(),
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"expected one result got 0",
	)
}

func TestSet_RunMeasurementError(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	expected := errors.New("boom")

	runner := &mockRunner{
		results: []dispatcher.MeasurementResult{
			{
				Err: expected,
			},
		},
	}

	err := s.Set(
		testSetterPort(),
		falconcore.Quantity{
			Value: 12,
			Unit:  falconcore.Volt,
		},
		runner,
		testSetterConnectedPorts(),
	)

	require.Error(t, err)

	assert.ErrorIs(
		t,
		err,
		expected,
	)
}

func TestSet_Success(
	t *testing.T,
) {
	s := NewMeasurementCharacteristicSetter()

	runner := &mockRunner{
		results: []dispatcher.MeasurementResult{
			{},
		},
	}

	err := s.Set(
		testSetterPort(),
		falconcore.Quantity{
			Value: 123,
			Unit:  falconcore.Volt,
		},
		runner,
		testSetterConnectedPorts(),
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		1,
		runner.runCalls,
	)

	require.Len(
		t,
		runner.requests,
		1,
	)

	req := runner.requests[0]

	assert.Equal(
		t,
		"set_source_voltage.lua",
		req.Script,
	)

	require.Len(
		t,
		req.Variables,
		2,
	)

	assert.Equal(
		t,
		"setter",
		req.Variables[0].Name,
	)

	assert.Equal(
		t,
		"setVoltage",
		req.Variables[1].Name,
	)

	_, ok := req.Variables[0].Value.Value.(instrumentserver.InstrumentTarget)

	assert.True(
		t,
		ok,
	)
}
