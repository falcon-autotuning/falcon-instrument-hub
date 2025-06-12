package measure

import (
	"sync"
	"time"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
)

// MeasurementStackItem represents a queued measurement
type MeasurementStackItem struct {
	MeasurementReady api.MeasurementReady
	Timestamp        time.Time
	Priority         int // Optional: for priority handling
}

// MeasurementStack implements a FIFO queue for measurements
type MeasurementStack struct {
	items []MeasurementStackItem
	mutex sync.RWMutex
}

// Push adds a measurement to the top of the stack
func (ms *MeasurementStack) Push(item MeasurementStackItem) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	// Add to the beginning (top) of the slice
	ms.items = append([]MeasurementStackItem{item}, ms.items...)
}

// Pop removes and returns a measurement from the bottom of the stack
func (ms *MeasurementStack) Pop() (MeasurementStackItem, bool) {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()

	if len(ms.items) == 0 {
		return MeasurementStackItem{}, false
	}

	// Remove from the end (bottom) of the slice
	item := ms.items[len(ms.items)-1]
	ms.items = ms.items[:len(ms.items)-1]
	return item, true
}

// Peek returns the next measurement without removing it
func (ms *MeasurementStack) Peek() (MeasurementStackItem, bool) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	if len(ms.items) == 0 {
		return MeasurementStackItem{}, false
	}

	return ms.items[len(ms.items)-1], true
}

// Size returns the number of queued measurements
func (ms *MeasurementStack) Size() int {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()
	return len(ms.items)
}

// Clear removes all measurements from the stack
func (ms *MeasurementStack) Clear() {
	ms.mutex.Lock()
	defer ms.mutex.Unlock()
	ms.items = nil
}
