//go:build cgo

package interpreter

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/listwaveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportporttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapstringbool"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/pairstringbool"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/port-transforms/porttransform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/waveform"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescoupledlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesdiscretizer"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesmapstringbool"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretespace"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretizer"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/coupledlabelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/domain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/labelleddomain"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/unitspace"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
)

type testFalconHandle interface {
	Close() error
}

func mustCreateTestHandle[T testFalconHandle](
	t *testing.T,
	create func() (T, error),
) T {
	t.Helper()
	handle, err := create()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, handle.Close())
	})
	return handle
}

func newP1O1MeasurementRequest(t *testing.T) *FalconMeasurementRequest {
	t.Helper()
	return newSetGetMeasurementRequest(t, []float64{0.25}, []int{1})
}

// Each voltage sets P1, P2, ... once; getterChannels selects O1, O2, ... .
func newSetGetMeasurementRequest(
	t *testing.T,
	voltages []float64,
	getterChannels []int,
) *FalconMeasurementRequest {
	t.Helper()
	volts := mustCreateTestHandle(t, symbolunit.NewVolt)
	var domains []*labelleddomain.Handle
	var identities []*porttransform.Handle
	var directions []*pairstringbool.Handle
	for i, voltage := range voltages {
		name := fmt.Sprintf("P%d", i+1)
		conn := mustCreateTestHandle(
			t,
			func() (*connection.Handle, error) {
				return connection.NewPlungerGate(name)
			},
		)
		setter := mustCreateTestHandle(
			t,
			func() (*instrumentport.Handle, error) {
				return instrumentport.NewKnob(
					name,
					"Source1",
					conn,
					instrument.DcVoltageSource,
					volts,
					"test source",
				)
			},
		)
		domains = append(
			domains,
			mustCreateTestHandle(
				t,
				func() (*labelleddomain.Handle, error) {
					return labelleddomain.NewFromPort(
						voltage,
						voltage,
						setter,
						true,
						true,
					)
				},
			),
		)
		identities = append(
			identities,
			mustCreateTestHandle(
				t,
				func() (*porttransform.Handle, error) {
					return porttransform.NewIdentityTransform(
						setter,
					)
				},
			),
		)
		directions = append(
			directions,
			mustCreateTestHandle(
				t,
				func() (*pairstringbool.Handle, error) {
					return pairstringbool.New(name, true)
				},
			),
		)
	}
	var meterPorts []*instrumentport.Handle
	for _, channel := range getterChannels {
		name := fmt.Sprintf("O%d", channel)
		conn := mustCreateTestHandle(
			t,
			func() (*connection.Handle, error) {
				return connection.NewOhmic(name)
			},
		)
		meterPorts = append(
			meterPorts,
			mustCreateTestHandle(
				t,
				func() (*instrumentport.Handle, error) {
					return instrumentport.NewMeter(
						name,
						"Meter1",
						conn,
						instrument.Voltmeter,
						volts,
						"test meter",
					)
				},
			),
		)
	}
	// The native library requires two control coordinates. Both map to the
	// same fixed voltage above: this describes a DC setting, not a sweep.
	setterDomain := mustCreateTestHandle(t, func() (*domain.Handle, error) {
		return domain.New(0, 1, true, true)
	})
	coupledDomain := mustCreateTestHandle(
		t,
		func() (*coupledlabelleddomain.Handle, error) {
			return coupledlabelleddomain.New(
				domains,
			)
		},
	)
	domainAxes := mustCreateTestHandle(
		t,
		func() (*axescoupledlabelleddomain.Handle, error) {
			return axescoupledlabelleddomain.New(
				[]*coupledlabelleddomain.Handle{coupledDomain},
			)
		},
	)

	increasing := mustCreateTestHandle(
		t,
		func() (*mapstringbool.Handle, error) {
			return mapstringbool.New(
				directions,
			)
		},
	)
	increasingAxes := mustCreateTestHandle(
		t,
		func() (*axesmapstringbool.Handle, error) {
			return axesmapstringbool.New(
				[]*mapstringbool.Handle{increasing},
			)
		},
	)

	cartesian := mustCreateTestHandle(
		t,
		func() (*discretizer.Handle, error) {
			return discretizer.NewCartesianDiscretizer(1)
		},
	)
	discretizerAxes := mustCreateTestHandle(
		t,
		func() (*axesdiscretizer.Handle, error) {
			return axesdiscretizer.New(
				[]*discretizer.Handle{cartesian},
			)
		},
	)
	unitSpace := mustCreateTestHandle(t, func() (*unitspace.Handle, error) {
		return unitspace.New(discretizerAxes, setterDomain)
	})
	space := mustCreateTestHandle(t, func() (*discretespace.Handle, error) {
		return discretespace.New(unitSpace, domainAxes, increasingAxes)
	})

	transforms := mustCreateTestHandle(
		t,
		func() (*listporttransform.Handle, error) {
			return listporttransform.New(
				identities,
			)
		},
	)
	setWaveform := mustCreateTestHandle(
		t,
		func() (*waveform.Handle, error) {
			return waveform.New(space, transforms)
		},
	)
	waveforms := mustCreateTestHandle(
		t,
		func() (*listwaveform.Handle, error) {
			return listwaveform.New([]*waveform.Handle{setWaveform})
		},
	)
	getters := mustCreateTestHandle(t, func() (*ports.Handle, error) {
		return ports.New(meterPorts)
	})
	meterTransforms := mustCreateTestHandle(
		t,
		mapinstrumentportporttransform.NewEmpty,
	)
	clock := mustCreateTestHandle(t, instrumentport.NewExecutionClock)
	timeDomain := mustCreateTestHandle(
		t,
		func() (*labelleddomain.Handle, error) {
			return labelleddomain.NewFromPort(
				0,
				1,
				clock,
				true,
				true,
			)
		},
	)
	request := mustCreateTestHandle(
		t,
		func() (*measurementrequest.Handle, error) {
			return measurementrequest.New(
				"set voltages and read meters",
				waveforms,
				getters,
				meterTransforms,
				timeDomain,
			)
		},
	)

	return &FalconMeasurementRequest{handle: request}
}

func TestMeasureGetSetCanHandleP1SetterAndO1Getter(t *testing.T) {
	request := newP1O1MeasurementRequest(t)

	matches, err := (&measureGetSetHandler{}).CanHandle(request)

	require.NoError(t, err)
	require.True(t, matches)
}
