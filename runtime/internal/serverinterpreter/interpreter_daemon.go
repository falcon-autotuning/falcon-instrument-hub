// Package serverinterpreter provides the server interpreter daemon.
//
// This file implements the InterpreterDaemon which coordinates measurement
// processing using NATS internal messaging. It is responsible for:
//   - Receiving and processing PROCESS_REQUEST commands
//   - Breaking down measurement requests into chunked instructions
//   - Coordinating with instruments via MEASUREMENT_READY
//   - Collecting PROCESS_DATA responses
//   - Uploading final results via UPLOAD_DATA
//
// The message types are aligned with falcon-api/embedded/commands/v1/ specifications.
package serverinterpreter

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// InterpreterDaemon processes measurement requests from falcon and coordinates
// with the instrument daemon through internal NATS messaging.
type InterpreterDaemon struct {
	// NATS connection
	nc *nats.Conn
	js nats.JetStreamContext

	// Subscriptions
	processRequestSub *nats.Subscription
	processDataSub    *nats.Subscription

	// Measurement processing state
	measurementGroups map[int64]*MeasurementInstructions
	groupsMutex       sync.RWMutex

	// Async data collection
	dataCollector *DataCollector

	// Configuration
	config InterpreterConfig

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Debug mode
	debug bool
}

// InterpreterConfig configures the interpreter daemon.
type InterpreterConfig struct {
	// NATSUrl is the NATS server URL
	NATSUrl string

	// Debug enables verbose logging
	Debug bool

	// StatusRefreshInterval is how often to publish status
	StatusRefreshInterval time.Duration
}

// DefaultInterpreterConfig returns default configuration.
func DefaultInterpreterConfig() InterpreterConfig {
	return InterpreterConfig{
		NATSUrl:               "nats://localhost:4222",
		Debug:                 true,
		StatusRefreshInterval: 500 * time.Millisecond,
	}
}

// NewInterpreterDaemon creates a new interpreter daemon.
func NewInterpreterDaemon(config InterpreterConfig) *InterpreterDaemon {
	ctx, cancel := context.WithCancel(context.Background())

	daemon := &InterpreterDaemon{
		measurementGroups: make(map[int64]*MeasurementInstructions),
		config:            config,
		ctx:               ctx,
		cancel:            cancel,
		debug:             config.Debug,
	}

	// Setup data collector with completion callback
	collectorConfig := DefaultDataCollectorConfig()
	collectorConfig.OnMeasurementComplete = daemon.handleMeasurementComplete
	collectorConfig.OnLog = func(msg string) {
		daemon.log(msg)
	}
	daemon.dataCollector = NewDataCollector(collectorConfig)

	return daemon
}

// Start connects to NATS and begins processing messages.
func (d *InterpreterDaemon) Start() error {
	var err error

	// Connect to NATS
	d.nc, err = nats.Connect(d.config.NATSUrl)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	d.log(fmt.Sprintf("Connected to NATS server at %s", d.config.NATSUrl))

	// Setup JetStream for large data transfers
	if err := d.setupJetStream(); err != nil {
		log.Printf("Warning: JetStream setup failed: %v", err)
		// Continue without JetStream - fall back to regular NATS
	}

	// Subscribe to channels
	if err := d.setupSubscriptions(); err != nil {
		return fmt.Errorf("failed to setup subscriptions: %w", err)
	}

	// Start data collector
	d.dataCollector.Start()

	// Start status publisher
	d.wg.Add(1)
	go d.publishStatus()

	d.log("InterpreterDaemon started successfully")
	return nil
}

// Stop gracefully shuts down the daemon.
func (d *InterpreterDaemon) Stop() error {
	d.cancel()

	// Stop data collector
	d.dataCollector.Stop()

	// Unsubscribe
	if d.processRequestSub != nil {
		d.processRequestSub.Unsubscribe()
	}
	if d.processDataSub != nil {
		d.processDataSub.Unsubscribe()
	}

	// Wait for goroutines
	d.wg.Wait()

	// Close NATS connection
	if d.nc != nil {
		d.nc.Drain()
	}

	return nil
}

