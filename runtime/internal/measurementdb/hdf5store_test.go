//go:build cgo

package measurementdb

import (
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDatabaseEntry() falconcore.DatabaseEntry {
	return falconcore.DatabaseEntry{
		Shape: falconcore.AxesInt{2, 2},

		UnitDomain: falconcore.AxesControlArray{
			{
				Data:  []float64{0, 1},
				Shape: []int{2},
			},
			{
				Data:  []float64{0, 1},
				Shape: []int{2},
			},
		},

		DomainLabels: falconcore.AxesCoupledLabelledDomain{
			{
				{
					InstrumentPort: falconcore.InstrumentPort{
						DefaultName:    "P1",
						InstrumentName: "DAC1",
						InstrumentType: instrument.Amnmeter,
						Description:    "test",
						Unit:           falconcore.Volt,
						Access:         access.Readwrite,
						Scope:          scope.Global,
						Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
						PortType:       porttype.PortTypeKnob,
					},
					Domain: falconcore.Domain{
						LesserBound:           -1,
						GreaterBound:          1,
						LesserBoundContained:  true,
						GreaterBoundContained: false,
					},
				},
			},
			{
				{
					InstrumentPort: falconcore.InstrumentPort{
						DefaultName:    "P3",
						InstrumentName: "DAC1",
						InstrumentType: instrument.Amnmeter,
						Description:    "test",
						Unit:           falconcore.Volt,
						Access:         access.Readwrite,
						Scope:          scope.Global,
						Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
						PortType:       porttype.PortTypeKnob,
					},
					Domain: falconcore.Domain{
						LesserBound:           -1,
						GreaterBound:          1,
						LesserBoundContained:  true,
						GreaterBoundContained: false,
					},
				},
			},
		},

		Ranges: falconcore.LabelledMeasuredArrays{
			{
				AcquisitionContext: falconcore.AcquisitionContext{
					Connection: falconcore.Connection{
						Name: "P2",
						Type: falconcore.PlungerGate,
					},
					Unit:           falconcore.Volt,
					InstrumentType: instrument.Amnmeter,
				},
				MeasuredArray: falconcore.MeasuredArray{
					Data:  []float64{1, 2, 3, 4},
					Shape: falconcore.AxesInt{2, 2},
				},
			},
		},

		Metadata: falconcore.Metadata{
			"key": "value",
		},

		Title:     "test_measurement",
		Id:        123,
		Timestamp: 456,
	}
}

func TestNewHDF5Store(t *testing.T) {
	store := NewHDF5Store()

	require.NotNil(t, store)
}

func TestHDF5Store_Write_Error(t *testing.T) {
	store := NewHDF5Store()

	entry := testDatabaseEntry()

	// force DatabaseEntry.NewFalconDatabaseEntry() failure
	entry.DomainLabels = falconcore.AxesCoupledLabelledDomain{
		{
			{
				InstrumentPort: falconcore.InstrumentPort{
					Unit: falconcore.Unit(999),
				},
			},
		},
	}

	path := filepath.Join(
		t.TempDir(),
		"test.h5",
	)

	err := store.Write(path, entry)

	require.Error(t, err)
}

func TestHDF5Store_Read_FileMissing(t *testing.T) {
	store := NewHDF5Store()

	_, err := store.Read(
		filepath.Join(
			t.TempDir(),
			"does-not-exist.h5",
		),
	)

	require.Error(t, err)
}

func TestHDF5Store_WriteRead_RoundTrip(t *testing.T) {
	store := NewHDF5Store()

	expected := testDatabaseEntry()

	path := filepath.Join(
		t.TempDir(),
		"measurement.h5",
	)

	err := store.Write(path, expected)
	require.NoError(t, err)

	actual, err := store.Read(path)
	require.NoError(t, err)

	assert.Equal(t, expected, actual)
}
