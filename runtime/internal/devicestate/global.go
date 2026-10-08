package devicestate

import (
	"fmt"
	"maps"
	"sync"
)

var (
	globalManager *DeviceStateManager
	globalMu      sync.RWMutex
)

func Startup(initial DeviceVoltageStates) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalManager != nil {
		return fmt.Errorf("device state manager already initialized")
	}

	state := make(DeviceVoltageStates, len(initial))

	maps.Copy(state, initial)

	globalManager = &DeviceStateManager{
		state: state,
	}

	return nil
}

func Close() {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalManager == nil {
		return
	}

	globalManager.Close()
	globalManager = nil
}

func Manager() *DeviceStateManager {
	globalMu.RLock()
	defer globalMu.RUnlock()

	return globalManager
}
