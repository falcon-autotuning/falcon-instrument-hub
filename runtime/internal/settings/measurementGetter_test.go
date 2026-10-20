package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRunner struct {
	results []dispatcher.MeasurementResult

	runCalls int
	requests []dispatcher.MeasurementRequest
}

func (m *mockRunner) RunAll(
	reqs []dispatcher.MeasurementRequest,
) []dispatcher.MeasurementResult {
	m.runCalls++

	m.requests = append(
		[]dispatcher.MeasurementRequest{},
		reqs...,
	)

	return m.results
}

var _ settingrouter.MeasurementRunner = (*mockRunner)(nil)

func TestGet_UnspecifiedCatalogUnit(t *testing.T) {
	connected := testConnectedPort()
	connected.Unit = falconcore.Dimensionless
	connected.UnitUnspecified = true
	port := testPort()
	port.Unit = connected.Unit
	runner := &mockRunner{
		results: []dispatcher.MeasurementResult{measurementResult("V", 0.123)},
	}

	q, err := NewMeasurementCharacteristicGetter().Get(
		port,
		runner,
		&config.ConnectedPorts{AllConnections: []config.ConnectedPort{connected}},
		testMetadata(),
	)

	require.NoError(t, err)
	assert.Equal(t, 0.123, q.Value)
	assert.Equal(t, falconcore.Volt, q.Unit)
}

func testPort() falconcore.InstrumentPort {
	return falconcore.InstrumentPort{
		DefaultName:    "P1",
		InstrumentName: "Source1",
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
		Unit:           falconcore.Volt,
	}
}

func testConnectedPort() config.ConnectedPort {
	return config.ConnectedPort{
		PortEntry: config.PortEntry{
			InstrumentName: "Source1",
			ChannelGroup:   "source",
			Channel:        1,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
			Unit:           falconcore.Volt,
		},
		PortName: "P1",
	}
}

func testMetadata() map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata {
	return map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata{
		"Source1": {
			API: &config.InstrumentAPI{},
		},
	}
}

func TestLookupDefinition_Found(t *testing.T) {
	def, ok := lookupDefinition(
		instrumentcharacteristic.InstrumentCharacteristicSourceVoltage,
	)

	require.True(t, ok)
	require.NotNil(t, def)
}

func TestLookupDefinition_NotFound(t *testing.T) {
	_, ok := lookupDefinition(
		instrumentcharacteristic.InstrumentCharacteristic(999),
	)

	assert.False(t, ok)
}

func TestMeasurementCharacteristicGetter_Name(
	t *testing.T,
) {
	g := NewMeasurementCharacteristicGetter()

	assert.Equal(
		t,
		"MeasurementCharacteristicGetter",
		g.Name(),
	)
}

func TestCanGet_PortMissing(
	t *testing.T,
) {
	g := NewMeasurementCharacteristicGetter()

	assert.False(
		t,
		g.CanGet(
			testPort(),
			testMetadata(),
			&config.ConnectedPorts{},
			t.TempDir(),
		),
	)
}

func TestCanGet_MissingMetadata(
	t *testing.T,
) {
	g := NewMeasurementCharacteristicGetter()

	tmpDir := t.TempDir()

	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(
				tmpDir,
				"get_source_voltage.lua",
			),
			[]byte("-- test"),
			0600,
		),
	)

	ports := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			testConnectedPort(),
		},
	}

	assert.False(
		t,
		g.CanGet(
			testPort(),
			nil,
			ports,
			tmpDir,
		),
	)
}

func TestCanGet_ScriptMissing(
	t *testing.T,
) {
	g := NewMeasurementCharacteristicGetter()

	ports := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			testConnectedPort(),
		},
	}

	assert.False(
		t,
		g.CanGet(
			testPort(),
			testMetadata(),
			ports,
			t.TempDir(),
		),
	)
}

func TestCanGet_Success(
	t *testing.T,
) {
	g := NewMeasurementCharacteristicGetter()

	tmpDir := t.TempDir()

	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(
				tmpDir,
				"get_source_voltage.lua",
			),
			[]byte("-- test"),
			0600,
		),
	)

	ports := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			testConnectedPort(),
		},
	}

	assert.True(
		t,
		g.CanGet(
			testPort(),
			testMetadata(),
			ports,
			tmpDir,
		),
	)
}

func TestGet_MissingMetadata(
	t *testing.T,
) {
	g := NewMeasurementCharacteristicGetter()

	_, err := g.Get(
		testPort(),
		&mockRunner{},
		nil,
		nil,
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"metadata missing",
	)
}

func TestGet_PortNotResolved(
	t *testing.T,
) {
	g := NewMeasurementCharacteristicGetter()

	_, err := g.Get(
		testPort(),
		&mockRunner{},
		&config.ConnectedPorts{},
		testMetadata(),
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"unable to resolve port",
	)
}

func TestGet_WrongResultCount(
	t *testing.T,
) {
	g := NewMeasurementCharacteristicGetter()

	ports := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			testConnectedPort(),
		},
	}

	runner := &mockRunner{
		results: nil,
	}

	_, err := g.Get(
		testPort(),
		runner,
		ports,
		testMetadata(),
	)

	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"expected one result",
	)
}

func TestGet_Success(
	t *testing.T,
) {
	g := NewMeasurementCharacteristicGetter()

	ports := &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			testConnectedPort(),
		},
	}

	runner := &mockRunner{
		results: []dispatcher.MeasurementResult{
			{
				Results: []instrumentserver.CallResult{
					{
						Return: []instrumentserver.ReturnValue{
							{
								Unit: "V",
								Value: instrumentserver.VariableValue{
									Value: float64(7),
								},
							},
						},
					},
				},
			},
		},
	}

	q, err := g.Get(
		testPort(),
		runner,
		ports,
		testMetadata(),
	)

	require.NoError(t, err)

	assert.Equal(t, 7.0, q.Value)
	assert.Equal(t, falconcore.Volt, q.Unit)

	require.Len(
		t,
		runner.requests,
		1,
	)

	req := runner.requests[0]

	assert.Equal(
		t,
		"get_source_voltage.lua",
		req.Script,
	)

	require.Len(
		t,
		req.Variables,
		1,
	)

	assert.Equal(
		t,
		"getter",
		req.Variables[0].Name,
	)

	target, ok := req.Variables[0].Value.Value.(instrumentserver.InstrumentTarget)

	assert.True(t, ok)
	assert.NotEmpty(t, string(target))
}
