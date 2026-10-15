package falconcore

import (
	"errors"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/farraydouble"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axescontrolarray"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/axesint"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/discrete-spaces/discretizer"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/domains/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockUnitSpaceReader struct {
	compileErr error

	spaceHandle *farraydouble.Handle
	spaceErr    error

	arrayHandle *axescontrolarray.Handle
	arrayErr    error

	items    []*discretizer.Handle
	itemsErr error

	domainHandle *domain.Handle
	domainErr    error
}

func (m *mockUnitSpaceReader) Compile() error {
	return m.compileErr
}

func (m *mockUnitSpaceReader) Space() (*farraydouble.Handle, error) {
	return m.spaceHandle, m.spaceErr
}

func (m *mockUnitSpaceReader) CreateArray(
	_ *axesint.Handle,
) (*axescontrolarray.Handle, error) {
	return m.arrayHandle, m.arrayErr
}

func (m *mockUnitSpaceReader) Items() ([]*discretizer.Handle, error) {
	return m.items, m.itemsErr
}

func (m *mockUnitSpaceReader) Domain() (*domain.Handle, error) {
	return m.domainHandle, m.domainErr
}

func TestFalconUnitSpaceCompiler_Compile(
	t *testing.T,
) {
	expected := FArray{
		Data:  []float64{1, 2, 3, 4},
		Shape: []uint64{2, 2},
	}

	raw, err := expected.NewFalconFArray()
	require.NoError(t, err)

	compiler := &falconUnitSpaceCompiler{
		reader: &mockUnitSpaceReader{
			spaceHandle: raw,
		},
	}

	result, err := compiler.Compile()

	require.NoError(t, err)

	assert.Equal(
		t,
		expected,
		result,
	)
}

func TestFalconUnitSpaceCompiler_CompileError(
	t *testing.T,
) {
	expected := errors.New("compile failed")

	compiler := &falconUnitSpaceCompiler{
		reader: &mockUnitSpaceReader{
			compileErr: expected,
		},
	}

	result, err := compiler.Compile()

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, FArray{}, result)
}

func TestFalconUnitSpaceCompiler_SpaceError(
	t *testing.T,
) {
	expected := errors.New("space failed")

	compiler := &falconUnitSpaceCompiler{
		reader: &mockUnitSpaceReader{
			spaceErr: expected,
		},
	}

	result, err := compiler.Compile()

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, FArray{}, result)
}

func TestFalconUnitSpaceCompiler_CreateArrayError(
	t *testing.T,
) {
	expected := errors.New("array failed")

	compiler := &falconUnitSpaceCompiler{
		reader: &mockUnitSpaceReader{
			arrayErr: expected,
		},
	}

	result, err := compiler.CreateArray(
		AxesInt{1, 2},
	)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, AxesControlArray{}, result)
}

func TestUnitSpaceFromFalcon_ItemsError(
	t *testing.T,
) {
	expected := errors.New("items failed")

	reader := &mockUnitSpaceReader{
		itemsErr: expected,
	}

	result, err := UnitSpaceFromFalcon(reader)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, UnitSpace{}, result)
}

func TestUnitSpaceFromFalcon_DomainError(
	t *testing.T,
) {
	rawDiscretizer, err := discretizer.NewCartesianDiscretizer(
		1,
	)
	require.NoError(t, err)
	defer rawDiscretizer.Close()

	expected := errors.New("domain failed")

	reader := &mockUnitSpaceReader{
		items: []*discretizer.Handle{
			rawDiscretizer,
		},
		domainErr: expected,
	}

	result, err := UnitSpaceFromFalcon(reader)

	require.Error(t, err)

	assert.ErrorIs(t, err, expected)
	assert.Equal(t, UnitSpace{}, result)
}

