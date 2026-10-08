package devicestate

import (
	"maps"
	"sync"
)

type DeviceStateManager struct {
	mu     sync.RWMutex
	state  DeviceVoltageStates
	closed bool
}

func (m *DeviceStateManager) UpdatePort(
	port ConnectionName,
	value Quantity,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.state[port]
	connection := old.Connection
	new := DeviceVoltageState{value, connection}
	m.state[port] = new
	return nil
}

func (m *DeviceStateManager) Port(
	port ConnectionName,
) (Quantity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.state[port]
	return value.Quantity, ok
}

func (m *DeviceStateManager) Snapshot() DeviceVoltageStates {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[ConnectionName]DeviceVoltageState, len(m.state))

	maps.Copy(result, m.state)

	return result
}

func (m *DeviceStateManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state = make(DeviceVoltageStates)
}

func (m *DeviceStateManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return
	}

	clear(m.state)
	m.closed = true
}
