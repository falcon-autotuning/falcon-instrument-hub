package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportquantity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSettingResponseReader struct {
	gettersFunc func() (*mapinstrumentportquantity.Handle, error)
	messageFunc func() (string, error)
}

func (m mockSettingResponseReader) Getters() (
	*mapinstrumentportquantity.Handle,
	error,
) {
	return m.gettersFunc()
}

func (m mockSettingResponseReader) Message() (
	string,
	error,
) {
	return m.messageFunc()
}

func TestSettingResponse_NewFalconSettingResponse(
	t *testing.T,
) {
	resp := SettingResponse{
		Message: "success",
		Getters: map[InstrumentPort]Quantity{
			{
				DefaultName:    "gate",
				InstrumentName: "dac",
				Unit:           Volt,
			}: {
				Value: 1.23,
				Unit:  Volt,
			},
		},
	}

	handle, err := resp.NewFalconSettingResponse()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestSettingResponse_NewFalconSettingResponse_BadPort(
	t *testing.T,
) {
	resp := SettingResponse{
		Getters: map[InstrumentPort]Quantity{
			{
				Unit: Unit(999),
			}: {
				Value: 1,
				Unit:  Volt,
			},
		},
	}

	_, err := resp.NewFalconSettingResponse()

	require.Error(t, err)
}

func TestSettingResponse_NewFalconSettingResponse_BadQuantity(
	t *testing.T,
) {
	resp := SettingResponse{
		Getters: map[InstrumentPort]Quantity{
			{
				DefaultName:    "gate",
				InstrumentName: "dac",
				Unit:           Volt,
			}: {
				Value: 1,
				Unit:  Unit(999),
			},
		},
	}

	_, err := resp.NewFalconSettingResponse()

	require.Error(t, err)
}

func TestSettingResponseFromFalcon_GettersError(
	t *testing.T,
) {
	_, err := SettingResponseFromFalcon(
		mockSettingResponseReader{
			gettersFunc: func() (
				*mapinstrumentportquantity.Handle,
				error,
			) {
				return nil, errors.New("boom")
			},
			messageFunc: func() (string, error) {
				return "", nil
			},
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestSettingResponseFromFalcon_MessageError(
	t *testing.T,
) {
	getters, err := mapinstrumentportquantity.NewEmpty()
	require.NoError(t, err)
	defer getters.Close()

	_, err = SettingResponseFromFalcon(
		mockSettingResponseReader{
			gettersFunc: func() (
				*mapinstrumentportquantity.Handle,
				error,
			) {
				return getters, nil
			},
			messageFunc: func() (string, error) {
				return "", errors.New("boom")
			},
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestSettingResponseFromFalcon_Success(
	t *testing.T,
) {
	inPort := InstrumentPort{
		DefaultName:    "getter",
		InstrumentName: "dac",
		Unit:           Volt,
	}

	expected := SettingResponse{
		Message: "ok",
		Getters: map[InstrumentPort]Quantity{
			inPort: {
				Value: 12.5,
				Unit:  Volt,
			},
		},
	}

	handle, err := expected.NewFalconSettingResponse()
	require.NoError(t, err)
	defer handle.Close()

	var reader SettingResponseReader = handle

	actual, err := SettingResponseFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(
		t,
		expected.Message,
		actual.Message,
	)

	require.Len(
		t,
		actual.Getters,
		1,
	)

	for port, quantity := range actual.Getters {
		assert.Equal(
			t,
			PortName("getter"),
			port.DefaultName,
		)

		assert.InDelta(
			t,
			12.5,
			quantity.Value,
			1e-9,
		)

		assert.Equal(
			t,
			Volt,
			quantity.Unit,
		)
	}
}
