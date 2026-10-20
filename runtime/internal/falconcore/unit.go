package falconcore

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
)

type Unit int

const (
	Meter Unit = iota
	Kilogram
	Second
	Ampere
	Kelvin
	Mole
	Candela
	Hertz
	Newton
	Pascal
	Joule
	Watt
	Coulomb
	Volt
	Farad
	Ohm
	Siemens
	Weber
	Tesla
	Henry
	Dimensionless
	Percent
	Radian
	Kilometer
	Millimeter
	Millivolt
	Kilovolt
	Milliampere
	Microampere
	Nanoampere
	Picoampere
	Millisecond
	Microsecond
	Nanosecond
	Picosecond
	Milliohm
	Kiloohm
	Megaohm
	Millihertz
	Kilohertz
	Megahertz
	Gigahertz
	MetersPerSecond
	MetersPerSecondSquared
	NewtonsPerMeter
	VoltsPerMeter
	VoltsPerSecond
	AmperesPerMeter
	WattsPerMeterKelvin
)

var symbolToUnit = map[string]Unit{
	"m":             Meter,
	"kg":            Kilogram,
	"s":             Second,
	"A":             Ampere,
	"K":             Kelvin,
	"mol":           Mole,
	"cd":            Candela,
	"Hz":            Hertz,
	"N":             Newton,
	"Pa":            Pascal,
	"J":             Joule,
	"W":             Watt,
	"C":             Coulomb,
	"V":             Volt,
	"F":             Farad,
	"Ω":             Ohm,
	"S":             Siemens,
	"Wb":            Weber,
	"T":             Tesla,
	"H":             Henry,
	"%":             Percent,
	"rad":           Radian,
	"km":            Kilometer,
	"mm":            Millimeter,
	"mV":            Millivolt,
	"kV":            Kilovolt,
	"mA":            Milliampere,
	"μA":            Microampere,
	"nA":            Nanoampere,
	"pA":            Picoampere,
	"ms":            Millisecond,
	"μs":            Microsecond,
	"ns":            Nanosecond,
	"ps":            Picosecond,
	"mΩ":            Milliohm,
	"kΩ":            Kiloohm,
	"MΩ":            Megaohm,
	"mHz":           Millihertz,
	"kHz":           Kilohertz,
	"MHz":           Megahertz,
	"GHz":           Gigahertz,
	"m/s":           MetersPerSecond,
	"m·s^-2":        MetersPerSecondSquared,
	"kg·s^-2":       NewtonsPerMeter,
	"m·kg·A·s^-3":   VoltsPerMeter,
	"m^2·kg·A·s^-4": VoltsPerSecond,
	"A/m":           AmperesPerMeter,
	"m·kg·K·s^-3":   WattsPerMeterKelvin,
	"":              Dimensionless,
}

func (u Unit) NewSymbolUnit() (*symbolunit.Handle, error) {
	switch u {
	case Meter:
		return symbolunit.NewMeter()
	case Kilogram:
		return symbolunit.NewKilogram()
	case Second:
		return symbolunit.NewSecond()
	case Ampere:
		return symbolunit.NewAmpere()
	case Kelvin:
		return symbolunit.NewKelvin()
	case Mole:
		return symbolunit.NewMole()
	case Candela:
		return symbolunit.NewCandela()
	case Hertz:
		return symbolunit.NewHertz()
	case Newton:
		return symbolunit.NewNewton()
	case Pascal:
		return symbolunit.NewPascal()
	case Joule:
		return symbolunit.NewJoule()
	case Watt:
		return symbolunit.NewWatt()
	case Coulomb:
		return symbolunit.NewCoulomb()
	case Volt:
		return symbolunit.NewVolt()
	case Farad:
		return symbolunit.NewFarad()
	case Ohm:
		return symbolunit.NewOhm()
	case Siemens:
		return symbolunit.NewSiemens()
	case Weber:
		return symbolunit.NewWeber()
	case Tesla:
		return symbolunit.NewTesla()
	case Henry:
		return symbolunit.NewHenry()
	case Dimensionless:
		return symbolunit.NewDimensionless()
	case Percent:
		return symbolunit.NewPercent()
	case Radian:
		return symbolunit.NewRadian()
	case Kilometer:
		return symbolunit.NewKilometer()
	case Millimeter:
		return symbolunit.NewMillimeter()
	case Millivolt:
		return symbolunit.NewMillivolt()
	case Kilovolt:
		return symbolunit.NewKilovolt()
	case Milliampere:
		return symbolunit.NewMilliampere()
	case Microampere:
		return symbolunit.NewMicroampere()
	case Nanoampere:
		return symbolunit.NewNanoampere()
	case Picoampere:
		return symbolunit.NewPicoampere()
	case Millisecond:
		return symbolunit.NewMillisecond()
	case Microsecond:
		return symbolunit.NewMicrosecond()
	case Nanosecond:
		return symbolunit.NewNanosecond()
	case Picosecond:
		return symbolunit.NewPicosecond()
	case Milliohm:
		return symbolunit.NewMilliohm()
	case Kiloohm:
		return symbolunit.NewKiloohm()
	case Megaohm:
		return symbolunit.NewMegaohm()
	case Millihertz:
		return symbolunit.NewMillihertz()
	case Kilohertz:
		return symbolunit.NewKilohertz()
	case Megahertz:
		return symbolunit.NewMegahertz()
	case Gigahertz:
		return symbolunit.NewGigahertz()
	case MetersPerSecond:
		return symbolunit.NewMetersPerSecond()
	case MetersPerSecondSquared:
		return symbolunit.NewMetersPerSecondSquared()
	case NewtonsPerMeter:
		return symbolunit.NewNewtonsPerMeter()
	case VoltsPerMeter:
		return symbolunit.NewVoltsPerMeter()
	case VoltsPerSecond:
		return symbolunit.NewVoltsPerSecond()
	case AmperesPerMeter:
		return symbolunit.NewAmperesPerMeter()
	case WattsPerMeterKelvin:
		return symbolunit.NewWattsPerMeterKelvin()
	default:
		return nil, fmt.Errorf("unsupported unit: %d", u)
	}
}

func SymbolToUnit(s string) (Unit, error) {
	unit, ok := symbolToUnit[s]
	if ok {
		return unit, nil
	}
	return 0, fmt.Errorf("unsupported unit symbol %q", s)
}

func SymbolFromFalcon(s *symbolunit.Handle) (Unit, error) {
	symbol, err := s.Symbol()
	if err != nil {
		return 0, err
	}
	return SymbolToUnit(symbol)
}
