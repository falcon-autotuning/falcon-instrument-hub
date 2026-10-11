package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeInstrumentPortReader struct {
	unitHandle *symbolunit.Handle
	unitErr    error

	defaultName    string
	defaultNameErr error

	instrumentName    string
	instrumentNameErr error

	instrumentType    instrument.Instrument
	instrumentTypeErr error

	description    string
	descriptionErr error

	pseudoName    *connection.Handle
	pseudoNameErr error

	access    access.Access
	accessErr error

	scope    scope.Scope
	scopeErr error

	characteristic    instrumentcharacteristic.InstrumentCharacteristic
	characteristicErr error

	portType    porttype.PortType
	portTypeErr error
}

func (f *fakeInstrumentPortReader) Units() (*symbolunit.Handle, error) {
	return f.unitHandle, f.unitErr
}

func (f *fakeInstrumentPortReader) DefaultName() (string, error) {
	return f.defaultName, f.defaultNameErr
}

func (f *fakeInstrumentPortReader) InstrumentName() (string, error) {
	return f.instrumentName, f.instrumentNameErr
}

func (f *fakeInstrumentPortReader) InstrumentType() (instrument.Instrument, error) {
	return f.instrumentType, f.instrumentTypeErr
}

func (f *fakeInstrumentPortReader) Description() (string, error) {
	return f.description, f.descriptionErr
}

func (f *fakeInstrumentPortReader) PseudoName() (*connection.Handle, error) {
	return f.pseudoName, f.pseudoNameErr
}

func (f *fakeInstrumentPortReader) Access() (access.Access, error) {
	return f.access, f.accessErr
}

func (f *fakeInstrumentPortReader) Scope() (scope.Scope, error) {
	return f.scope, f.scopeErr
}

func (f *fakeInstrumentPortReader) Characteristic() (instrumentcharacteristic.InstrumentCharacteristic, error) {
	return f.characteristic, f.characteristicErr
}

func (f *fakeInstrumentPortReader) Type() (porttype.PortType, error) {
	return f.portType, f.portTypeErr
}

var _ InstrumentPortReader = (*fakeInstrumentPortReader)(nil)

func TestInstrumentPort_NewFalconInstrumentPort(t *testing.T) {
	p := InstrumentPort{
		DefaultName:    "Vg",
		InstrumentName: "dac",
		InstrumentType: instrument.Instrument(0),
		Description:    "gate voltage",
		Unit:           Volt,
		PseudoName: Connection{
			Name: "P1",
			Type: PlungerGate,
		},
		Access:         access.Access(0),
		Scope:          scope.Scope(0),
		Characteristic: instrumentcharacteristic.InstrumentCharacteristic(0),
		PortType:       porttype.PortType(0),
	}

	handle, err := p.NewFalconInstrumentPort()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestInstrumentPort_NewFalconInstrumentPort_UnitError(t *testing.T) {
	handle, err := InstrumentPort{
		Unit: Unit(999),
	}.NewFalconInstrumentPort()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestInstrumentPortFromFalcon(t *testing.T) {
	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)

	defer unit.Close()

	port, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitHandle:     unit,
			defaultName:    "Vg",
			instrumentName: "dac",
			instrumentType: instrument.Instrument(0),
			description:    "gate",
			access:         access.Access(0),
			scope:          scope.Scope(0),
			characteristic: instrumentcharacteristic.InstrumentCharacteristic(0),
			portType:       porttype.PortType(0),
		},
	)

	require.NoError(t, err)

	assert.Equal(t, PortName("Vg"), port.DefaultName)
	assert.Equal(t, InstrumentName("dac"), port.InstrumentName)
	assert.Equal(t, Volt, port.Unit)
	assert.Equal(t, "gate", port.Description)
}

func TestInstrumentPortFromFalcon_UnitsError(t *testing.T) {
	_, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitErr: errors.New("unit error"),
		},
	)

	require.Error(t, err)
}

func TestInstrumentPortFromFalcon_DefaultNameError(t *testing.T) {
	unit, _ := symbolunit.NewVolt()
	defer unit.Close()

	_, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitHandle:     unit,
			defaultNameErr: errors.New("name error"),
		},
	)

	require.Error(t, err)
}

func TestInstrumentPortFromFalcon_InstrumentNameError(t *testing.T) {
	unit, _ := symbolunit.NewVolt()
	defer unit.Close()

	_, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitHandle:        unit,
			defaultName:       "P1",
			instrumentNameErr: errors.New("instrument name error"),
		},
	)

	require.Error(t, err)
}

func TestInstrumentPortFromFalcon_InstrumentTypeError(t *testing.T) {
	unit, _ := symbolunit.NewVolt()
	defer unit.Close()

	_, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitHandle:        unit,
			defaultName:       "P1",
			instrumentName:    "dac",
			instrumentTypeErr: errors.New("type error"),
		},
	)

	require.Error(t, err)
}

func TestInstrumentPortFromFalcon_DescriptionError(t *testing.T) {
	unit, _ := symbolunit.NewVolt()
	defer unit.Close()

	_, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitHandle:     unit,
			defaultName:    "P1",
			instrumentName: "dac",
			descriptionErr: errors.New("description error"),
		},
	)

	require.Error(t, err)
}

func TestInstrumentPortFromFalcon_AccessError(t *testing.T) {
	unit, _ := symbolunit.NewVolt()
	defer unit.Close()

	_, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitHandle:     unit,
			defaultName:    "P1",
			instrumentName: "dac",
			accessErr:      errors.New("access error"),
		},
	)

	require.Error(t, err)
}

func TestInstrumentPortFromFalcon_ScopeError(t *testing.T) {
	unit, _ := symbolunit.NewVolt()
	defer unit.Close()

	_, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitHandle:     unit,
			defaultName:    "P1",
			instrumentName: "dac",
			scopeErr:       errors.New("scope error"),
		},
	)

	require.Error(t, err)
}

func TestInstrumentPortFromFalcon_CharacteristicError(t *testing.T) {
	unit, _ := symbolunit.NewVolt()
	defer unit.Close()

	_, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitHandle:        unit,
			defaultName:       "P1",
			instrumentName:    "dac",
			characteristicErr: errors.New("characteristic error"),
		},
	)

	require.Error(t, err)
}

func TestInstrumentPortFromFalcon_TypeError(t *testing.T) {
	unit, _ := symbolunit.NewVolt()
	defer unit.Close()

	_, err := InstrumentPortFromFalcon(
		&fakeInstrumentPortReader{
			unitHandle:     unit,
			defaultName:    "P1",
			instrumentName: "dac",
			portTypeErr:    errors.New("port type error"),
		},
	)

	require.Error(t, err)
}