// setupJetStream configures JetStream for large data transfers.
func (d *InterpreterDaemon) setupJetStream() error {
	d.log("Setting up JetStream...")

	js, err := d.nc.JetStream()
	if err != nil {
		return err
	}
	d.js = js

	// Create or update stream for measurement data
	streamConfig := &nats.StreamConfig{
		Name:      "MEASUREMENT_DATA",
		Subjects:  []string{"measurement.data.>"},
		Retention: nats.LimitsPolicy,
		MaxAge:    24 * time.Hour,
		MaxMsgs:   10000,
		MaxBytes:  1024 * 1024 * 1024, // 1GB
		Storage:   nats.FileStorage,
	}

	_, err = js.AddStream(streamConfig)
	if err != nil {
		// Try updating existing stream
		_, err = js.UpdateStream(streamConfig)
		if err != nil {
			return fmt.Errorf("failed to create/update stream: %w", err)
		}
	}

	d.log("JetStream stream created successfully")
	return nil
}

// setupSubscriptions sets up NATS subscriptions.
func (d *InterpreterDaemon) setupSubscriptions() error {
	var err error

	// Subscribe to PROCESS_REQUEST
	d.processRequestSub, err = d.nc.Subscribe(
		RuntimeChannels.ProcessRequest,
		d.handleRequest,
	)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", RuntimeChannels.ProcessRequest, err)
	}
	d.log(fmt.Sprintf("Subscribed to channel: %s", RuntimeChannels.ProcessRequest))

	// Subscribe to PROCESS_DATA
	d.processDataSub, err = d.nc.Subscribe(
		RuntimeChannels.ProcessData,
		d.handleData,
	)
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", RuntimeChannels.ProcessData, err)
	}
	d.log(fmt.Sprintf("Subscribed to channel: %s", RuntimeChannels.ProcessData))

	return nil
}

// log sends a log message to NATS and optionally prints it.
func (d *InterpreterDaemon) log(message string) {
	if d.debug {
		log.Printf("[InterpreterDaemon] %s", message)
	}

	if d.nc == nil {
		return
	}

	msg := LogMessage{
		Message:   message,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	d.nc.Publish(RuntimeChannels.Log+".interpreter", data)
}

// publishStatus periodically publishes daemon status.
func (d *InterpreterDaemon) publishStatus() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.StatusRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			pending := d.dataCollector.GetPendingCount()
			status := StatusMessage{
				Timestamp: time.Now().Unix(),
				Status:    pending > 0,
			}

			data, err := json.Marshal(status)
			if err != nil {
				continue
			}

			d.nc.Publish(RuntimeChannels.Status+".interpreter", data)
		}
	}
}

