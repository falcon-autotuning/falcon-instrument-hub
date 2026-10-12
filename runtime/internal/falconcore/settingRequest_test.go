package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportquantity"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingRequest_NewFalconSettingRequest(t *testing.T) {
	req := SettingRequest{
		Message: "hello",
		Getters: Ports{
			InstrumentPort{
				DefaultName:    "gate",
				InstrumentName: "dac",
				Unit:           Volt,
			},
		},
		Setters: map[InstrumentPort]Quantity{
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

	handle, err := req.NewFalconSettingRequest()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestSettingRequest_NewFalconSettingRequest_BadPort(
	t *testing.T,
) {
	req := SettingRequest{
		Setters: map[InstrumentPort]Quantity{
			{
				Unit: Unit(999),
			}: {
				Value: 1,
				Unit:  Volt,
			},
		},
	}

	_, err := req.NewFalconSettingRequest()

	require.Error(t, err)
}

func TestSettingRequest_NewFalconSettingRequest_BadQuantity(
	t *testing.T,
) {
	req := SettingRequest{
		Setters: map[InstrumentPort]Quantity{
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

	_, err := req.NewFalconSettingRequest()

	require.Error(t, err)
}

func TestSettingRequest_NewFalconSettingRequest_BadGetter(
	t *testing.T,
) {
	req := SettingRequest{
		Getters: Ports{
			{
				Unit: Unit(999),
			},
		},
	}

	_, err := req.NewFalconSettingRequest()

	require.Error(t, err)
}

type mockSettingRequestReader struct {
	setters func() (*mapinstrumentportquantity.Handle, error)
	getters func() (*ports.Handle, error)
	message func() (string, error)
}

func (m mockSettingRequestReader) Setters() (
	*mapinstrumentportquantity.Handle,
	error,
) {
	return m.setters()
}

func (m mockSettingRequestReader) Getters() (
	*ports.Handle,
	error,
) {
	return m.getters()
}

func (m mockSettingRequestReader) Message() (
	string,
	error,
) {
	return m.message()
}

func TestSettingRequestFromFalcon_SettersError(
	t *testing.T,
) {
	_, err := SettingRequestFromFalcon(
		mockSettingRequestReader{
			setters: func() (
				*mapinstrumentportquantity.Handle,
				error,
			) {
				return nil, errors.New("boom")
			},
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestSettingRequestFromFalcon_MessageError(
	t *testing.T,
) {
	setters, err := mapinstrumentportquantity.NewEmpty()
	require.NoError(t, err)

	getters, err := ports.NewEmpty()
	require.NoError(t, err)

	defer setters.Close()
	defer getters.Close()

	_, err = SettingRequestFromFalcon(
		mockSettingRequestReader{
			setters: func() (
				*mapinstrumentportquantity.Handle,
				error,
			) {
				return setters, nil
			},
			getters: func() (
				*ports.Handle,
				error,
			) {
				return getters, nil
			},
			message: func() (string, error) {
				return "", errors.New("boom")
			},
		},
	)

	require.Error(t, err)
}

func TestSettingRequestFromFalcon_GettersError(
	t *testing.T,
) {
	setters, err := mapinstrumentportquantity.NewEmpty()
	require.NoError(t, err)

	defer setters.Close()

	_, err = SettingRequestFromFalcon(
		mockSettingRequestReader{
			setters: func() (
				*mapinstrumentportquantity.Handle,
				error,
			) {
				return setters, nil
			},
			message: func() (string, error) {
				return "msg", nil
			},
			getters: func() (*ports.Handle, error) {
				return nil, errors.New("boom")
			},
		},
	)

	require.Error(t, err)
}

func TestSettingRequestFromFalcon_Success(
	t *testing.T,
) {
	req := SettingRequest{
		Message: "hello",
		Getters: Ports{
			{
				DefaultName:    "getter",
				InstrumentName: "dac",
				Unit:           Volt,
			},
		},
		Setters: map[InstrumentPort]Quantity{
			{
				DefaultName:    "setter",
				InstrumentName: "dac",
				Unit:           Volt,
			}: {
				Value: 12.5,
				Unit:  Volt,
			},
		},
	}

	real, err := req.NewFalconSettingRequest()
	require.NoError(t, err)
	defer real.Close()

	out, err := SettingRequestFromFalcon(real)

	require.NoError(t, err)

	assert.Equal(t, req.Message, out.Message)
	assert.Len(t, out.Getters, 1)
	assert.Len(t, out.Setters, 1)
}
