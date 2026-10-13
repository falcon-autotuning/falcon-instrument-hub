package devicestate

import (
	"maps"
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

type DeviceStateManager struct {
	mu     sync.RWMutex
	state  falconcore.DeviceVoltageStates
	closed bool
}

func (m *DeviceStateManager) UpdatePort(
	port falconcore.ConnectionName,
	value falconcore.Quantity,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.state[port]
	connection := old.Connection
	new := falconcore.DeviceVoltageState{
		Quantity:   value,
		Connection: connection,
	}
	m.state[port] = new
	return nil
}

func (m *DeviceStateManager) Port(
	port falconcore.ConnectionName,
) (falconcore.Quantity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.state[port]
	return value.Quantity, ok
}

func (m *DeviceStateManager) Snapshot() falconcore.DeviceVoltageStates {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(
		map[falconcore.ConnectionName]falconcore.DeviceVoltageState,
		len(m.state),
	)

	maps.Copy(result, m.state)

	return result
}

func (m *DeviceStateManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state = make(falconcore.DeviceVoltageStates)
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