// handleRequest handles PROCESS_REQUEST commands.
// Request and Configurations can be either JSON strings or parsed objects.
func (d *InterpreterDaemon) handleRequest(msg *nats.Msg) {
	d.log("Received PROCESS_REQUEST")

	var request ProcessRequestMessage
	if err := json.Unmarshal(msg.Data, &request); err != nil {
		d.log(fmt.Sprintf("Error parsing request: %v", err))
		return
	}

	// Convert Request to JSON string if it's an object
	var requestJSON string
	switch r := request.Request.(type) {
	case string:
		requestJSON = r
	case map[string]interface{}, []interface{}:
		data, err := json.Marshal(r)
		if err != nil {
			d.log(fmt.Sprintf("Error marshaling request: %v", err))
			return
		}
		requestJSON = string(data)
	default:
		d.log(fmt.Sprintf("Unexpected request type: %T", request.Request))
		return
	}

	// Convert Configurations to JSON string if it's an object
	var configJSON string
	switch c := request.Configurations.(type) {
	case string:
		configJSON = c
	case map[string]interface{}:
		data, err := json.Marshal(c)
		if err != nil {
			d.log(fmt.Sprintf("Error marshaling configurations: %v", err))
			return
		}
		configJSON = string(data)
	case nil:
		configJSON = "{}"
	default:
		d.log(fmt.Sprintf("Unexpected configurations type: %T", request.Configurations))
		return
	}

	// Parse configurations
	config, err := ParseConfigurationsJSON(configJSON)
	if err != nil {
		d.log(fmt.Sprintf("Error parsing configurations: %v", err))
		return
	}

	// Process the measurement request
	result, err := d.processRequest(requestJSON, config, request.ProcessID)
	if err != nil {
		d.log(fmt.Sprintf("Error processing request: %v", err))
		return
	}

	d.log("Request successfully processed and chunked...")

	// Register for async data collection
	err = d.dataCollector.RegisterMeasurement(
		request.ProcessID,
		result.DataCount,
		request.DataPath,
		result.Shape,
		requestJSON,
	)
	if err != nil {
		d.log(fmt.Sprintf("Error registering measurement: %v", err))
		return
	}

	// Deploy measurement instructions to instrument daemon
	if err := d.deployMeasurements(request.ProcessID); err != nil {
		d.log(fmt.Sprintf("Error deploying measurements: %v", err))
		return
	}

	d.log("Measurement successfully deployed....")
	d.log(fmt.Sprintf("Waiting for data for id %d (expected %d chunks)", request.ProcessID, result.DataCount))
}

// processRequest processes a measurement request and creates instructions.
func (d *InterpreterDaemon) processRequest(
	requestJSON string,
	config ConfigurationMap,
	processID int64,
) (*ProcessRequestResult, error) {
	d.log("Processing measurement request...")

	// For now, we'll create a simplified waveform extraction
	// In a full implementation, this would use the falcon-core bindings
	waveformData, getters, err := d.extractWaveformData(requestJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to extract waveform data: %w", err)
	}

	// Create processor and process waveform
	processor := NewWaveformProcessor(config)
	result, err := processor.ProcessWaveformData(waveformData, getters)
	if err != nil {
		return nil, fmt.Errorf("failed to process waveform: %w", err)
	}

	// Store instructions for later deployment
	d.groupsMutex.Lock()
	d.measurementGroups[processID] = result.Instructions
	d.groupsMutex.Unlock()

	return result, nil
}

// extractWaveformData extracts waveform data from a measurement request JSON.
// This is a placeholder that should be replaced with actual falcon-core integration.
func (d *InterpreterDaemon) extractWaveformData(requestJSON string) (*WaveformData, []GetterInfo, error) {
	// Parse the request to extract basic structure
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(requestJSON), &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to parse request: %w", err)
	}

	// Extract getters
	var getters []GetterInfo
	if gettersRaw, ok := raw["getters"].([]interface{}); ok {
		for _, g := range gettersRaw {
			if gJSON, err := json.Marshal(g); err == nil {
				getters = append(getters, GetterInfo{PortJSON: string(gJSON)})
			}
		}
	}

	// Extract waveform data - this is simplified
	// In reality, you'd use the falcon-core bindings to properly parse this
	waveform := &WaveformData{
		RawTimeTrace: [][]float64{{0.0}}, // Placeholder
		AxisDomains:  [][]LabelledDomainInfo{},
		TimeDomain:   DomainBounds{Min: 0, Max: 0.001},
		Shape:        []int{1},
	}

	// Try to extract actual waveform data
	if waveforms, ok := raw["waveforms"].([]interface{}); ok && len(waveforms) > 0 {
		d.log(fmt.Sprintf("Found %d waveforms in request", len(waveforms)))
		// More detailed extraction would go here using falcon-core
	}

	return waveform, getters, nil
}

