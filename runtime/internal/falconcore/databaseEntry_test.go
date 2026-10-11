package falconcore

import (
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDatabaseLabelledDomain() LabelledDomain {
	return LabelledDomain{
		InstrumentPort: InstrumentPort{
			DefaultName:    "P1",
			InstrumentName: "DAC1",
			InstrumentType: instrument.Amnmeter,
			Description:    "test",
			Unit:           Volt,
			Access:         access.Readwrite,
			Scope:          scope.Global,
			Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
			PortType:       porttype.PortTypeKnob,
		},
		Domain: Domain{
			LesserBound:           -1,
			GreaterBound:          1,
			LesserBoundContained:  true,
			GreaterBoundContained: false,
		},
	}
}

func testDatabaseEntry() DatabaseEntry {
	return DatabaseEntry{
		Shape: AxesInt{2, 2},

		UnitDomain: AxesControlArray{
			{
				Data:  []float64{0, 1},
				Shape: []int{2},
			},
		},

		DomainLabels: AxesCoupledLabelledDomain{
			{
				testDatabaseLabelledDomain(),
			},
		},

		Ranges: LabelledMeasuredArrays{
			{
				AcquisitionContext: AcquisitionContext{
					Connection: Connection{
						Name: "P1",
						Type: PlungerGate,
					},
					Unit:           Volt,
					InstrumentType: instrument.Amnmeter,
				},
				MeasuredArray: MeasuredArray{
					Data:  []float64{1, 2},
					Shape: []int{2},
				},
			},
		},

		Metadata: Metadata{
			"key": "value",
		},

		Title:     "test_measurement",
		Id:        123,
		Timestamp: 456,
	}
}

func TestDatabaseEntry_NewFalconDatabaseEntry_Success(
	t *testing.T,
) {
	entry := testDatabaseEntry()

	handle, err := entry.NewFalconDatabaseEntry()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestDatabaseEntry_NewFalconDatabaseEntry_UnitDomainError(
	t *testing.T,
) {
	entry := testDatabaseEntry()

	entry.UnitDomain = AxesControlArray{
		{
			Data:  []float64{1},
			Shape: []int{-1},
		},
	}

	handle, err := entry.NewFalconDatabaseEntry()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestDatabaseEntry_NewFalconDatabaseEntry_DomainLabelsError(
	t *testing.T,
) {
	entry := testDatabaseEntry()

	entry.DomainLabels = AxesCoupledLabelledDomain{
		{
			{
				InstrumentPort: InstrumentPort{
					Unit: Unit(999),
				},
			},
		},
	}

	handle, err := entry.NewFalconDatabaseEntry()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestDatabaseEntry_NewFalconDatabaseEntry_RangesError(
	t *testing.T,
) {
	entry := testDatabaseEntry()

	entry.Ranges = LabelledMeasuredArrays{
		{
			AcquisitionContext: AcquisitionContext{
				Connection: Connection{
					Type: ConnectionType(999),
				},
				Unit: Volt,
			},
		},
	}

	handle, err := entry.NewFalconDatabaseEntry()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestFalconDatabaseEntryFromFalcon_Success(
	t *testing.T,
) {
	expected := testDatabaseEntry()

	handle, err := expected.NewFalconDatabaseEntry()
	require.NoError(t, err)
	defer handle.Close()

	out, err := FalconDatabaseEntryFromFalcon(handle)

	require.NoError(t, err)

	assert.Equal(t, expected.Shape, out.Shape)
	assert.Equal(t, expected.UnitDomain, out.UnitDomain)
	assert.Equal(t, expected.DomainLabels, out.DomainLabels)
	assert.Equal(t, expected.Ranges, out.Ranges)
	assert.Equal(t, expected.Metadata, out.Metadata)
	assert.Equal(t, expected.Title, out.Title)
	assert.Equal(t, expected.Id, out.Id)
	assert.Equal(t, expected.Timestamp, out.Timestamp)
}
