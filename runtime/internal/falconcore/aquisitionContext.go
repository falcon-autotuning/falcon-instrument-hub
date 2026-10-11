package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/autotuner-interfaces/contexts/acquisitioncontext"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
)

type AcquisitionContext struct {
	Connection
	Unit
	InstrumentType instrument.Instrument
}

func (a AcquisitionContext) NewFalconAcquisitionContext() (*acquisitioncontext.Handle, error) {
	conn, err := a.NewFalconConnection()
	if err != nil {
		return nil, err
	}
	unit, err := a.NewSymbolUnit()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	defer unit.Close()
	return acquisitioncontext.New(conn, a.InstrumentType, unit)
}

type AcquisitionContextReader interface {
	Connection() (*connection.Handle, error)
	Units() (*symbolunit.Handle, error)
	InstrumentType() (instrument.Instrument, error)
}

var _ AcquisitionContextReader = (*acquisitioncontext.Handle)(nil)

func AcquisitionContextFromFalcon(a AcquisitionContextReader) (AcquisitionContext, error) {
	var out AcquisitionContext
	conn, err := a.Connection()
	if err != nil {
		return AcquisitionContext{}, err
	}
	out.Connection, err = ConnectionFromFalcon(conn)
	defer conn.Close()
	if err != nil {
		return AcquisitionContext{}, err
	}
	unit, err := a.Units()
	if err != nil {
		return AcquisitionContext{}, err
	}
	out.Unit, err = SymbolFromFalcon(unit)
	defer unit.Close()
	if err != nil {
		return AcquisitionContext{}, err
	}
	out.InstrumentType, err = a.InstrumentType()
	return out, err
}
