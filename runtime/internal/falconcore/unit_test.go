package falconcore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_RoundTrip(t *testing.T) {
	allUnits := []Unit{
		Meter,
		Kilogram,
		Second,
		Ampere,
		Kelvin,
		Mole,
		Candela,
		Hertz,
		Newton,
		Pascal,
		Joule,
		Watt,
		Coulomb,
		Volt,
		Farad,
		Ohm,
		Siemens,
		Weber,
		Tesla,
		Henry,
		Dimensionless,
		Percent,
		Radian,
		Kilometer,
		Millimeter,
		Millivolt,
		Kilovolt,
		Milliampere,
		Microampere,
		Nanoampere,
		Picoampere,
		Millisecond,
		Microsecond,
		Nanosecond,
		Picosecond,
		Milliohm,
		Kiloohm,
		Megaohm,
		Millihertz,
		Kilohertz,
		Megahertz,
		Gigahertz,
		MetersPerSecond,
		MetersPerSecondSquared,
		NewtonsPerMeter,
		VoltsPerMeter,
		VoltsPerSecond,
		AmperesPerMeter,
		WattsPerMeterKelvin,
	}

	for _, expected := range allUnits {
		t.Run(funcName(expected), func(t *testing.T) {
			handle, err := expected.NewSymbolUnit()

			require.NoError(t, err)
			require.NotNil(t, handle)

			defer func() {
				assert.NoError(t, handle.Close())
			}()

			actual, err := SymbolFromFalcon(handle)

			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestUnit_NewSymbolUnit_Invalid(t *testing.T) {
	handle, err := Unit(999999).NewSymbolUnit()

	require.Error(t, err)
	assert.Nil(t, handle)
	assert.Contains(t, err.Error(), "unsupported unit")
}

func TestSymbolFromFalcon_AllMappedSymbols(t *testing.T) {
	for symbol, expectedUnit := range symbolToUnit {
		t.Run(symbol, func(t *testing.T) {
			handle, err := expectedUnit.NewSymbolUnit()

			require.NoError(t, err)
			require.NotNil(t, handle)

			defer func() {
				assert.NoError(t, handle.Close())
			}()

			actualUnit, err := SymbolFromFalcon(handle)

			require.NoError(t, err)
			assert.Equal(t, expectedUnit, actualUnit)
		})
	}
}

func funcName(u Unit) string {
	switch u {
	case Meter:
		return "Meter"
	case Kilogram:
		return "Kilogram"
	case Second:
		return "Second"
	case Ampere:
		return "Ampere"
	case Kelvin:
		return "Kelvin"
	case Mole:
		return "Mole"
	case Candela:
		return "Candela"
	case Hertz:
		return "Hertz"
	case Newton:
		return "Newton"
	case Pascal:
		return "Pascal"
	case Joule:
		return "Joule"
	case Watt:
		return "Watt"
	case Coulomb:
		return "Coulomb"
	case Volt:
		return "Volt"
	case Farad:
		return "Farad"
	case Ohm:
		return "Ohm"
	case Siemens:
		return "Siemens"
	case Weber:
		return "Weber"
	case Tesla:
		return "Tesla"
	case Henry:
		return "Henry"
	case Dimensionless:
		return "Dimensionless"
	case Percent:
		return "Percent"
	case Radian:
		return "Radian"
	case Kilometer:
		return "Kilometer"
	case Millimeter:
		return "Millimeter"
	case Millivolt:
		return "Millivolt"
	case Kilovolt:
		return "Kilovolt"
	case Milliampere:
		return "Milliampere"
	case Microampere:
		return "Microampere"
	case Nanoampere:
		return "Nanoampere"
	case Picoampere:
		return "Picoampere"
	case Millisecond:
		return "Millisecond"
	case Microsecond:
		return "Microsecond"
	case Nanosecond:
		return "Nanosecond"
	case Picosecond:
		return "Picosecond"
	case Milliohm:
		return "Milliohm"
	case Kiloohm:
		return "Kiloohm"
	case Megaohm:
		return "Megaohm"
	case Millihertz:
		return "Millihertz"
	case Kilohertz:
		return "Kilohertz"
	case Megahertz:
		return "Megahertz"
	case Gigahertz:
		return "Gigahertz"
	case MetersPerSecond:
		return "MetersPerSecond"
	case MetersPerSecondSquared:
		return "MetersPerSecondSquared"
	case NewtonsPerMeter:
		return "NewtonsPerMeter"
	case VoltsPerMeter:
		return "VoltsPerMeter"
	case VoltsPerSecond:
		return "VoltsPerSecond"
	case AmperesPerMeter:
		return "AmperesPerMeter"
	case WattsPerMeterKelvin:
		return "WattsPerMeterKelvin"
	default:
		return "Unknown"
	}
}
