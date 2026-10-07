package testutil

import (
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportquantity"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/quantity"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/stretchr/testify/require"
)

func NewVoltQuantity(
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

func BuildRealFalconRequest(
	t *testing.T,
) string {
	t.Helper()

	knob := NewTestKnobPort(t)
	defer knob.Close()

	meter := NewTestMeterPort(t)
	defer meter.Close()

	q := NewVoltQuantity(t, 5)
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

func NewTestKnobPort(
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

func NewTestMeterPort(
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