// deployMeasurements sends measurement instructions to the instrument daemon.
func (d *InterpreterDaemon) deployMeasurements(processID int64) error {
	d.groupsMutex.RLock()
	instructions, exists := d.measurementGroups[processID]
	d.groupsMutex.RUnlock()

	if !exists {
		return fmt.Errorf("no instructions found for process %d", processID)
	}

	for i := 0; i < instructions.Len(); i++ {
		instruction := instructions.At(i)

		d.log(fmt.Sprintf("Step %d of %d deploying for measurement %d",
			i+1, instructions.Len(), processID))

		// Send UPDATE_DAEMON_PROPERTY for each requirement
		for portJSON, props := range instruction.Requirements {
			for propName, propValue := range props {
				if err := d.updateDaemonProperty(propName, portJSON, propValue); err != nil {
					d.log(fmt.Sprintf("Warning: failed to update property: %v", err))
				}
			}
		}

		// Build and send MEASUREMENT_READY
		requirements := make([]string, 0)
		for portJSON, props := range instruction.Requirements {
			reqEntry := RequirementEntry{
				Setter:   portJSON,
				Property: make([]string, 0),
				Values:   make([]float64, 0),
			}
			for name, val := range props {
				reqEntry.Property = append(reqEntry.Property, name)
				if v, ok := val.(float64); ok {
					reqEntry.Values = append(reqEntry.Values, v)
				}
			}
			reqJSON, _ := json.Marshal(reqEntry)
			requirements = append(requirements, string(reqJSON))
		}

		ready := MeasurementReadyMessage{
			Timestamp:    time.Now().Unix(),
			Getters:      instruction.Getters,
			Setters:      instruction.Setters,
			Requirements: requirements,
			HasSet:       len(instruction.Setters) > 0,
			HasTrigger:   instruction.Buffered,
			IsBuffered:   instruction.Buffered,
			ProcessID:    processID,
			ChunkID:      int64(i),
		}

		data, err := json.Marshal(ready)
		if err != nil {
			return fmt.Errorf("failed to marshal MEASUREMENT_READY: %w", err)
		}

		if err := d.nc.Publish(RuntimeChannels.MeasurementReady, data); err != nil {
			return fmt.Errorf("failed to publish MEASUREMENT_READY: %w", err)
		}

		// Small delay between deployments
		time.Sleep(50 * time.Millisecond)
	}

	return nil
}

