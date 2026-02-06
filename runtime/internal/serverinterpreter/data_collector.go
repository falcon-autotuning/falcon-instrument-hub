// Package serverinterpreter provides async data collection using Go channels.
//
// This file implements the asynchronous data collection pattern from the
// the interpreter daemon pattern, using Go channels instead of asyncio queues.
package serverinterpreter

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// DataCollector manages asynchronous data collection for measurements.
type DataCollector struct {
	// Input channel for receiving data entries
	dataChannel chan *DataEntry

	// Pending measurements waiting for data
	pendingMeasurements map[int64]*PendingMeasurement
	pendingMutex        sync.RWMutex

	// Callbacks
	onMeasurementComplete func(pm *PendingMeasurement) error
	onLog                 func(message string)

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// DataCollectorConfig configures the data collector.
type DataCollectorConfig struct {
	// QueueSize is the size of the data channel buffer
	QueueSize int

	// OnMeasurementComplete is called when a measurement has all its data
	OnMeasurementComplete func(pm *PendingMeasurement) error

	// OnLog is called for logging messages
	OnLog func(message string)
}

// DefaultDataCollectorConfig returns default configuration.
func DefaultDataCollectorConfig() DataCollectorConfig {
	return DataCollectorConfig{
		QueueSize: MaxNumDataPoints,
		OnMeasurementComplete: func(pm *PendingMeasurement) error {
			log.Printf("Measurement %d complete with %d/%d chunks",
				pm.MeasurementID, len(pm.CollectedData), pm.ExpectedCount)
			return nil
		},
		OnLog: func(message string) {
			log.Printf("[DataCollector] %s", message)
		},
	}
}

// NewDataCollector creates a new data collector.
func NewDataCollector(config DataCollectorConfig) *DataCollector {
	ctx, cancel := context.WithCancel(context.Background())

	dc := &DataCollector{
		dataChannel:           make(chan *DataEntry, config.QueueSize),
		pendingMeasurements:   make(map[int64]*PendingMeasurement),
		onMeasurementComplete: config.OnMeasurementComplete,
		onLog:                 config.OnLog,
		ctx:                   ctx,
		cancel:                cancel,
	}

	if dc.onLog == nil {
		dc.onLog = func(string) {}
	}
	if dc.onMeasurementComplete == nil {
		dc.onMeasurementComplete = func(*PendingMeasurement) error { return nil }
	}

	return dc
}

// Start begins the data collection processing goroutines.
func (dc *DataCollector) Start() {
	// Start the main data processor
	dc.wg.Add(1)
	go dc.processDataQueue()

	// Start the stale measurement cleanup task
	dc.wg.Add(1)
	go dc.cleanupStaleMeasurements()
}

// Stop gracefully shuts down the data collector.
func (dc *DataCollector) Stop() {
	dc.cancel()
	close(dc.dataChannel)
	dc.wg.Wait()
}

// RegisterMeasurement registers a new measurement for data collection.
func (dc *DataCollector) RegisterMeasurement(
	measurementID int64,
	expectedCount int,
	dataPath string,
	shape []int,
	requestJSON string,
) error {
	dc.pendingMutex.Lock()
	defer dc.pendingMutex.Unlock()

	if _, exists := dc.pendingMeasurements[measurementID]; exists {
		return fmt.Errorf("measurement %d already registered", measurementID)
	}

	dc.pendingMeasurements[measurementID] = &PendingMeasurement{
		MeasurementID: measurementID,
		ExpectedCount: expectedCount,
		DataPath:      dataPath,
		Shape:         shape,
		RequestJSON:   requestJSON,
		CollectedData: make([]*DataEntry, 0, expectedCount),
		CreatedAt:     time.Now().Unix(),
	}

	dc.onLog(fmt.Sprintf("Registered measurement %d, expecting %d chunks", measurementID, expectedCount))
	return nil
}

// QueueData adds a data entry to the processing queue.
func (dc *DataCollector) QueueData(entry *DataEntry) error {
	select {
	case dc.dataChannel <- entry:
		return nil
	case <-dc.ctx.Done():
		return fmt.Errorf("data collector stopped")
	default:
		return fmt.Errorf("data queue full, dropping data for measurement %d", entry.MeasurementID)
	}
}

// processDataQueue is the main goroutine that processes incoming data.
func (dc *DataCollector) processDataQueue() {
	defer dc.wg.Done()

	// Track requeued entries to prevent infinite loops
	requeuedEntries := make(map[int64]int) // measurementID -> requeue count

	for {
		select {
		case <-dc.ctx.Done():
			return

		case entry, ok := <-dc.dataChannel:
			if !ok {
				return
			}

			dc.pendingMutex.RLock()
			pending, exists := dc.pendingMeasurements[entry.MeasurementID]
			dc.pendingMutex.RUnlock()

			if !exists {
				// Measurement not yet registered, requeue with limit
				requeuedEntries[entry.MeasurementID]++
				if requeuedEntries[entry.MeasurementID] > 10 {
					dc.onLog(fmt.Sprintf("Dropping data for unregistered measurement %d after 10 retries",
						entry.MeasurementID))
					continue
				}

				dc.onLog(fmt.Sprintf("Measurement %d not registered yet, requeuing (attempt %d)",
					entry.MeasurementID, requeuedEntries[entry.MeasurementID]))

				// Small delay before requeue
				time.Sleep(100 * time.Millisecond)

				select {
				case dc.dataChannel <- entry:
				default:
					dc.onLog(fmt.Sprintf("Failed to requeue data for measurement %d", entry.MeasurementID))
				}
				continue
			}

			// Clear requeue counter on success
			delete(requeuedEntries, entry.MeasurementID)

			// Check for duplicate chunk
			dc.pendingMutex.Lock()
			isDuplicate := false
			for _, collected := range pending.CollectedData {
				if collected.ChunkID == entry.ChunkID {
					isDuplicate = true
					break
				}
			}

			if isDuplicate {
				dc.pendingMutex.Unlock()
				dc.onLog(fmt.Sprintf("Duplicate chunk %d for measurement %d, ignoring",
					entry.ChunkID, entry.MeasurementID))
				continue
			}

			// Add the data
			pending.AddDataEntry(entry)
			isComplete := pending.IsComplete()
			dc.pendingMutex.Unlock()

			dc.onLog(fmt.Sprintf("Collected data %d/%d for measurement %d (%.1f%%)",
				len(pending.CollectedData), pending.ExpectedCount,
				entry.MeasurementID, pending.CompletionPercentage()))

			// Check if measurement is complete
			if isComplete {
				dc.onLog(fmt.Sprintf("Measurement %d complete, processing...", entry.MeasurementID))

				// Remove from pending before processing
				dc.pendingMutex.Lock()
				completedMeasurement := dc.pendingMeasurements[entry.MeasurementID]
				delete(dc.pendingMeasurements, entry.MeasurementID)
				dc.pendingMutex.Unlock()

				// Process completion in goroutine to not block queue
				go func(pm *PendingMeasurement) {
					if err := dc.onMeasurementComplete(pm); err != nil {
						dc.onLog(fmt.Sprintf("Error completing measurement %d: %v", pm.MeasurementID, err))
					}
				}(completedMeasurement)
			}
		}
	}
}

// cleanupStaleMeasurements removes measurements that have timed out.
func (dc *DataCollector) cleanupStaleMeasurements() {
	defer dc.wg.Done()

	ticker := time.NewTicker(time.Duration(StaleMeasurementCheckup) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-dc.ctx.Done():
			return

		case <-ticker.C:
			dc.pendingMutex.Lock()
			currentTime := time.Now().Unix()
			var staleIDs []int64

			for id, pending := range dc.pendingMeasurements {
				if currentTime-pending.CreatedAt > StaleMeasurementTimeout {
					dc.onLog(fmt.Sprintf("Warning: Measurement %d timed out with %d/%d data points (%.1f%%)",
						id, len(pending.CollectedData), pending.ExpectedCount, pending.CompletionPercentage()))
					staleIDs = append(staleIDs, id)
				}
			}

			for _, id := range staleIDs {
				delete(dc.pendingMeasurements, id)
			}
			dc.pendingMutex.Unlock()
		}
	}
}

// GetPendingCount returns the number of pending measurements.
func (dc *DataCollector) GetPendingCount() int {
	dc.pendingMutex.RLock()
	defer dc.pendingMutex.RUnlock()
	return len(dc.pendingMeasurements)
}

// GetPendingMeasurement returns a pending measurement by ID.
func (dc *DataCollector) GetPendingMeasurement(id int64) (*PendingMeasurement, bool) {
	dc.pendingMutex.RLock()
	defer dc.pendingMutex.RUnlock()
	pm, exists := dc.pendingMeasurements[id]
	return pm, exists
}
