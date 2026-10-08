package devicestate

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/physics/units/symbolunit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeQuantityReader struct {
	value    float64
	valueErr error
	unit     *symbolunit.Handle
	unitErr  error
}

func (f *fakeQuantityReader) Value() (float64, error) {
	return f.value, f.valueErr
}

func (f *fakeQuantityReader) Unit() (*symbolunit.Handle, error) {
	return f.unit, f.unitErr
}

func TestQuantityFromFalcon_Success(t *testing.T) {
	unit, err := symbolunit.NewVolt()
	require.NoError(t, err)
	defer unit.Close()

	reader := &fakeQuantityReader{
		value: 123.45,
		unit:  unit,
	}

	q, err := QuantityFromFalcon(reader)

	require.NoError(t, err)
	assert.Equal(t, 123.45, q.Value)
	assert.Equal(t, Volt, q.Unit)
}

func TestQuantityFromFalcon_ValueError(t *testing.T) {
	reader := &fakeQuantityReader{
		valueErr: errors.New("value error"),
	}

	q, err := QuantityFromFalcon(reader)

	require.Error(t, err)
	assert.EqualError(t, err, "value error")
	assert.Equal(t, Quantity{}, q)
}

func TestQuantityFromFalcon_UnitError(t *testing.T) {
	reader := &fakeQuantityReader{
		value:   1.0,
		unitErr: errors.New("unit error"),
	}

	q, err := QuantityFromFalcon(reader)

	require.Error(t, err)
	assert.EqualError(t, err, "unit error")
	assert.Equal(t, Quantity{}, q)
}

func TestQuantity_NewFalconQuantity(t *testing.T) {
	raw := Quantity{
		Value: 5.5,
		Unit:  Volt,
	}

	handle, err := raw.NewFalconQuantity()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	value, err := handle.Value()
	require.NoError(t, err)

	unit, err := handle.Unit()
	require.NoError(t, err)

	symbol, err := unit.Symbol()
	require.NoError(t, err)

	assert.Equal(t, 5.5, value)
	assert.Equal(t, "V", symbol)
}

func TestQuantity_NewFalconQuantity_InvalidUnit(t *testing.T) {
	raw := Quantity{
		Value: 1.0,
		Unit:  Unit(999999),
	}

	handle, err := raw.NewFalconQuantity()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestQuantity_RoundTrip(t *testing.T) {
	original := Quantity{
		Value: 42.0,
		Unit:  Volt,
	}

	handle, err := original.NewFalconQuantity()
	require.NoError(t, err)
	defer handle.Close()

	result, err := QuantityFromFalcon(handle)
	require.NoError(t, err)

	assert.Equal(t, original, result)
}