// updateDaemonProperty sends an UPDATE_DAEMON_PROPERTY message.
func (d *InterpreterDaemon) updateDaemonProperty(property, portJSON string, value interface{}) error {
	msg := UpdateDaemonPropertyMessage{
		Timestamp: time.Now().Unix(),
		Property:  property,
		Name:      portJSON,
		Value:     value,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return d.nc.Publish(RuntimeChannels.UpdateDaemonProperty, data)
}

// handleData handles PROCESS_DATA commands.
// Supports both string data format (per falcon-api spec) and legacy map format.
func (d *InterpreterDaemon) handleData(msg *nats.Msg) {
	// First try to parse as the new format (Data as string)
	var dataMsg ProcessDataMessage
	if err := json.Unmarshal(msg.Data, &dataMsg); err != nil {
		d.log(fmt.Sprintf("Error parsing data message: %v", err))
		return
	}

	// Parse the data payload - Data is now a JSON string
	parsedData := make(map[string][]float64)

	// Try parsing Data as a JSON object containing port->values mappings
	var dataMap map[string]interface{}
	if err := json.Unmarshal([]byte(dataMsg.Data), &dataMap); err == nil {
		for portJSON, rawValues := range dataMap {
			switch v := rawValues.(type) {
			case []interface{}:
				values := make([]float64, len(v))
				for i, val := range v {
					if f, ok := val.(float64); ok {
						values[i] = f
					}
				}
				parsedData[portJSON] = values
			case string:
				// Values might be JSON-encoded
				var values []float64
				if err := json.Unmarshal([]byte(v), &values); err == nil {
					parsedData[portJSON] = values
				}
			}
		}
	} else {
		d.log(fmt.Sprintf("Warning: could not parse data field: %v", err))
	}

	// Queue the data for async processing
	entry := &DataEntry{
		MeasurementID: dataMsg.ProcessID,
		ChunkID:       dataMsg.ChunkID,
		Data:          parsedData,
		Timestamp:     dataMsg.Timestamp,
	}

	if err := d.dataCollector.QueueData(entry); err != nil {
		d.log(fmt.Sprintf("Error queueing data: %v", err))
	}
}

// handleMeasurementComplete is called when all data for a measurement is collected.
func (d *InterpreterDaemon) handleMeasurementComplete(pm *PendingMeasurement) error {
	d.log(fmt.Sprintf("Processing complete measurement %d", pm.MeasurementID))

	// Get sorted chunk data
	chunkData := pm.GetSortedChunkData()

	// Calculate data points per queue
	dataPointsPerQueue, err := CalculateDataPointsPerQueue(pm.Shape, pm.ExpectedCount)
	if err != nil {
		return fmt.Errorf("failed to calculate data points: %w", err)
	}

	// Align data into sub-chunks
	aligner := &ChunkDataAligner{}
	alignedSubChunks := aligner.DivideToSubChunks(chunkData, dataPointsPerQueue)

	// Average the data
	finalData := make(map[string][]float64)
	for _, subChunk := range alignedSubChunks {
		for portJSON, data := range subChunk {
			if _, exists := finalData[portJSON]; !exists {
				finalData[portJSON] = make([]float64, 0)
			}
			avg := AverageSubChunk(data)
			finalData[portJSON] = append(finalData[portJSON], avg)
		}
	}

	// Build response and upload
	responseJSON, err := d.buildResponseJSON(finalData, pm.Shape)
	if err != nil {
		return fmt.Errorf("failed to build response: %w", err)
	}

	return d.uploadData(responseJSON, pm.MeasurementID)
}

// buildResponseJSON builds a MeasurementResponse JSON string.
func (d *InterpreterDaemon) buildResponseJSON(data map[string][]float64, shape []int) (string, error) {
	// Build a response structure matching falcon-core's MeasurementResponse
	response := map[string]interface{}{
		"arrays": map[string]interface{}{
			"arrays": make([]map[string]interface{}, 0),
		},
	}

	arrays := response["arrays"].(map[string]interface{})["arrays"].([]map[string]interface{})

	for portJSON, values := range data {
		array := map[string]interface{}{
			"label": portJSON,
			"array": map[string]interface{}{
				"data":  values,
				"shape": shape,
			},
		}
		arrays = append(arrays, array)
	}

	response["arrays"].(map[string]interface{})["arrays"] = arrays

	jsonData, err := json.Marshal(response)
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

// uploadData sends the measurement response back to falcon.
// Uses the new UploadDataMessage format with channel and stream fields.
func (d *InterpreterDaemon) uploadData(responseJSON string, processID int64) error {
	d.log(fmt.Sprintf("Uploading data for ProcessID: %d", processID))

	dataChannel := fmt.Sprintf("measurement.data.%d", processID)
	streamName := "MEASUREMENT_DATA"

	// If JetStream is available, publish the actual data there
	if d.js != nil {
		// Publish raw response data to JetStream
		_, err := d.js.Publish(dataChannel, []byte(responseJSON))
		if err != nil {
			d.log(fmt.Sprintf("JetStream publish failed, falling back to regular NATS: %v", err))
		} else {
			d.log(fmt.Sprintf("Published data to JetStream channel: %s", dataChannel))
		}
	}

	// Send notification on UPLOAD_DATA channel with channel/stream reference
	// This matches falcon-api/embedded/commands/v1/upload_data.yaml
	notification := UploadDataMessage{
		Timestamp: time.Now().Unix(),
		ProcessID: processID,
		UnitHash:  0, // Set by coordinator if applicable
		Channel:   dataChannel,
		Stream:    streamName,
	}

	notificationData, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	return d.nc.Publish(RuntimeChannels.UploadData, notificationData)
}

// Run starts the daemon and blocks until stopped.
func (d *InterpreterDaemon) Run() error {
	if err := d.Start(); err != nil {
		return err
	}

	// Block until context is cancelled
	<-d.ctx.Done()

	return d.Stop()
}
