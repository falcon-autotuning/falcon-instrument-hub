//go:build cgo

package interpreter

import (
	"testing"

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
	"github.com/stretchr/testify/require"
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

	volts := mustCreateTestHandle(t, symbolunit.NewVolt)
	p1 := mustCreateTestHandle(t, func() (*connection.Handle, error) {
		return connection.NewPlungerGate("P1")
	})
	setter := mustCreateTestHandle(t, func() (*instrumentport.Handle, error) {
		return instrumentport.NewKnob(
			"P1",
			"Source1",
			p1,
			instrument.DcVoltageSource,
			volts,
			"mock P1 setter",
		)
	})
	o1 := mustCreateTestHandle(t, func() (*connection.Handle, error) {
		return connection.NewOhmic("O1")
	})
	getter := mustCreateTestHandle(t, func() (*instrumentport.Handle, error) {
		return instrumentport.NewMeter(
			"O1",
			"Meter1",
			o1,
			instrument.Voltmeter,
			volts,
			"mock O1 getter",
		)
	})

	setterDomain := mustCreateTestHandle(t, func() (*domain.Handle, error) {
		return domain.New(0, 1, true, true)
	})
	labelledSetterDomain := mustCreateTestHandle(t, func() (*labelleddomain.Handle, error) {
		return labelleddomain.NewFromPortAndDomain(setter, setterDomain)
	})
	coupledDomain := mustCreateTestHandle(t, func() (*coupledlabelleddomain.Handle, error) {
		return coupledlabelleddomain.New([]*labelleddomain.Handle{labelledSetterDomain})
	})
	domainAxes := mustCreateTestHandle(t, func() (*axescoupledlabelleddomain.Handle, error) {
		return axescoupledlabelleddomain.New([]*coupledlabelleddomain.Handle{coupledDomain})
	})

	increasingPair := mustCreateTestHandle(t, func() (*pairstringbool.Handle, error) {
		return pairstringbool.New("P1", true)
	})
	increasing := mustCreateTestHandle(t, func() (*mapstringbool.Handle, error) {
		return mapstringbool.New([]*pairstringbool.Handle{increasingPair})
	})
	increasingAxes := mustCreateTestHandle(t, func() (*axesmapstringbool.Handle, error) {
		return axesmapstringbool.New([]*mapstringbool.Handle{increasing})
	})

	cartesian := mustCreateTestHandle(t, func() (*discretizer.Handle, error) {
		return discretizer.NewCartesianDiscretizer(1)
	})
	discretizerAxes := mustCreateTestHandle(t, func() (*axesdiscretizer.Handle, error) {
		return axesdiscretizer.New([]*discretizer.Handle{cartesian})
	})
	unitSpace := mustCreateTestHandle(t, func() (*unitspace.Handle, error) {
		return unitspace.New(discretizerAxes, setterDomain)
	})
	space := mustCreateTestHandle(t, func() (*discretespace.Handle, error) {
		return discretespace.New(unitSpace, domainAxes, increasingAxes)
	})

	identity := mustCreateTestHandle(t, func() (*porttransform.Handle, error) {
		return porttransform.NewIdentityTransform(setter)
	})
	transforms := mustCreateTestHandle(t, func() (*listporttransform.Handle, error) {
		return listporttransform.New([]*porttransform.Handle{identity})
	})
	setWaveform := mustCreateTestHandle(t, func() (*waveform.Handle, error) {
		return waveform.New(space, transforms)
	})
	waveforms := mustCreateTestHandle(t, func() (*listwaveform.Handle, error) {
		return listwaveform.New([]*waveform.Handle{setWaveform})
	})
	getters := mustCreateTestHandle(t, func() (*ports.Handle, error) {
		return ports.New([]*instrumentport.Handle{getter})
	})
	meterTransforms := mustCreateTestHandle(t, mapinstrumentportporttransform.NewEmpty)
	clock := mustCreateTestHandle(t, instrumentport.NewExecutionClock)
	timeDomain := mustCreateTestHandle(t, func() (*labelleddomain.Handle, error) {
		return labelleddomain.NewFromPort(0, 1, clock, true, true)
	})
	request := mustCreateTestHandle(t, func() (*measurementrequest.Handle, error) {
		return measurementrequest.New(
			"set P1 and measure O1",
			waveforms,
			getters,
			meterTransforms,
			timeDomain,
		)
	})

	return &FalconMeasurementRequest{handle: request}
}

func TestMeasureGetSetCanHandleP1SetterAndO1Getter(t *testing.T) {
	request := newP1O1MeasurementRequest(t)

	matches, err := (&measureGetSetHandler{}).CanHandle(request)

	require.NoError(t, err)
	require.True(t, matches)
}
