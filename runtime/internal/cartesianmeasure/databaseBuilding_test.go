package cartesianmeasure

import (
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testConnectionName = "P1"
	testInstrumentName = "Source1"
	testChannelGroup   = "analog"
	testChannel        = 1
)

func testPort() falconcore.InstrumentPort {
	unit, err := falconcore.SymbolToUnit("V")
	if err != nil {
		panic(err)
	}

	return falconcore.InstrumentPort{
		DefaultName:    "Source1.analog.1.voltage",
		InstrumentName: "Source1",
		InstrumentType: instrument.DcVoltageSource,
		Unit:           unit,
		Access:         access.Read,
		Scope:          scope.Local,
		PortType:       porttype.PortTypeMeter,
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
		PseudoName: falconcore.Connection{
			Name: testConnectionName,
		},
	}
}

func testConnectedPorts() *config.ConnectedPorts {
	port := testPort()

	cp := config.ConnectedPort{
		PortEntry: config.PortEntry{
			InstrumentName: string(port.InstrumentName),
			InstrumentType: port.InstrumentType,
			Role:           port.PortType,
			Access:         port.Access,
			Scope:          port.Scope,
			Characteristic: port.Characteristic,
			Unit:           port.Unit,
			ChannelGroup:   "analog",
			Channel:        1,
		},
		PortName:   config.PortName(port.DefaultName),
		DeviceName: testConnectionName,
		Handle: falconcore.Connection{
			Name: testConnectionName,
		},
	}

	return &config.ConnectedPorts{
		AllConnections: []config.ConnectedPort{
			cp,
		},
		Meters: []config.ConnectedPort{
			cp,
		},
	}
}

func TestTestHelpersResolvePort(
	t *testing.T,
) {
	port := testPort()

	resolved, found := settingrouter.ResolvePort(
		port,
		testConnectedPorts(),
	)

	require.True(t, found)
	require.NotNil(t, resolved)
}

func TestGenerateID_Deterministic(
	t *testing.T,
) {
	id1 := generateID("measurement-1")
	id2 := generateID("measurement-1")

	assert.Equal(t, id1, id2)
}

func TestGenerateID_DifferentInputs(
	t *testing.T,
) {
	id1 := generateID("measurement-1")
	id2 := generateID("measurement-2")

	assert.NotEqual(t, id1, id2)
}

func TestBuildMeasurementResponse(
	t *testing.T,
) {
	arrays := falconcore.LabelledMeasuredArrays{}

	resp := BuildMeasurementResponse(
		arrays,
		"abc123",
	)

	assert.Equal(t, arrays, resp.Arrays)

	assert.Contains(
		t,
		resp.Message,
		"abc123",
	)
}

func TestBuildMetadata_Empty(
	t *testing.T,
) {
	meta := buildMetadata(nil)

	assert.Empty(t, meta)
}

func TestBuildMetadata_Voltage(
	t *testing.T,
) {
	port := testPort()

	meta := buildMetadata(
		map[falconcore.InstrumentPort]falconcore.Quantity{
			port: {
				Value: 1.25,
				Unit:  port.Unit,
			},
		},
	)

	require.Len(t, meta, 1)

	value, ok := meta["voltage.P1"]
	require.True(t, ok)

	assert.Contains(
		t,
		value,
		"1.25",
	)
}

func TestBuildWiremap(
	t *testing.T,
) {
	ports := testConnectedPorts()

	port := testPort()

	wiremap, err := buildWiremap(
		port,
		ports,
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		port.PseudoName.Name,
		wiremap.Connection,
	)

	require.Len(
		t,
		wiremap.Instruments,
		1,
	)

	assert.Equal(
		t,
		"Source1",
		wiremap.Instruments[0].Name,
	)
}

func TestBuildWiremap_PortNotFound(
	t *testing.T,
) {
	_, err := buildWiremap(
		testPort(),
		&config.ConnectedPorts{},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"not found",
	)
}

func TestBuildGetterInfo(
	t *testing.T,
) {
	ports := testConnectedPorts()

	info, err := buildGetterInfo(
		falconcore.Ports{
			testPort(),
		},
		ports,
	)

	require.NoError(t, err)

	require.Len(t, info, 1)

	assert.Equal(
		t,
		testPort().Unit,
		info[0].Unit,
	)
}

func TestBuildRanges(
	t *testing.T,
) {
	port := testPort()

	target := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    1,
	}

	output := PrimitiveMeasurementOutput{
		Getters: map[Target]RawData{
			target: {1, 2, 3, 4},
		},
	}

	ranges, err := buildRanges(
		falconcore.Ports{port},
		output,
		testConnectedPorts(),
		Shape{4},
	)

	require.NoError(t, err)

	require.Len(t, ranges, 1)

	assert.Equal(
		t,
		[]float64{1, 2, 3, 4},
		ranges[0].MeasuredArray.Data,
	)

	assert.Equal(
		t,
		[]int{4},
		ranges[0].MeasuredArray.Shape,
	)
}

func TestBuildRanges_ShapeMismatch(
	t *testing.T,
) {
	target := Target{
		Instrument: "Source1",
		Group:      "analog",
		Channel:    1,
	}

	output := PrimitiveMeasurementOutput{
		Getters: map[Target]RawData{
			target: {1, 2, 3},
		},
	}

	_, err := buildRanges(
		falconcore.Ports{testPort()},
		output,
		testConnectedPorts(),
		Shape{2, 2},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"requires 4",
	)
}

func TestBuildRanges_MissingTargetMapping(
	t *testing.T,
) {
	output := PrimitiveMeasurementOutput{
		Getters: map[Target]RawData{
			{
				Instrument: "missing",
				Group:      "x",
				Channel:    99,
			}: {1},
		},
	}

	_, err := buildRanges(
		falconcore.Ports{testPort()},
		output,
		testConnectedPorts(),
		Shape{1},
	)

	require.Error(t, err)

	assert.Contains(
		t,
		err.Error(),
		"missing port mapping",
	)
}

func TestBuildInsertEntry_NoWaveforms(
	t *testing.T,
) {
	h := &CartesianMeasurementHandler{}

	_, err := h.BuildInsertEntry(
		&MeasurementRequest{},
		testConnectedPorts(),
		Shape{1},
		"id",
	)

	require.Error(t, err)
}

func TestBuildDatabaseEntry_NoWaveforms(
	t *testing.T,
) {
	h := &CartesianMeasurementHandler{}

	_, err := h.BuildDatabaseEntry(
		&MeasurementRequest{},
		PrimitiveMeasurementOutput{},
		Shape{1},
		testConnectedPorts(),
		nil,
		"id",
	)

	require.Error(t, err)
}
