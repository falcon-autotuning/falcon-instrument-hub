package handlers

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/measure"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

var SetCommand = api.GetCommandName(api.Set{})

// Test helper to create a test NATS server
func setupTestNATSServer(t *testing.T) *nats.Conn {
	// For testing purposes, we'll use a mock or embedded NATS server
	// In a real test environment, you might want to use nats-server/test
	// package
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skip("NATS server not available for testing")
	}
	return nc
}

// setupTestInstrumentHandler creates a real instrument handler for testing
func setupTestInstrumentHandler2(t *testing.T) *instrument.Handler {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create test logger with proper file paths
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	// Setup NATS connection for the instrument handler
	nc := setupTestNATSServer(t)

	// Create test config with the correct structure
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	// Create instrument handler with the correct signature
	handler, err := instrument.NewHandler(
		logger,
		nats.DefaultURL,
		nc,
		cfg,
		"python",
	)
	require.NoError(t, err)

	// Create mock instruments in the handler for testing
	handler.Instruments = map[instrument.Name]*instrument.InstrumentProcess{
		"instrument1": {
			Name:        "instrument1",
			Initialized: true,
			Ports: map[instrument.PropertyName]map[instrument.Index]instrument.JsonPort{
				"knobs": {
					"1": createTestPortJSON("port1"),
				},
				measure.Arm: {
					measure.GlobalIndex: createTestPortJSON("port1"),
				},
			},
			Configuration: map[instrument.PropertyName]map[instrument.Index]instrument.PortConfiguration{
				"knobs": {
					"1": {
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
				},
				measure.Arm: {
					measure.GlobalIndex: {
						"type": "boolean",
					},
				},
			},
		},
		"instrument2": {
			Name:        "instrument2",
			Initialized: true,
			Ports: map[instrument.PropertyName]map[instrument.Index]instrument.JsonPort{
				"knobs": {
					"2": createTestPortJSON("port2"),
				},
				measure.Arm: {
					measure.GlobalIndex: createTestPortJSON("port2"),
				},
			},
			Configuration: map[instrument.PropertyName]map[instrument.Index]instrument.PortConfiguration{
				"knobs": {
					"2": {
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
				},
				measure.Arm: {
					measure.GlobalIndex: {
						"type": "boolean",
					},
				},
			},
		},
		"getter_instrument": {
			Name:        "getter_instrument",
			Initialized: true,
			Ports: map[instrument.PropertyName]map[instrument.Index]instrument.JsonPort{
				"knobs": {
					"10": createTestPortJSON("getter_port"),
				},
				measure.Arm: {
					measure.GlobalIndex: createTestPortJSON("getter_port"),
				},
			},
			Configuration: map[instrument.PropertyName]map[instrument.Index]instrument.PortConfiguration{
				"knobs": {
					"10": map[string]any{
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
				},
				measure.Arm: {
					measure.GlobalIndex: map[string]any{
						"type": "boolean",
					},
				},
			},
		},
		"setter_instrument": {
			Name:        "setter_instrument",
			Initialized: true,
			Ports: map[instrument.PropertyName]map[instrument.Index]instrument.JsonPort{
				"knobs": {
					"20": createTestPortJSON("setter_port"),
				},
				measure.Arm: {
					measure.GlobalIndex: createTestPortJSON("setter_port"),
				},
				"master": {
					measure.GlobalIndex: createTestPortJSON("master_port"),
				},
			},
			Configuration: map[instrument.PropertyName]map[instrument.Index]instrument.PortConfiguration{
				"knobs": {
					"20": map[string]any{
						"bounds": []float64{0, 100},
						"unit":   "V",
					},
				},
				measure.Arm: {
					measure.GlobalIndex: map[string]any{
						"type": "boolean",
					},
				},
				"master": {
					measure.GlobalIndex: map[string]any{
						"value": true,
					},
				},
			},
		},
	}

	return handler
}

// createTestPortJSON creates a test port JSON string with the given port name
// and property
func createTestPortJSON(
	portName config.InstrumentConnection,
) instrument.JsonPort {
	port := instrument.PortObject{
		Class:  "Knob",
		Module: "falcon_core.instrument_interfaces.names.knob",
		PseudoName: instrument.PsuedoName{
			Class:  "Knob",
			Module: "falcon_core.instrument_interfaces.names.knob",
			Name:   portName,
		},
		InstrumentType: "DAC",
		Units:          map[string]any{"unit": "V"},
		Description:    "Test port",
	}
	data, _ := json.Marshal(port)
	return instrument.JsonPort(data)
}

func TestMeasurementReadyHandler_BufferedMeasurement(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := measure.NewMeasurementReadyHandler(
		logger,
		instrumentHandler,
		cfg,
	)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create buffered measurement request
	getterPortJSON := createTestPortJSON("getter_port")

	setterInstruction := map[string]any{
		"setter":   string(createTestPortJSON("setter_port")),
		"property": []string{"knobs"},
		"values":   []any{5.0},
	}
	setterInstructionJSON, _ := json.Marshal(setterInstruction)

	measurementReady := api.MeasurementReady{
		ProcessId: 2,
		Getters:   []string{string(getterPortJSON)},
		Setters: []string{
			string(setterInstructionJSON),
		}, // Use instruction format
		Buffered: true,
	}

	measurementData, err := json.Marshal(measurementReady)
	require.NoError(t, err)

	// Track the sequence of operations using atomic operations
	var armedReceived int64
	var getterTriggered int64
	var setterTriggered int64
	var executingReceived int64
	var setCommandCount int64
	var armCommandCount int64

	// IMPORTANT: Let the measurement ready handler subscribe first
	// so it gets priority on messages
	time.Sleep(100 * time.Millisecond)

	// Subscribe to SET commands to simulate setter instrument arming
	// Use a specific subject to avoid conflicts
	setSubSetter, err := nc.Subscribe(
		instrument.SetCommand+".setter_instrument",
		func(msg *nats.Msg) {
			var setCmd api.Set
			if err := json.Unmarshal(msg.Data, &setCmd); err != nil {
				t.Logf(
					"Failed to unmarshal %s command: %v",
					instrument.SetCommand,
					err,
				)
				return
			}

			atomic.AddInt64(&setCommandCount, 1)
			t.Logf(
				"TEST: Received %s command: property=%s, index=%d, value=%v, processId=%d, chunkId=%d",
				instrument.SetCommand,
				setCmd.Property,
				setCmd.Index,
				setCmd.Value,
				setCmd.ProcessId,
				setCmd.ChunkId,
			)

			// Simulate ARM command received - send ARMED response
			if setCmd.Property == string(measure.Arm) {
				atomic.AddInt64(&armCommandCount, 1)
				t.Logf(
					"TEST: %s command detected, sending %s response",
					measure.Arm,
					measure.ArmedMessage,
				)

				armed := api.Armed{
					ProcessId: setCmd.ProcessId,
					ChunkId:   setCmd.ChunkId,
				}
				armedData, _ := json.Marshal(armed)
				armedSubject := measure.ArmedMessage + ".setter_instrument"
				if err := nc.Publish(armedSubject, armedData); err != nil {
					t.Logf(
						"TEST: Failed to publish %s: %v",
						measure.ArmedMessage,
						err,
					)
				} else {
					t.Logf("TEST: Published %s to %s", measure.ArmedMessage, armedSubject)
					atomic.AddInt64(&armedReceived, 1)
				}
			}
		},
	)
	require.NoError(t, err)
	defer setSubSetter.Unsubscribe()

	// Subscribe to TRIGGER commands for getter instrument
	triggerSubGetter, err := nc.Subscribe(
		measure.TriggerMessage+".getter_instrument",
		func(msg *nats.Msg) {
			var triggerCmd api.Trigger
			if err := json.Unmarshal(msg.Data, &triggerCmd); err != nil {
				t.Logf(
					"Failed to unmarshal %s command: %v",
					measure.TriggerMessage,
					err,
				)
				return
			}

			t.Logf(
				"TEST: Received %s for getter: processId=%d, chunkId=%d",
				measure.TriggerMessage,
				triggerCmd.ProcessId,
				triggerCmd.ChunkId,
			)
			atomic.AddInt64(&getterTriggered, 1)

			// Simulate getter instrument executing - send EXECUTING response
			executing := api.Executing{
				ProcessId: triggerCmd.ProcessId,
				ChunkId:   triggerCmd.ChunkId,
			}
			executingData, _ := json.Marshal(executing)
			executingSubject := measure.ExecutingMessage + ".getter_instrument"
			if err := nc.Publish(executingSubject, executingData); err != nil {
				t.Logf(
					"TEST: Failed to publish %s : %v",
					measure.ExecutingMessage,
					err,
				)
			} else {
				t.Logf("TEST: Published %s to %s", measure.ExecutingMessage, executingSubject)
				atomic.AddInt64(&executingReceived, 1)
			}
		},
	)
	require.NoError(t, err)
	defer triggerSubGetter.Unsubscribe()

	// Subscribe to TRIGGER commands for setter instrument
	triggerSubSetter, err := nc.Subscribe(
		measure.TriggerMessage+".setter_instrument", // Specific subject
		func(msg *nats.Msg) {
			var triggerCmd api.Trigger
			if err := json.Unmarshal(msg.Data, &triggerCmd); err != nil {
				t.Logf(
					"Failed to unmarshal %s command: %v",
					measure.TriggerMessage,
					err,
				)
				return
			}

			t.Logf(
				"TEST: Received %s for setter: processId=%d, chunkId=%d",
				measure.TriggerMessage,
				triggerCmd.ProcessId,
				triggerCmd.ChunkId,
			)
			atomic.AddInt64(&setterTriggered, 1)

			// Simulate instrument returning buffered data after setter is
			// triggered
			returnData := api.ReturnData{
				Data:      []any{99.9, 88.8},
				Property:  "knobs",
				Index:     10, // This must match the getter_instrument index
				ProcessId: triggerCmd.ProcessId,
				ChunkId:   triggerCmd.ChunkId,
			}
			returnDataBytes, _ := json.Marshal(returnData)

			// Publish the RETURN_DATA response
			returnDataSubject := measure.ReturnDataMessage + ".getter_instrument"
			if err := nc.Publish(returnDataSubject, returnDataBytes); err != nil {
				t.Logf(
					"TEST: Failed to publish %s : %v",
					measure.ReturnDataMessage,
					err,
				)
			} else {
				t.Logf("TEST: Published %s to %s", measure.ReturnDataMessage, returnDataSubject)
			}
		},
	)
	require.NoError(t, err)
	defer triggerSubSetter.Unsubscribe()

	// Subscribe to PROCESS_DATA to capture the final result
	processDataReceived := make(chan api.ProcessData, 1)
	processDataSub, err := nc.Subscribe(
		measure.ProcessDataMessage,
		func(msg *nats.Msg) {
			var processData api.ProcessData
			if err := json.Unmarshal(msg.Data, &processData); err != nil {
				t.Logf(
					"Failed to unmarshal %s : %v",
					measure.ProcessDataMessage,
					err,
				)
				return
			}
			t.Logf(
				"Received %s: processId=%d",
				measure.ProcessDataMessage,
				processData.ProcessId,
			)
			processDataReceived <- processData
		},
	)
	require.NoError(t, err)
	defer processDataSub.Unsubscribe()

	// Send MEASUREMENT_READY message
	err = nc.Publish(measure.MeasurementReadyMessage, measurementData)
	require.NoError(t, err)

	// Wait for PROCESS_DATA response
	select {
	case processData := <-processDataReceived:
		assert.Equal(t, int64(2), processData.ProcessId)

		// Parse the data JSON
		var results map[string]any
		err := json.Unmarshal([]byte(processData.Data), &results)
		require.NoError(t, err)

		// Verify the buffered result
		expectedData := []interface{}{99.9, 88.8}
		assert.Equal(t, expectedData, results[string(getterPortJSON)])
		assert.Greater(t, processData.Timestamp, int64(0))

	case <-time.After(5 * time.Second):
		// Print debug information before failing
		t.Logf("Debug info:")
		t.Logf(
			"  SET commands received: %d",
			atomic.LoadInt64(&setCommandCount),
		)
		t.Logf(
			"  ARM commands received: %d",
			atomic.LoadInt64(&armCommandCount),
		)
		t.Logf("  ARMED responses sent: %d", atomic.LoadInt64(&armedReceived))
		t.Logf("  Getter triggers: %d", atomic.LoadInt64(&getterTriggered))
		t.Logf("  Setter triggers: %d", atomic.LoadInt64(&setterTriggered))
		t.Logf("  EXECUTING received: %d", atomic.LoadInt64(&executingReceived))
		t.Fatal("Timeout waiting for PROCESS_DATA")
	}

	// Verify the sequence
	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&armedReceived),
		"Should have received one ARMED",
	)
	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&getterTriggered),
		"Should have triggered getter once",
	)
	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&setterTriggered),
		"Should have triggered setter once",
	)
	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&executingReceived),
		"Should have received one EXECUTING",
	)
	assert.GreaterOrEqual(
		t,
		atomic.LoadInt64(&setCommandCount),
		int64(2),
		"Should have received at least 2 SET commands",
	)
	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&armCommandCount),
		"Should have received exactly 1 ARM command",
	)
}

