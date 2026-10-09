package devicestate

import (
	"maps"
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

type DeviceStateManager struct {
	mu     sync.RWMutex
	state  DeviceVoltageStates
	closed bool
}

func (m *DeviceStateManager) UpdatePort(
	port falconcore.ConnectionName,
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
	port falconcore.ConnectionName,
) (Quantity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.state[port]
	return value.Quantity, ok
}

func (m *DeviceStateManager) Snapshot() DeviceVoltageStates {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[falconcore.ConnectionName]DeviceVoltageState, len(m.state))

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
