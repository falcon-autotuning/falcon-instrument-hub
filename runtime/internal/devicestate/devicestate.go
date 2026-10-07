package devicestate

import (
	"errors"
	"fmt"
	"maps"
	"sync"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/quantity"
)

type Quantity interface {
	Value() (float64, error)
	UnitSymbol() (string, error)
	Close() error
}

type FalconQuantity struct {
	Handle *quantity.Handle
}

func (q FalconQuantity) Value() (float64, error) {
	if q.Handle == nil {
		return 0, fmt.Errorf("quantity handle is nil")
	}
	return q.Handle.Value()
}

func (q FalconQuantity) UnitSymbol() (string, error) {
	if q.Handle == nil {
		return "", fmt.Errorf("quantity handle is nil")
	}

	u, err := q.Handle.Unit()
	if err != nil {
		return "", err
	}

	return u.Symbol()
}

func (q FalconQuantity) Close() error {
	if q.Handle == nil {
		return nil
	}
	return q.Handle.Close()
}

var _ Quantity = (*FalconQuantity)(nil)

type ConnectionName string

type DeviceStateManager struct {
	mu     sync.RWMutex
	state  map[ConnectionName]Quantity
	closed bool
}

func (m *DeviceStateManager) UpdatePort(
	port ConnectionName,
	value Quantity,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.state[port]; ok {
		if err := old.Close(); err != nil {
			return err
		}
	}

	m.state[port] = value
	return nil
}

func (m *DeviceStateManager) Port(
	port ConnectionName,
) (Quantity, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.state[port]
	return value, ok
}

func (m *DeviceStateManager) Snapshot() map[ConnectionName]Quantity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[ConnectionName]Quantity, len(m.state))

	maps.Copy(result, m.state)

	return result
}

func (m *DeviceStateManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state = make(map[ConnectionName]Quantity)
}

func (m *DeviceStateManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	var errs []error

	for key, q := range m.state {
		if err := q.Close(); err != nil {
			errs = append(
				errs,
				fmt.Errorf("close %s: %w", key, err),
			)
		}
	}

	clear(m.state)
	m.closed = true

	return errors.Join(errs...)
}

var (
	globalManager *DeviceStateManager
	globalMu      sync.RWMutex
)

func Startup(initial map[ConnectionName]Quantity) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalManager != nil {
		return fmt.Errorf("device state manager already initialized")
	}

	state := make(map[ConnectionName]Quantity, len(initial))

	maps.Copy(state, initial)

	globalManager = &DeviceStateManager{
		state: state,
	}

	return nil
}

func Close() error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalManager == nil {
		return nil
	}

	err := globalManager.Close()
	globalManager = nil

	return err
}

func Manager() *DeviceStateManager {
	globalMu.RLock()
	defer globalMu.RUnlock()

	return globalManager
}
