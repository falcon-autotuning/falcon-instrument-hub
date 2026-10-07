package devicestate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeQuantity struct {
	value      float64
	unit       string
	closeCalls int
	closeErr   error
}

func (f *fakeQuantity) Value() (float64, error) {
	return f.value, nil
}

func (f *fakeQuantity) UnitSymbol() (string, error) {
	return f.unit, nil
}

func (f *fakeQuantity) Close() error {
	f.closeCalls++
	return f.closeErr
}

func resetGlobalState() {
	globalMu.Lock()
	defer globalMu.Unlock()

	globalManager = nil
}

func TestFalconQuantity_Value_NilHandle(t *testing.T) {
	q := &FalconQuantity{}

	v, err := q.Value()

	require.Error(t, err)
	assert.Equal(t, 0.0, v)
	assert.EqualError(t, err, "quantity handle is nil")
}

func TestFalconQuantity_UnitSymbol_NilHandle(t *testing.T) {
	q := &FalconQuantity{}

	unit, err := q.UnitSymbol()

	require.Error(t, err)
	assert.Empty(t, unit)
	assert.EqualError(t, err, "quantity handle is nil")
}

func TestFalconQuantity_Close_NilHandle(t *testing.T) {
	q := &FalconQuantity{}

	require.NoError(t, q.Close())
}

func TestDeviceStateManager_UpdatePort_NewPort(t *testing.T) {
	q := &fakeQuantity{}

	m := &DeviceStateManager{
		state: make(map[ConnectionName]Quantity),
	}

	err := m.UpdatePort("P1", q)

	require.NoError(t, err)

	got, ok := m.Port("P1")

	require.True(t, ok)
	assert.Same(t, q, got)
}

func TestDeviceStateManager_UpdatePort_OverwritePort(t *testing.T) {
	old := &fakeQuantity{}
	newQ := &fakeQuantity{}

	m := &DeviceStateManager{
		state: map[ConnectionName]Quantity{
			"P1": old,
		},
	}

	err := m.UpdatePort("P1", newQ)

	require.NoError(t, err)

	assert.Equal(t, 1, old.closeCalls)

	got, ok := m.Port("P1")
	require.True(t, ok)

	assert.Same(t, newQ, got)
}

func TestDeviceStateManager_Port_Missing(t *testing.T) {
	m := &DeviceStateManager{
		state: make(map[ConnectionName]Quantity),
	}

	_, ok := m.Port("missing")

	assert.False(t, ok)
}

func TestDeviceStateManager_Port_Existing(t *testing.T) {
	expected := &fakeQuantity{}

	m := &DeviceStateManager{
		state: map[ConnectionName]Quantity{
			"P1": expected,
		},
	}

	got, ok := m.Port("P1")

	require.True(t, ok)
	assert.Same(t, expected, got)
}

func TestDeviceStateManager_Snapshot(t *testing.T) {
	m := &DeviceStateManager{
		state: map[ConnectionName]Quantity{
			"P1": &fakeQuantity{},
			"P2": &fakeQuantity{},
		},
	}

	snapshot := m.Snapshot()

	assert.Len(t, snapshot, 2)

	delete(snapshot, "P1")

	assert.Len(t, snapshot, 1)
	assert.Len(t, m.state, 2)
}

func TestDeviceStateManager_Clear(t *testing.T) {
	m := &DeviceStateManager{
		state: map[ConnectionName]Quantity{
			"P1": &fakeQuantity{},
			"P2": &fakeQuantity{},
		},
	}

	m.Clear()

	assert.Empty(t, m.state)
}

func TestDeviceStateManager_Close(t *testing.T) {
	q1 := &fakeQuantity{}
	q2 := &fakeQuantity{}

	m := &DeviceStateManager{
		state: map[ConnectionName]Quantity{
			"P1": q1,
			"P2": q2,
		},
	}

	err := m.Close()

	require.NoError(t, err)

	assert.Equal(t, 1, q1.closeCalls)
	assert.Equal(t, 1, q2.closeCalls)

	assert.True(t, m.closed)
	assert.Empty(t, m.state)
}

func TestDeviceStateManager_Close_AlreadyClosed(t *testing.T) {
	m := &DeviceStateManager{
		closed: true,
		state: map[ConnectionName]Quantity{
			"P1": &fakeQuantity{},
		},
	}

	err := m.Close()

	require.NoError(t, err)
	assert.True(t, m.closed)
}

func TestStartup(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	initial := map[ConnectionName]Quantity{
		"P1": &fakeQuantity{},
		"P2": &fakeQuantity{},
	}

	err := Startup(initial)

	require.NoError(t, err)

	m := Manager()
	require.NotNil(t, m)

	assert.Len(t, m.state, 2)
}

func TestStartup_AlreadyInitialized(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	require.NoError(t, Startup(map[ConnectionName]Quantity{}))

	err := Startup(map[ConnectionName]Quantity{})

	require.Error(t, err)
	assert.EqualError(t, err, "device state manager already initialized")
}

func TestManager_ReturnsNilWhenNotInitialized(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	assert.Nil(t, Manager())
}

func TestGlobalClose_WhenNil(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	require.NoError(t, Close())
}

func TestGlobalClose(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	require.NoError(t, Startup(map[ConnectionName]Quantity{
		"P1": &fakeQuantity{},
	}))

	require.NotNil(t, Manager())

	require.NoError(t, Close())

	assert.Nil(t, Manager())
}

func TestUpdatePort_CloseFailure(t *testing.T) {
	old := &fakeQuantity{
		closeErr: errors.New("boom"),
	}

	newQ := &fakeQuantity{}

	m := &DeviceStateManager{
		state: map[ConnectionName]Quantity{
			"p1": old,
		},
	}

	err := m.UpdatePort("p1", newQ)

	require.EqualError(t, err, "boom")

	assert.Equal(t, 1, old.closeCalls)

	_, ok := m.Port("p1")
	assert.True(t, ok)

	// update should not have happened
	assert.Same(t, old, m.state["p1"])
}

func TestDeviceStateManager_Close_Error(t *testing.T) {
	m := &DeviceStateManager{
		state: map[ConnectionName]Quantity{
			"P1": &fakeQuantity{
				closeErr: errors.New("err1"),
			},
			"P2": &fakeQuantity{
				closeErr: errors.New("err2"),
			},
		},
	}

	err := m.Close()

	require.Error(t, err)

	assert.ErrorContains(t, err, "close P1")
	assert.ErrorContains(t, err, "close P2")

	assert.True(t, m.closed)
	assert.Empty(t, m.state)
}

func TestDeviceStateManager_Snapshot_Copy(t *testing.T) {
	q := &fakeQuantity{}

	m := &DeviceStateManager{
		state: map[ConnectionName]Quantity{
			"P1": q,
		},
	}

	snapshot := m.Snapshot()

	snapshot["P2"] = &fakeQuantity{}

	_, ok := m.state["P2"]
	assert.False(t, ok)
}
