package falconcore

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDomainReader struct {
	lesser           float64
	greater          float64
	lesserContained  bool
	greaterContained bool

	lesserErr           error
	greaterErr          error
	lesserContainedErr  error
	greaterContainedErr error
}

func (f *fakeDomainReader) LesserBound() (float64, error) {
	return f.lesser, f.lesserErr
}

func (f *fakeDomainReader) GreaterBound() (float64, error) {
	return f.greater, f.greaterErr
}

func (f *fakeDomainReader) LesserBoundContained() (bool, error) {
	return f.lesserContained, f.lesserContainedErr
}

func (f *fakeDomainReader) GreaterBoundContained() (bool, error) {
	return f.greaterContained, f.greaterContainedErr
}

var _ DomainReader = (*fakeDomainReader)(nil)

func TestDomain_NewFalconDomain(t *testing.T) {
	d := Domain{
		LesserBound:           -1.5,
		GreaterBound:          2.5,
		LesserBoundContained:  true,
		GreaterBoundContained: false,
	}

	handle, err := d.NewFalconDomain()

	require.NoError(t, err)
	require.NotNil(t, handle)

	assert.NoError(t, handle.Close())
}

func TestDomainFromFalcon(t *testing.T) {
	d, err := DomainFromFalcon(
		&fakeDomainReader{
			lesser:           -5,
			greater:          10,
			lesserContained:  true,
			greaterContained: false,
		},
	)

	require.NoError(t, err)

	assert.Equal(
		t,
		Domain{
			LesserBound:           -5,
			GreaterBound:          10,
			LesserBoundContained:  true,
			GreaterBoundContained: false,
		},
		d,
	)
}

func TestDomainFromFalcon_LesserBoundError(t *testing.T) {
	expectedErr := errors.New("lesser error")

	d, err := DomainFromFalcon(
		&fakeDomainReader{
			lesserErr: expectedErr,
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, Domain{}, d)
}

func TestDomainFromFalcon_GreaterBoundError(t *testing.T) {
	expectedErr := errors.New("greater error")

	d, err := DomainFromFalcon(
		&fakeDomainReader{
			greaterErr: expectedErr,
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, Domain{}, d)
}

func TestDomainFromFalcon_LesserBoundContainedError(t *testing.T) {
	expectedErr := errors.New("contained error")

	d, err := DomainFromFalcon(
		&fakeDomainReader{
			lesserContainedErr: expectedErr,
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, Domain{}, d)
}

func TestDomainFromFalcon_GreaterBoundContainedError(t *testing.T) {
	expectedErr := errors.New("contained error")

	d, err := DomainFromFalcon(
		&fakeDomainReader{
			greaterContainedErr: expectedErr,
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, expectedErr)
	assert.Equal(t, Domain{}, d)
}
