package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLabelledDomainReader struct {
	port    *instrumentport.Handle
	portErr error

	lesser           float64
	greater          float64
	lesserContained  bool
	greaterContained bool

	lesserErr           error
	greaterErr          error
	lesserContainedErr  error
	greaterContainedErr error

	portCalls int
}

func (m *mockLabelledDomainReader) Port() (*instrumentport.Handle, error) {
	m.portCalls++
	return m.port, m.portErr
}

func (m *mockLabelledDomainReader) LesserBound() (float64, error) {
	return m.lesser, m.lesserErr
}

func (m *mockLabelledDomainReader) GreaterBound() (float64, error) {
	return m.greater, m.greaterErr
}

func (m *mockLabelledDomainReader) LesserBoundContained() (bool, error) {
	return m.lesserContained, m.lesserContainedErr
}

func (m *mockLabelledDomainReader) GreaterBoundContained() (bool, error) {
	return m.greaterContained, m.greaterContainedErr
}

func validInstrumentPort() InstrumentPort {
	return InstrumentPort{
		DefaultName:    "P1",
		InstrumentName: "DAC1",
		InstrumentType: instrument.Amnmeter,
		Description:    "test",
		Unit:           Volt,
		Access:         access.Readwrite,
		Scope:          scope.Global,
		Characteristic: instrumentcharacteristic.InstrumentCharacteristicNone,
		PortType:       porttype.PortTypeKnob,
	}
}

func TestLabelledDomain_NewFalconLabelledDomain_Success(t *testing.T) {
	l := LabelledDomain{
		InstrumentPort: validInstrumentPort(),
		Domain: Domain{
			LesserBound:           -1,
			GreaterBound:          1,
			LesserBoundContained:  true,
			GreaterBoundContained: false,
		},
	}

	handle, err := l.NewFalconLabelledDomain()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestLabelledDomain_NewFalconLabelledDomain_PortError(t *testing.T) {
	l := LabelledDomain{
		InstrumentPort: InstrumentPort{
			DefaultName:    "P1",
			InstrumentName: "DAC1",
			Unit:           Unit(999),
		},
		Domain: Domain{
			LesserBound:  0,
			GreaterBound: 1,
		},
	}

	handle, err := l.NewFalconLabelledDomain()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestLabelledDomainFromFalcon_PortError(t *testing.T) {
	expectedErr := errors.New("port failed")

	reader := &mockLabelledDomainReader{
		portErr: expectedErr,
	}

	out, err := LabelledDomainFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, LabelledDomain{}, out)
}

func TestLabelledDomainFromFalcon_DomainError(t *testing.T) {
	port, err := validInstrumentPort().NewFalconInstrumentPort()
	require.NoError(t, err)
	defer port.Close()

	expectedErr := errors.New("lesser failed")

	reader := &mockLabelledDomainReader{
		port:      port,
		lesserErr: expectedErr,
	}

	out, err := LabelledDomainFromFalcon(reader)

	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, LabelledDomain{}, out)
}

func TestLabelledDomainFromFalcon_Success(t *testing.T) {
	port, err := validInstrumentPort().NewFalconInstrumentPort()
	require.NoError(t, err)
	defer port.Close()

	reader := &mockLabelledDomainReader{
		port: port,

		lesser:           -1,
		greater:          1,
		lesserContained:  true,
		greaterContained: false,
	}

	out, err := LabelledDomainFromFalcon(reader)

	require.NoError(t, err)

	assert.Equal(t,
		LabelledDomain{
			InstrumentPort: validInstrumentPort(),
			Domain: Domain{
				LesserBound:           -1,
				GreaterBound:          1,
				LesserBoundContained:  true,
				GreaterBoundContained: false,
			},
		},
		out,
	)
}
