package falconcore

import (
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/device-structures/connection"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
)

type (
	PortName       string
	InstrumentName string
)

type InstrumentPort struct {
	DefaultName    PortName
	InstrumentName InstrumentName
	InstrumentType instrument.Instrument
	Description    string
	Unit           Unit
	PseudoName     Connection
	Access         access.Access
	Scope          scope.Scope
	Characteristic instrumentcharacteristic.InstrumentCharacteristic
	PortType       porttype.PortType
}

func (i InstrumentPort) NewFalconInstrumentPort() (*instrumentport.Handle, error) {
	conn, _ := i.PseudoName.NewFalconConnection() // Since Connection is not necessarily required
	unit, err := i.Unit.NewSymbolUnit()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	defer unit.Close()
	return instrumentport.NewPort(string(i.DefaultName), string(i.InstrumentName), i.Scope, i.Access, i.Characteristic, i.PortType, conn, i.InstrumentType, unit, i.Description)
}

type InstrumentPortReader interface {
	Units() (*symbolunit.Handle, error)
	DefaultName() (string, error)
	InstrumentName() (string, error)
	InstrumentType() (instrument.Instrument, error)
	Description() (string, error)
	PseudoName() (*connection.Handle, error)
	Access() (access.Access, error)
	Scope() (scope.Scope, error)
	Characteristic() (instrumentcharacteristic.InstrumentCharacteristic, error)
	Type() (porttype.PortType, error)
}

var _ InstrumentPortReader = (*instrumentport.Handle)(nil)

func InstrumentPortFromFalcon(i InstrumentPortReader) (InstrumentPort, error) {
	var out InstrumentPort
	realunit, err := i.Units()
	if err != nil {
		return out, err
	}
	out.Unit, err = SymbolFromFalcon(realunit)
	if err != nil {
		return out, err
	}
	defer realunit.Close()
	name, err := i.DefaultName()
	if err != nil {
		return out, err
	}
	out.DefaultName = PortName(name)
	instname, err := i.InstrumentName()
	if err != nil {
		return out, err
	}
	out.InstrumentName = InstrumentName(instname)
	out.InstrumentType, err = i.InstrumentType()
	if err != nil {
		return out, err
	}
	out.Description, err = i.Description()
	if err != nil {
		return out, err
	}
	out.Scope, err = i.Scope()
	if err != nil {
		return out, err
	}
	// Global ports such as ExecutionClock have no device connection.
	// Reading their pseudo-name would raise a C-API error.
	if out.Scope == scope.Local {
		conn, err := i.PseudoName()
		if err != nil {
			return out, err
		}
		if conn != nil {
			defer conn.Close()
			out.PseudoName, err = ConnectionFromFalcon(conn)
			if err != nil {
				return out, err
			}
		}
	}
	out.Access, err = i.Access()
	if err != nil {
		return out, err
	}
	out.Characteristic, err = i.Characteristic()
	if err != nil {
		return out, err
	}
	out.PortType, err = i.Type()
	return out, err
}