func TestMeasurementReadyHandler_AsynchronousBuffering(t *testing.T) {
	// Test multiple measurements can be pipelined
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := measure.NewMeasurementReadyHandler(
		logger,
		instrumentHandler,
		cfg,
	)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Track SET commands received by instruments
	var setCommandCount int64
	var uniqueChunkIds []int64
	var chunkIdMutex sync.Mutex

	// Subscribe to SET commands to track pipelining
	setSubSetter, err := nc.Subscribe(
		SetCommand+".setter_instrument",
		func(msg *nats.Msg) {
			var setCmd api.Set
			json.Unmarshal(msg.Data, &setCmd)

			atomic.AddInt64(&setCommandCount, 1)

			// Track unique ChunkIds
			chunkIdMutex.Lock()
			found := false
			found = slices.Contains(uniqueChunkIds, setCmd.ChunkId)
			if !found {
				uniqueChunkIds = append(uniqueChunkIds, setCmd.ChunkId)
			}
			chunkIdMutex.Unlock()

			// Send ARMED response for ARM commands
			if setCmd.Property == "ARM" {
				armed := api.Armed{
					ProcessId: setCmd.ProcessId,
					ChunkId:   setCmd.ChunkId,
				}
				armedData, _ := json.Marshal(armed)
				nc.Publish(measure.ArmedMessage+".setter_instrument", armedData)
			}
		},
	)
	require.NoError(t, err)
	defer setSubSetter.Unsubscribe()

	// Create multiple measurements
	getterPortJSON := createTestPortJSON("getter_port")
	setterInstruction := map[string]interface{}{
		"setter":   string(createTestPortJSON("setter_port")),
		"property": []string{"knobs"},
		"values":   []interface{}{5.0},
	}
	setterInstructionJSON, _ := json.Marshal(setterInstruction)

	// Send 3 measurements rapidly
	for i := 1; i <= 3; i++ {
		measurementReady := api.MeasurementReady{
			ProcessId: int64(100 + i),
			Getters:   []string{string(getterPortJSON)},
			Setters:   []string{string(setterInstructionJSON)},
			Buffered:  true,
		}

		measurementData, _ := json.Marshal(measurementReady)
		nc.Publish(measure.MeasurementReadyMessage, measurementData)
	}

	// Wait for SET commands to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify that SET commands were sent for all measurements
	setCount := atomic.LoadInt64(&setCommandCount)
	assert.GreaterOrEqual(
		t,
		setCount,
		int64(3),
		"Should have sent SET commands for all measurements",
	)

	// Verify that different ChunkIds were assigned
	chunkIdMutex.Lock()
	assert.GreaterOrEqual(
		t,
		len(uniqueChunkIds),
		3,
		"Should have assigned unique ChunkIds",
	)
	chunkIdMutex.Unlock()

	// Verify ChunkIds are sequential
	chunkIdMutex.Lock()
	for i := 1; i < len(uniqueChunkIds); i++ {
		assert.Greater(
			t,
			uniqueChunkIds[i],
			uniqueChunkIds[i-1],
			"ChunkIds should be sequential",
		)
	}
	chunkIdMutex.Unlock()
}