func TestUnitSpaceFromFalcon(
	t *testing.T,
) {
	rawDiscretizer, err := discretizer.NewCartesianDiscretizer(
		0.5,
	)
	require.NoError(t, err)
	defer rawDiscretizer.Close()

	rawDomain, err := domain.New(0, 10, false, false)
	require.NoError(t, err)
	defer rawDomain.Close()

	reader := &mockUnitSpaceReader{
		items: []*discretizer.Handle{
			rawDiscretizer,
		},
		domainHandle: rawDomain,
	}

	result, err := UnitSpaceFromFalcon(reader)

	require.NoError(t, err)

	require.Len(
		t,
		result.Axes,
		1,
	)

	assert.Equal(
		t,
		CartesianDiscretization,
		result.Axes[0].Type,
	)

	assert.InDelta(
		t,
		0.5,
		result.Axes[0].Delta,
		1e-12,
	)

	assert.NotNil(
		t,
		result.UnitSpaceCompiler,
	)
}

func TestUnitSpaceFromFalcon_IntegrationCompile(
	t *testing.T,
) {
	rawDiscretizer, err := discretizer.NewCartesianDiscretizer(
		1,
	)
	require.NoError(t, err)
	defer rawDiscretizer.Close()

	rawDomain, err := domain.New(
		0,
		1,
		false,
		false,
	)
	require.NoError(t, err)
	defer rawDomain.Close()

	array := FArray{
		Data:  []float64{0, 1},
		Shape: []uint64{2},
	}

	rawSpace, err := array.NewFalconFArray()
	require.NoError(t, err)

	reader := &mockUnitSpaceReader{
		items: []*discretizer.Handle{
			rawDiscretizer,
		},
		domainHandle: rawDomain,
		spaceHandle:  rawSpace,
	}

	space, err := UnitSpaceFromFalcon(reader)
	require.NoError(t, err)

	result, err := space.Compile()

	require.NoError(t, err)

	assert.Equal(
		t,
		array,
		result,
	)
}

func TestUnitSpace_NewFalconUnitSpace_Empty(
	t *testing.T,
) {
	u := UnitSpace{
		Axes: []Discretizer{},
		Domain: Domain{
			LesserBound:  0,
			GreaterBound: 1,
		},
	}

	handle, err := u.NewFalconUnitSpace()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestUnitSpace_NewFalconUnitSpace_DiscretizerError(
	t *testing.T,
) {
	u := UnitSpace{
		Axes: []Discretizer{
			{
				Type:  DiscretizationType(999),
				Delta: 1,
			},
		},
		Domain: Domain{
			LesserBound:  0,
			GreaterBound: 1,
		},
	}

	handle, err := u.NewFalconUnitSpace()

	require.Error(t, err)
	assert.Nil(t, handle)
}

func TestUnitSpace_NewFalconUnitSpace(
	t *testing.T,
) {
	u := UnitSpace{
		Axes: []Discretizer{
			{
				Type:  CartesianDiscretization,
				Delta: 0.5,
			},
		},
		Domain: Domain{
			LesserBound:           0,
			GreaterBound:          10,
			LesserBoundContained:  false,
			GreaterBoundContained: false,
		},
	}

	handle, err := u.NewFalconUnitSpace()

	require.NoError(t, err)
	require.NotNil(t, handle)
	defer handle.Close()

	size, err := handle.Size()
	require.NoError(t, err)

	assert.Equal(t, uint64(1), size)
}

func TestUnitSpace_RoundTrip(
	t *testing.T,
) {
	original := UnitSpace{
		Axes: []Discretizer{
			{
				Type:  CartesianDiscretization,
				Delta: 0.5,
			},
		},
		Domain: Domain{
			LesserBound:           0,
			GreaterBound:          10,
			LesserBoundContained:  false,
			GreaterBoundContained: false,
		},
	}

	raw, err := original.NewFalconUnitSpace()
	require.NoError(t, err)
	defer raw.Close()

	result, err := UnitSpaceFromFalcon(raw)

	require.NoError(t, err)

	assert.Equal(
		t,
		original.Domain,
		result.Domain,
	)

	require.Len(
		t,
		result.Axes,
		1,
	)

	assert.Equal(
		t,
		original.Axes[0].Type,
		result.Axes[0].Type,
	)

	assert.InDelta(
		t,
		original.Axes[0].Delta,
		result.Axes[0].Delta,
		1e-12,
	)
}
