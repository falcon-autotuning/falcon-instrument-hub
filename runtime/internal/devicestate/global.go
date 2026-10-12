package devicestate

import (
	"fmt"
	"maps"
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

var (
	globalManager *DeviceStateManager
	globalMu      sync.RWMutex
)

func Startup(initial falconcore.DeviceVoltageStates) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalManager != nil {
		return fmt.Errorf("device state manager already initialized")
	}

	state := make(falconcore.DeviceVoltageStates, len(initial))

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