func TestMeasurementReadyHandler_UnbufferedMeasurement(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := measure.NewMeasurementReadyHandler(
		logger,
		instrumentHandler,
		cfg,
	)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create unbuffered measurement request with multiple setters
	getterPortJSON1 := createTestPortJSON("getter_port")
	getterPortJSON2 := createTestPortJSON("port1") // instrument1 port

	setterInstruction1 := map[string]interface{}{
		"setter": string(
			createTestPortJSON("setter_port"),
		), // setter_instrument
		"property": []string{"knobs"},
		"values":   []interface{}{5.0},
	}
	setterInstruction2 := map[string]interface{}{
		"setter":   string(createTestPortJSON("port2")), // instrument2 port
		"property": []string{"knobs"},
		"values":   []interface{}{10.0},
	}
	setterInstruction1JSON, _ := json.Marshal(setterInstruction1)
	setterInstruction2JSON, _ := json.Marshal(setterInstruction2)

	measurementReady := api.MeasurementReady{
		ProcessId: 4,
		Getters:   []string{string(getterPortJSON1), string(getterPortJSON2)},
		Setters: []string{
			string(setterInstruction1JSON),
			string(setterInstruction2JSON),
		},
		Buffered: false, // Unbuffered measurement
	}

	measurementData, err := json.Marshal(measurementReady)
	require.NoError(t, err)

	// Track the sequence of operations
	var armedCount int64
	var getterTriggerCount int64
	var setterTriggerCount int64
	var executingCount int64
	var setCommandCount int64
	var armCommandCount int64
	var triggeredInstruments []string
	var armedInstruments []string
	var mu sync.Mutex

	// Subscribe to SET commands to simulate setter instruments arming
	setSubMultiple, err := nc.Subscribe(
		"SET.>", // Listen to all SET commands
		func(msg *nats.Msg) {
			var setCmd api.Set
			if err := json.Unmarshal(msg.Data, &setCmd); err != nil {
				t.Logf("Failed to unmarshal SET command: %v", err)
				return
			}

			atomic.AddInt64(&setCommandCount, 1)
			instrumentName := strings.Split(msg.Subject, ".")[1]

			t.Logf(
				"TEST: Received SET command on %s: property=%s, index=%d, value=%v, processId=%d, chunkId=%d",
				instrumentName,
				setCmd.Property,
				setCmd.Index,
				setCmd.Value,
				setCmd.ProcessId,
				setCmd.ChunkId,
			)

			// Simulate ARM command received - send ARMED response
			if setCmd.Property == "ARM" {
				atomic.AddInt64(&armCommandCount, 1)
				mu.Lock()
				armedInstruments = append(armedInstruments, instrumentName)
				mu.Unlock()

				t.Logf(
					"TEST: ARM command detected on %s, sending ARMED response",
					instrumentName,
				)

				armed := api.Armed{
					ProcessId: setCmd.ProcessId,
					ChunkId:   setCmd.ChunkId,
				}
				armedData, _ := json.Marshal(armed)
				armedSubject := "ARMED." + instrumentName
				if err := nc.Publish(armedSubject, armedData); err != nil {
					t.Logf("TEST: Failed to publish ARMED: %v", err)
				} else {
					t.Logf("TEST: Published ARMED to %s", armedSubject)
					atomic.AddInt64(&armedCount, 1)
				}
			}
		},
	)
	require.NoError(t, err)
	defer setSubMultiple.Unsubscribe()

	// Subscribe to TRIGGER commands for all instruments
	triggerSubAll, err := nc.Subscribe(
		"TRIGGER.>",
		func(msg *nats.Msg) {
			var triggerCmd api.Trigger
			if err := json.Unmarshal(msg.Data, &triggerCmd); err != nil {
				t.Logf("Failed to unmarshal TRIGGER command: %v", err)
				return
			}

			instrumentName := strings.Split(msg.Subject, ".")[1]
			mu.Lock()
			triggeredInstruments = append(triggeredInstruments, instrumentName)
			mu.Unlock()

			t.Logf(
				"TEST: Received TRIGGER for %s: processId=%d, chunkId=%d",
				instrumentName,
				triggerCmd.ProcessId,
				triggerCmd.ChunkId,
			)

			// Check if this instrument is in the getter instruments list
			if strings.Contains(instrumentName, "getter") ||
				instrumentName == "instrument1" {
				atomic.AddInt64(&getterTriggerCount, 1)

				// Simulate getter instrument executing - send EXECUTING
				// response
				executing := api.Executing{
					ProcessId: triggerCmd.ProcessId,
					ChunkId:   triggerCmd.ChunkId,
				}
				executingData, _ := json.Marshal(executing)
				executingSubject := "EXECUTING." + instrumentName
				if err := nc.Publish(executingSubject, executingData); err != nil {
					t.Logf("TEST: Failed to publish EXECUTING: %v", err)
				} else {
					t.Logf("TEST: Published EXECUTING to %s", executingSubject)
					atomic.AddInt64(&executingCount, 1)
				}
			} else {
				atomic.AddInt64(&setterTriggerCount, 1)
				// For unbuffered measurements, setters don't typically send
				// data back
			}
		},
	)
	require.NoError(t, err)
	defer triggerSubAll.Unsubscribe()

	// Give time for subscriptions to be ready
	time.Sleep(100 * time.Millisecond)

	// Send MEASUREMENT_READY message
	err = nc.Publish(measure.MeasurementReadyMessage, measurementData)
	require.NoError(t, err)

	// Wait for processing to complete
	time.Sleep(1 * time.Second)
	// Verify the sequence for unbuffered measurement
	totalTriggers := atomic.LoadInt64(
		&getterTriggerCount,
	) + atomic.LoadInt64(
		&setterTriggerCount,
	)

	t.Logf("Final counts:")
	t.Logf("  ARMED responses: %d", atomic.LoadInt64(&armedCount))
	t.Logf("  Getter triggers: %d", atomic.LoadInt64(&getterTriggerCount))
	t.Logf("  Setter triggers: %d", atomic.LoadInt64(&setterTriggerCount))
	t.Logf("  Total triggers: %d", totalTriggers)

	// Should process normally with first setter only
	// Expect both getter and setter to be triggered (total 2)
	assert.Equal(
		t,
		int64(4),
		totalTriggers,
		"Should have triggered both getter and setter instruments",
	)

	assert.Equal(
		t,
		int64(2),
		atomic.LoadInt64(&getterTriggerCount),
		"Should have triggered getter instrument once",
	)

	assert.Equal(
		t,
		int64(2),
		atomic.LoadInt64(&setterTriggerCount),
		"Should have triggered setter instrument once (ignoring duplicate setters)",
	)
}

func TestMeasurementReadyHandler_ChunkIdAssignment(t *testing.T) {
	// Test that ChunkIds are assigned correctly and uniquely
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := measure.NewMeasurementReadyHandler(
		logger,
		instrumentHandler,
		cfg,
	)

	// Verify initial ChunkId is 1
	assert.Equal(t, int64(1), handler.NextChunkId)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Track ChunkIds in messages
	var receivedChunkIds []int64
	var chunkIdMutex sync.Mutex

	// Subscribe to SET commands to capture ChunkIds
	setSub, err := nc.Subscribe(
		"SET.>",
		func(msg *nats.Msg) {
			var setCmd api.Set
			if json.Unmarshal(msg.Data, &setCmd) == nil {
				chunkIdMutex.Lock()
				// Only count each unique ChunkId once
				found := false
				for _, existingId := range receivedChunkIds {
					if existingId == setCmd.ChunkId {
						found = true
						break
					}
				}
				if !found {
					receivedChunkIds = append(receivedChunkIds, setCmd.ChunkId)
				}
				chunkIdMutex.Unlock()
			}
		},
	)
	require.NoError(t, err)
	defer setSub.Unsubscribe()

	// Subscribe to SET commands to send ARMED responses
	setSub, err = nc.Subscribe(
		"SET.>", // Use explicit subject pattern
		func(msg *nats.Msg) {
			t.Logf("TEST: Received SET message on subject '%s'", msg.Subject)

			var setCmd api.Set
			if json.Unmarshal(msg.Data, &setCmd) == nil {
				t.Logf(
					"TEST: Parsed SET command: property=%s, index=%d, processId=%d, chunkId=%d",
					setCmd.Property,
					setCmd.Index,
					setCmd.ProcessId,
					setCmd.ChunkId,
				)

				if setCmd.Property == "ARM" {
					t.Logf("TEST: ARM command detected, sending ARMED response")

					armed := api.Armed{
						ProcessId: setCmd.ProcessId,
						ChunkId:   setCmd.ChunkId,
					}
					armedData, _ := json.Marshal(armed)

					// Extract instrument name from the SET subject and create
					// ARMED subject
					subjectParts := strings.Split(msg.Subject, ".")
					if len(subjectParts) >= 2 {
						instrumentName := subjectParts[1]
						armedSubject := measure.ArmedMessage + "." + instrumentName
						t.Logf(
							"TEST: Publishing ARMED to subject '%s'",
							armedSubject,
						)
						if err := nc.Publish(armedSubject, armedData); err != nil {
							t.Logf("TEST: Failed to publish ARMED: %v", err)
						} else {
							t.Logf("TEST: Successfully published ARMED")
						}
					} else {
						t.Logf("TEST: Invalid SET subject format: %s", msg.Subject)
					}
				}
			} else {
				t.Logf("TEST: Failed to unmarshal SET command from subject '%s'", msg.Subject)
			}
		},
	)
	require.NoError(t, err)
	defer setSub.Unsubscribe()

	// Send multiple measurements
	setterInstruction := map[string]interface{}{
		"setter":   string(createTestPortJSON("setter_port")),
		"property": []string{"knobs"},
		"values":   []interface{}{5.0},
	}
	setterInstructionJSON, _ := json.Marshal(setterInstruction)

	for i := 1; i <= 5; i++ {
		measurementReady := api.MeasurementReady{
			ProcessId: int64(i),
			Getters:   []string{string(createTestPortJSON("getter_port"))},
			Setters:   []string{string(setterInstructionJSON)},
			Buffered:  true,
		}

		measurementData, _ := json.Marshal(measurementReady)
		nc.Publish(measure.MeasurementReadyMessage, measurementData)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Verify ChunkIds were assigned sequentially
	chunkIdMutex.Lock()
	assert.GreaterOrEqual(
		t,
		len(receivedChunkIds),
		5,
		"Should have received ChunkIds for all measurements",
	)

	// Check that we got sequential ChunkIds starting from 1
	expectedChunkIds := make(map[int64]bool)
	for i := int64(1); i <= 5; i++ {
		expectedChunkIds[i] = false
	}

	for _, chunkId := range receivedChunkIds {
		if _, exists := expectedChunkIds[chunkId]; exists {
			expectedChunkIds[chunkId] = true
		}
	}

	for i := int64(1); i <= 5; i++ {
		assert.True(
			t,
			expectedChunkIds[i],
			"Should have assigned ChunkId %d",
			i,
		)
	}
	chunkIdMutex.Unlock()

	// Verify nextChunkId has been incremented
	assert.Equal(
		t,
		int64(6),
		handler.NextChunkId,
		"nextChunkId should be 6 after 5 measurements",
	)
}

func TestMeasurementReadyHandler_BufferedMeasurement_MultipleSetter_Error(
	t *testing.T,
) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := measure.NewMeasurementReadyHandler(
		logger,
		instrumentHandler,
		cfg,
	)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create buffered measurement request with multiple setters (should log
	// error)
	getterPortJSON := createTestPortJSON("getter_port")

	setterInstruction := map[string]interface{}{
		"setter":   string(createTestPortJSON("setter_port")),
		"property": []string{"knobs"},
		"values":   []interface{}{5.0},
	}
	setterInstructionJSON, _ := json.Marshal(setterInstruction)

	measurementReady := api.MeasurementReady{
		ProcessId: 3,
		Getters:   []string{string(getterPortJSON)},
		Setters: []string{
			string(setterInstructionJSON),
			string(setterInstructionJSON),
		}, // Multiple setters
		Buffered: true,
	}

	measurementData, err := json.Marshal(measurementReady)
	require.NoError(t, err)

	// Count triggers - should still get both getter and setter triggers
	var getterTriggerCount int64
	var setterTriggerCount int64

	// Subscribe to SET commands to simulate instruments responding to ARM
	// commands
	setSub, err := nc.Subscribe(
		"SET.setter_instrument",
		func(msg *nats.Msg) {
			var setCmd api.Set
			if err := json.Unmarshal(msg.Data, &setCmd); err != nil {
				t.Logf("Failed to unmarshal SET command: %v", err)
				return
			}

			t.Logf(
				"TEST: Received SET command: property=%s, index=%d, processId=%d, chunkId=%d",
				setCmd.Property,
				setCmd.Index,
				setCmd.ProcessId,
				setCmd.ChunkId,
			)

			// When instrument receives ARM command, it responds with ARMED
			if setCmd.Property == "ARM" {
				t.Logf("TEST: ARM command received, sending ARMED response")

				armed := api.Armed{
					ProcessId: setCmd.ProcessId,
					ChunkId:   setCmd.ChunkId,
				}
				armedData, _ := json.Marshal(armed)
				armedSubject := "ARMED.setter_instrument"

				if err := nc.Publish(armedSubject, armedData); err != nil {
					t.Logf("TEST: Failed to publish ARMED: %v", err)
				} else {
					t.Logf("TEST: Published ARMED to %s", armedSubject)
				}
			}
		},
	)
	require.NoError(t, err)
	defer setSub.Unsubscribe()

	// Subscribe to TRIGGER commands to simulate instrument execution
	triggerSubGetter, err := nc.Subscribe(
		"TRIGGER.getter_instrument",
		func(msg *nats.Msg) {
			var triggerCmd api.Trigger
			if err := json.Unmarshal(msg.Data, &triggerCmd); err != nil {
				return
			}

			t.Logf("TEST: Received TRIGGER for getter")
			atomic.AddInt64(&getterTriggerCount, 1)

			// Getter responds with EXECUTING
			executing := api.Executing{
				ProcessId: triggerCmd.ProcessId,
				ChunkId:   triggerCmd.ChunkId,
			}
			executingData, _ := json.Marshal(executing)
			nc.Publish("EXECUTING.getter_instrument", executingData)
		},
	)
	require.NoError(t, err)
	defer triggerSubGetter.Unsubscribe()

	// Subscribe to TRIGGER commands for setter
	triggerSubSetter, err := nc.Subscribe(
		"TRIGGER.setter_instrument",
		func(msg *nats.Msg) {
			t.Logf("TEST: Received TRIGGER for setter")
			atomic.AddInt64(&setterTriggerCount, 1)
		},
	)
	require.NoError(t, err)
	defer triggerSubSetter.Unsubscribe()

	// Send MEASUREMENT_READY message
	err = nc.Publish(measure.MeasurementReadyMessage, measurementData)
	require.NoError(t, err)

	// Wait a bit for processing
	time.Sleep(500 * time.Millisecond)

	// Check final counts
	totalTriggers := atomic.LoadInt64(
		&getterTriggerCount,
	) + atomic.LoadInt64(
		&setterTriggerCount,
	)

	t.Logf("Final counts:")
	t.Logf("  Getter triggers: %d", atomic.LoadInt64(&getterTriggerCount))
	t.Logf("  Setter triggers: %d", atomic.LoadInt64(&setterTriggerCount))
	t.Logf("  Total triggers: %d", totalTriggers)

	// Should process normally with first setter only
	// Expect both getter and setter to be triggered (total 2)
	assert.Equal(
		t,
		int64(2),
		totalTriggers,
		"Should have triggered both getter and setter instruments",
	)

	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&getterTriggerCount),
		"Should have triggered getter instrument once",
	)

	assert.Equal(
		t,
		int64(1),
		atomic.LoadInt64(&setterTriggerCount),
		"Should have triggered setter instrument once (ignoring duplicate setters)",
	)
}

func TestMeasurementReadyHandler_NoGetters_Error(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)

	instrumentHandler := setupTestInstrumentHandler2(t)
	cfg := &config.Config{
		DeviceConfigPath: filepath.Join(tempDir, "device_config.json"),
		WiremapPath:      filepath.Join(tempDir, "wiremap.json"),
	}

	handler := measure.NewMeasurementReadyHandler(
		logger,
		instrumentHandler,
		cfg,
	)

	// Setup NATS connection
	nc := setupTestNATSServer(t)
	defer nc.Close()

	err = handler.Subscribe(nc)
	require.NoError(t, err)
	defer handler.Unsubscribe()

	// Create measurement request with no getters
	measurementReady := api.MeasurementReady{
		ProcessId: 5,
		Getters:   []string{}, // No getters
		Setters:   []string{},
	}

	measurementData, err := json.Marshal(measurementReady)
	require.NoError(t, err)

	// Send MEASUREMENT_READY message
	err = nc.Publish(measure.MeasurementReadyMessage, measurementData)
	require.NoError(t, err)

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Test should complete without crashing (error handling verification)
}
