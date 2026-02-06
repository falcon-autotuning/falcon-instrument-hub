// Package serverinterpreter provides tests for quantum dot device measurement scenarios.
//
// These tests validate the server interpreter's ability to handle:
// - Multi-channel voltage setting on quantum dot gates
// - 1D voltage sweeps with current measurement between ohmics
// - Complex measurement routines typical of quantum dot tuning
package serverinterpreter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Quantum Dot Device Configuration Types
// =============================================================================

// QuantumDotDevice represents a quantum dot device configuration.
// This models a typical semiconductor quantum dot with:
// - Multiple gate channels (plunger gates, barrier gates, etc.)
// - Ohmic contacts for current measurement
// - A DAC for voltage control
// - A current meter (DMM/lockin) for ohmic-to-ohmic current measurement
type QuantumDotDevice struct {
	Name         string                  `json:"name"`
	Gates        []GateChannel           `json:"gates"`
	Ohmics       []OhmicContact          `json:"ohmics"`
	CurrentMeter OhmicCurrentMeter       `json:"current_meter"`
}

// GateChannel represents a voltage-controlled gate on the quantum dot.
type GateChannel struct {
	Name        string  `json:"name"`         // e.g., "P1", "B1", "LP", "RP"
	DACId       string  `json:"dac_id"`       // DAC instrument ID
	Channel     int     `json:"channel"`      // DAC channel number
	MinVoltage  float64 `json:"min_voltage"`  // Safe min voltage
	MaxVoltage  float64 `json:"max_voltage"`  // Safe max voltage
	Description string  `json:"description"`  // e.g., "Left plunger gate"
}

// OhmicContact represents an ohmic contact on the device.
type OhmicContact struct {
	Name    string `json:"name"`    // e.g., "source", "drain"
	Channel int    `json:"channel"` // Ohmic channel number
}

// OhmicCurrentMeter represents the current measurement between ohmics.
type OhmicCurrentMeter struct {
	InstrumentId   string `json:"instrument_id"`   // e.g., "DMM1" or "LOCKIN1"
	SourceOhmic    string `json:"source_ohmic"`    // Source ohmic name
	DrainOhmic     string `json:"drain_ohmic"`     // Drain ohmic name
	CurrentChannel int    `json:"current_channel"` // Meter channel for current
}

// =============================================================================
// Test Fixtures
// =============================================================================

// createTestQuantumDotDevice creates a typical 5-gate quantum dot device
// configuration for testing purposes.
func createTestQuantumDotDevice() QuantumDotDevice {
	return QuantumDotDevice{
		Name: "QD1",
		Gates: []GateChannel{
			{Name: "P1", DACId: "QDAC1", Channel: 1, MinVoltage: -2.0, MaxVoltage: 0.5, Description: "Plunger gate 1"},
			{Name: "P2", DACId: "QDAC1", Channel: 2, MinVoltage: -2.0, MaxVoltage: 0.5, Description: "Plunger gate 2"},
			{Name: "B1", DACId: "QDAC1", Channel: 3, MinVoltage: -2.0, MaxVoltage: 0.5, Description: "Barrier gate 1"},
			{Name: "B2", DACId: "QDAC1", Channel: 4, MinVoltage: -2.0, MaxVoltage: 0.5, Description: "Barrier gate 2"},
			{Name: "B3", DACId: "QDAC1", Channel: 5, MinVoltage: -2.0, MaxVoltage: 0.5, Description: "Barrier gate 3"},
		},
		Ohmics: []OhmicContact{
			{Name: "source", Channel: 1},
			{Name: "drain", Channel: 2},
		},
		CurrentMeter: OhmicCurrentMeter{
			InstrumentId:   "DMM1",
			SourceOhmic:    "source",
			DrainOhmic:     "drain",
			CurrentChannel: 0,
		},
	}
}

// =============================================================================
// Test 1: Setting Voltages on Multiple Quantum Dot Channels
// =============================================================================

func TestQuantumDot_SetMultipleGateVoltages(t *testing.T) {
	device := createTestQuantumDotDevice()

	t.Run("set all gates to initial tuning point", func(t *testing.T) {
		// Typical scenario: Initialize all gates to a starting voltage
		gateVoltages := map[string]float64{
			"P1": -0.5,  // Plunger 1
			"P2": -0.6,  // Plunger 2
			"B1": -1.0,  // Barrier 1 (more negative to deplete)
			"B2": -1.0,  // Barrier 2
			"B3": -1.0,  // Barrier 3
		}

		// Build the measurement request JSON
		setters := make([]map[string]interface{}, 0)
		setVoltages := make(map[string]float64)

		for _, gate := range device.Gates {
			voltage, exists := gateVoltages[gate.Name]
			require.True(t, exists, "Missing voltage for gate %s", gate.Name)

			setter := map[string]interface{}{
				"id":      gate.DACId,
				"channel": gate.Channel,
			}
			setters = append(setters, setter)

			// Key format: "QDAC1:1" for channel 1, etc.
			key := InstrumentTarget{Id: gate.DACId, Channel: gate.Channel}.Serialize()
			setVoltages[key] = voltage
		}

		requestJSON := map[string]interface{}{
			"measurementName": "initialize_quantum_dot_gates",
			"message":         "Set all QD1 gates to initial tuning point",
			"setters":         setters,
			"setVoltages":     setVoltages,
		}

		jsonBytes, err := json.Marshal(requestJSON)
		require.NoError(t, err)

		// Parse and validate
		parsed, err := ParseMeasurementRequestJSON(string(jsonBytes))
		require.NoError(t, err)

		assert.Equal(t, "initialize_quantum_dot_gates", parsed.MeasurementName)
		assert.Len(t, parsed.Setters, 5, "Should have 5 gate setters")

		// Verify all voltages are correctly associated
		setVoltageRequests := parsed.ToSetVoltageRequests()
		assert.Len(t, setVoltageRequests, 5, "Should generate 5 set voltage requests")

		// Verify correct voltage values
		for _, req := range setVoltageRequests {
			key := req.Setter.Serialize()
			expectedVoltage := setVoltages[key]
			assert.InDelta(t, expectedVoltage, req.SetVoltage, 0.001,
				"Voltage mismatch for %s", key)
		}
	})

	t.Run("set subset of gates for fine tuning", func(t *testing.T) {
		// Scenario: Only adjust plunger gates while barriers stay fixed
		plungerVoltages := map[string]float64{
			"P1": -0.45,
			"P2": -0.55,
		}

		setters := make([]map[string]interface{}, 0)
		setVoltages := make(map[string]float64)

		for _, gate := range device.Gates {
			if voltage, exists := plungerVoltages[gate.Name]; exists {
				setter := map[string]interface{}{
					"id":      gate.DACId,
					"channel": gate.Channel,
				}
				setters = append(setters, setter)

				key := InstrumentTarget{Id: gate.DACId, Channel: gate.Channel}.Serialize()
				setVoltages[key] = voltage
			}
		}

		requestJSON := map[string]interface{}{
			"measurementName": "tune_plunger_gates",
			"setters":         setters,
			"setVoltages":     setVoltages,
		}

		jsonBytes, err := json.Marshal(requestJSON)
		require.NoError(t, err)

		parsed, err := ParseMeasurementRequestJSON(string(jsonBytes))
		require.NoError(t, err)

		assert.Len(t, parsed.Setters, 2, "Should only have 2 plunger setters")
		setVoltageRequests := parsed.ToSetVoltageRequests()
		assert.Len(t, setVoltageRequests, 2)
	})

	t.Run("validate voltage bounds checking", func(t *testing.T) {
		// Create a request that would set a voltage
		gate := device.Gates[0] // P1

		// Test within bounds
		validVoltage := (gate.MinVoltage + gate.MaxVoltage) / 2
		assert.GreaterOrEqual(t, validVoltage, gate.MinVoltage)
		assert.LessOrEqual(t, validVoltage, gate.MaxVoltage)

		// Create request
		requestJSON := map[string]interface{}{
			"measurementName": "set_single_gate",
			"setters": []map[string]interface{}{
				{"id": gate.DACId, "channel": gate.Channel},
			},
			"setVoltages": map[string]float64{
				InstrumentTarget{Id: gate.DACId, Channel: gate.Channel}.Serialize(): validVoltage,
			},
		}

		jsonBytes, err := json.Marshal(requestJSON)
		require.NoError(t, err)

		parsed, err := ParseMeasurementRequestJSON(string(jsonBytes))
		require.NoError(t, err)

		requests := parsed.ToSetVoltageRequests()
		require.Len(t, requests, 1)
		assert.Equal(t, validVoltage, requests[0].SetVoltage)
	})
}

func TestQuantumDot_SetVoltagesWithMockServer(t *testing.T) {
	device := createTestQuantumDotDevice()
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	// Parse mock server URL
	urlParts := parseTestServerURL(mock.URL())
	
	config := BridgeConfig{
		ScriptServerHost: urlParts.host,
		ScriptServerPort: urlParts.port,
		ScriptOutputDir:  t.TempDir(),
	}

	bridge, err := NewBridge(config)
	require.NoError(t, err)

	t.Run("execute multi-gate voltage set via bridge", func(t *testing.T) {
		// Set P1 and B1 gates
		gateVoltages := []struct {
			gate    GateChannel
			voltage float64
		}{
			{device.Gates[0], -0.5}, // P1
			{device.Gates[2], -1.0}, // B1
		}

		for _, gv := range gateVoltages {
			result, err := bridge.ExecuteSetVoltage(gv.gate.DACId, gv.gate.Channel, gv.voltage)
			require.NoError(t, err)
			assert.Equal(t, "completed", result.Status)
		}

		// Verify requests were sent
		requests := mock.GetRequests()
		assert.GreaterOrEqual(t, len(requests), 2, "Should have at least 2 requests")
	})
}

// =============================================================================
// Test 2: 1D Voltage Sweep with Current Measurement
// =============================================================================

// VoltageSweepConfig defines a 1D voltage sweep configuration.
type VoltageSweepConfig struct {
	SweepGate    GateChannel   // The gate to sweep
	StartVoltage float64       // Sweep start voltage
	StopVoltage  float64       // Sweep end voltage
	NumPoints    int           // Number of points in sweep
	StepTimeMs   float64       // Time per step in milliseconds
	Meter        CurrentMeterConfig
}

// CurrentMeterConfig defines current measurement configuration.
type CurrentMeterConfig struct {
	InstrumentId string  // DMM or lockin ID
	Channel      int     // Measurement channel
	SampleRate   int     // Samples per second
	NumSamples   int     // Samples per point
}

func TestQuantumDot_1DVoltageSweepCurrentMeasurement(t *testing.T) {
	device := createTestQuantumDotDevice()

	t.Run("create 1D sweep request on plunger gate", func(t *testing.T) {
		// Define sweep: P1 from -1V to -0.5V, 101 points, measuring source-drain current
		sweepConfig := VoltageSweepConfig{
			SweepGate:    device.Gates[0], // P1
			StartVoltage: -1.0,
			StopVoltage:  -0.5,
			NumPoints:    101,
			StepTimeMs:   10.0,
			Meter: CurrentMeterConfig{
				InstrumentId: device.CurrentMeter.InstrumentId,
				Channel:      device.CurrentMeter.CurrentChannel,
				SampleRate:   1000,
				NumSamples:   10,
			},
		}

		// Build waveform data for the sweep
		waveformData := buildSweepWaveformData(sweepConfig)

		assert.Equal(t, sweepConfig.NumPoints, len(waveformData.RawTimeTrace))
		assert.Len(t, waveformData.Shape, 1)
		assert.Equal(t, sweepConfig.NumPoints, waveformData.Shape[0])

		// Verify voltage values are correctly distributed
		stepSize := (sweepConfig.StopVoltage - sweepConfig.StartVoltage) / float64(sweepConfig.NumPoints-1)
		for i, point := range waveformData.RawTimeTrace {
			expectedVoltage := sweepConfig.StartVoltage + float64(i)*stepSize
			assert.InDelta(t, expectedVoltage, point[0], 0.0001,
				"Voltage mismatch at point %d", i)
		}
	})

	t.Run("process sweep with waveform processor - unbuffered", func(t *testing.T) {
		sweepConfig := VoltageSweepConfig{
			SweepGate:    device.Gates[0], // P1
			StartVoltage: -1.0,
			StopVoltage:  -0.5,
			NumPoints:    11, // Smaller for test
			StepTimeMs:   10.0,
			Meter: CurrentMeterConfig{
				InstrumentId: device.CurrentMeter.InstrumentId,
				Channel:      device.CurrentMeter.CurrentChannel,
				SampleRate:   1000,
				NumSamples:   10,
			},
		}

		waveformData := buildSweepWaveformData(sweepConfig)
		getters := []GetterInfo{
			{PortJSON: mustMarshalJSON(t, map[string]interface{}{
				"id":      sweepConfig.Meter.InstrumentId,
				"channel": sweepConfig.Meter.Channel,
			})},
		}

		// Create processor without buffered measurement support
		config := make(ConfigurationMap)
		processor := NewWaveformProcessor(config)

		result, err := processor.ProcessWaveformData(waveformData, getters)
		require.NoError(t, err)

		// Unbuffered: should have one instruction per point
		assert.Equal(t, sweepConfig.NumPoints, result.DataCount)
		assert.Equal(t, sweepConfig.NumPoints, result.Instructions.Len())
	})

	t.Run("process sweep with waveform processor - buffered", func(t *testing.T) {
		sweepConfig := VoltageSweepConfig{
			SweepGate:    device.Gates[0],
			StartVoltage: -1.0,
			StopVoltage:  -0.5,
			NumPoints:    11,
			StepTimeMs:   10.0,
			Meter: CurrentMeterConfig{
				InstrumentId: device.CurrentMeter.InstrumentId,
				Channel:      device.CurrentMeter.CurrentChannel,
				SampleRate:   1000,
				NumSamples:   10,
			},
		}

		waveformData := buildSweepWaveformData(sweepConfig)
		
		// Build port JSONs
		gatePortJSON := mustMarshalJSON(t, map[string]interface{}{
			"id":      sweepConfig.SweepGate.DACId,
			"channel": sweepConfig.SweepGate.Channel,
		})
		meterPortJSON := mustMarshalJSON(t, map[string]interface{}{
			"id":      sweepConfig.Meter.InstrumentId,
			"channel": sweepConfig.Meter.Channel,
		})

		getters := []GetterInfo{{PortJSON: meterPortJSON}}

		// Add axis domain for the sweep gate
		waveformData.AxisDomains = [][]LabelledDomainInfo{
			{
				{
					LabelJSON: gatePortJSON,
					DomainBounds: DomainBounds{
						Min: sweepConfig.StartVoltage,
						Max: sweepConfig.StopVoltage,
					},
				},
			},
		}

		// Create processor with buffered measurement support
		config := make(ConfigurationMap)
		config[gatePortJSON] = InstrumentConfiguration{
			Properties: map[string]interface{}{
				SupportedProperties.SupportsBufferedMeasurements: true,
				SupportedProperties.Slope:                        100.0, // V/s
			},
		}
		config[meterPortJSON] = InstrumentConfiguration{
			Properties: map[string]interface{}{
				SupportedProperties.SupportsBufferedMeasurements: true,
				SupportedProperties.SampleRate:                   sweepConfig.Meter.SampleRate,
			},
		}

		processor := NewWaveformProcessor(config)
		result, err := processor.ProcessWaveformData(waveformData, getters)
		require.NoError(t, err)

		// Buffered: should have fewer instructions (chunks based on direction changes)
		assert.Greater(t, sweepConfig.NumPoints, 0)
		assert.NotNil(t, result.Instructions)
	})

	t.Run("verify sweep generates MEASUREMENT_READY messages", func(t *testing.T) {
		sweepConfig := VoltageSweepConfig{
			SweepGate:    device.Gates[0],
			StartVoltage: -1.0,
			StopVoltage:  -0.5,
			NumPoints:    5, // Small for testing
			StepTimeMs:   10.0,
			Meter: CurrentMeterConfig{
				InstrumentId: device.CurrentMeter.InstrumentId,
				Channel:      device.CurrentMeter.CurrentChannel,
				SampleRate:   1000,
				NumSamples:   10,
			},
		}

		waveformData := buildSweepWaveformData(sweepConfig)
		meterPortJSON := mustMarshalJSON(t, map[string]interface{}{
			"id":      sweepConfig.Meter.InstrumentId,
			"channel": sweepConfig.Meter.Channel,
		})
		getters := []GetterInfo{{PortJSON: meterPortJSON}}

		config := make(ConfigurationMap)
		processor := NewWaveformProcessor(config)

		result, err := processor.ProcessWaveformData(waveformData, getters)
		require.NoError(t, err)

		// Each instruction should have the getter
		for i := 0; i < result.Instructions.Len(); i++ {
			instruction := result.Instructions.At(i)
			assert.Contains(t, instruction.Getters, meterPortJSON,
				"Instruction %d should include the current meter", i)
		}
	})
}

func TestQuantumDot_1DSweepWithBridge(t *testing.T) {
	device := createTestQuantumDotDevice()
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := parseTestServerURL(mock.URL())

	config := BridgeConfig{
		ScriptServerHost: urlParts.host,
		ScriptServerPort: urlParts.port,
		ScriptOutputDir:  t.TempDir(),
	}

	bridge, err := NewBridge(config)
	require.NoError(t, err)

	t.Run("execute 1D sweep measurement request", func(t *testing.T) {
		// Build a sweep request JSON similar to what falcon-core generates
		sweepGate := device.Gates[0] // P1
		
		requestJSON := map[string]interface{}{
			"measurementName": "1d_plunger_sweep",
			"message":         "Sweep P1 gate and measure source-drain current",
			"setters": []map[string]interface{}{
				{
					"id":      sweepGate.DACId,
					"channel": sweepGate.Channel,
				},
			},
			"getters": []map[string]interface{}{
				{
					"id":      device.CurrentMeter.InstrumentId,
					"channel": device.CurrentMeter.CurrentChannel,
				},
			},
			// Simplified: set the starting voltage
			"setVoltages": map[string]float64{
				InstrumentTarget{Id: sweepGate.DACId, Channel: sweepGate.Channel}.Serialize(): -1.0,
			},
		}

		jsonBytes, err := json.Marshal(requestJSON)
		require.NoError(t, err)

		result, err := bridge.ExecuteMeasurementRequestJSON(string(jsonBytes))
		require.NoError(t, err)
		assert.Equal(t, "completed", result.Status)
	})
}

// =============================================================================
// Test 3: Combined Scenarios - Real Tuning Operations
// =============================================================================

func TestQuantumDot_RealTuningScenarios(t *testing.T) {
	device := createTestQuantumDotDevice()

	t.Run("pinch-off measurement - sweep barrier and measure current", func(t *testing.T) {
		// Pinch-off: Sweep a barrier gate from 0V to -2V and measure current
		barrier := device.Gates[2] // B1
		
		requestJSON := map[string]interface{}{
			"measurementName": "pinch_off_B1",
			"message":         "Measure pinch-off curve for barrier B1",
			"setters": []map[string]interface{}{
				{"id": barrier.DACId, "channel": barrier.Channel},
			},
			"getters": []map[string]interface{}{
				{
					"id":      device.CurrentMeter.InstrumentId,
					"channel": device.CurrentMeter.CurrentChannel,
				},
			},
			"waveforms": []map[string]interface{}{
				{
					"transforms": []map[string]interface{}{
						{
							"port": map[string]interface{}{
								"default_name":    barrier.Name,
								"id":              barrier.DACId,
								"channel":         barrier.Channel,
								"instrument_type": "DAC",
								"is_knob":         true,
							},
						},
					},
				},
			},
		}

		jsonBytes, err := json.Marshal(requestJSON)
		require.NoError(t, err)

		parsed, err := ParseMeasurementRequestJSON(string(jsonBytes))
		require.NoError(t, err)

		assert.Equal(t, "pinch_off_B1", parsed.MeasurementName)
		assert.Len(t, parsed.Setters, 1)
		assert.Len(t, parsed.Getters, 1)
	})

	t.Run("coulomb diamond preparation - set multiple gates", func(t *testing.T) {
		// Before measuring Coulomb diamonds, set all gates to operating point
		operatingPoint := map[string]float64{
			"P1": -0.42,  // Fine-tuned plunger voltage
			"P2": -0.58,
			"B1": -0.95,  // Barriers defining the dot
			"B2": -0.92,
			"B3": -0.98,
		}

		setters := make([]map[string]interface{}, 0)
		setVoltages := make(map[string]float64)

		for _, gate := range device.Gates {
			if voltage, exists := operatingPoint[gate.Name]; exists {
				setters = append(setters, map[string]interface{}{
					"id":      gate.DACId,
					"channel": gate.Channel,
				})
				key := InstrumentTarget{Id: gate.DACId, Channel: gate.Channel}.Serialize()
				setVoltages[key] = voltage
			}
		}

		requestJSON := map[string]interface{}{
			"measurementName": "coulomb_diamond_preparation",
			"message":         "Set gates to Coulomb diamond operating point",
			"setters":         setters,
			"setVoltages":     setVoltages,
		}

		jsonBytes, err := json.Marshal(requestJSON)
		require.NoError(t, err)

		parsed, err := ParseMeasurementRequestJSON(string(jsonBytes))
		require.NoError(t, err)

		requests := parsed.ToSetVoltageRequests()
		assert.Len(t, requests, 5)

		// Verify each gate gets correct voltage
		for _, req := range requests {
			key := req.Setter.Serialize()
			assert.Contains(t, setVoltages, key)
			assert.InDelta(t, setVoltages[key], req.SetVoltage, 0.001)
		}
	})

	t.Run("charge stability diagram - 2D sweep both plungers", func(t *testing.T) {
		// 2D sweep: P1 on fast axis, P2 on slow axis
		// This creates a charge stability diagram
		p1 := device.Gates[0]
		p2 := device.Gates[1]

		requestJSON := map[string]interface{}{
			"measurementName": "charge_stability_diagram",
			"message":         "2D sweep P1 vs P2 for charge stability",
			"setters": []map[string]interface{}{
				{"id": p1.DACId, "channel": p1.Channel},
				{"id": p2.DACId, "channel": p2.Channel},
			},
			"getters": []map[string]interface{}{
				{
					"id":      device.CurrentMeter.InstrumentId,
					"channel": device.CurrentMeter.CurrentChannel,
				},
			},
		}

		jsonBytes, err := json.Marshal(requestJSON)
		require.NoError(t, err)

		parsed, err := ParseMeasurementRequestJSON(string(jsonBytes))
		require.NoError(t, err)

		assert.Equal(t, "charge_stability_diagram", parsed.MeasurementName)
		assert.Len(t, parsed.Setters, 2, "Should have both plunger gates as setters")
		assert.Len(t, parsed.Getters, 1, "Should have current meter as getter")
	})
}

// =============================================================================
// Test NATS Message Format Validation
// =============================================================================

func TestQuantumDot_NATSMessageFormats(t *testing.T) {
	device := createTestQuantumDotDevice()

	t.Run("ProcessRequestMessage for multi-gate set", func(t *testing.T) {
		// Build the inner measurement request
		setters := make([]map[string]interface{}, 0)
		for _, gate := range device.Gates[:2] { // First 2 gates
			setters = append(setters, map[string]interface{}{
				"id":      gate.DACId,
				"channel": gate.Channel,
			})
		}

		innerRequest := map[string]interface{}{
			"measurementName": "set_gates",
			"setters":         setters,
			"setVoltages": map[string]float64{
				"QDAC1:1": -0.5,
				"QDAC1:2": -0.6,
			},
		}

		// Build ProcessRequestMessage
		processRequest := ProcessRequestMessage{
			ProcessID:      12345,
			Request:        innerRequest,
			Configurations: map[string]interface{}{},
			DataPath:       "/data/measurements/2026-02-06/",
		}

		jsonBytes, err := json.Marshal(processRequest)
		require.NoError(t, err)

		// Verify it can be parsed back
		var parsed ProcessRequestMessage
		err = json.Unmarshal(jsonBytes, &parsed)
		require.NoError(t, err)

		assert.Equal(t, int64(12345), parsed.ProcessID)
		assert.NotNil(t, parsed.Request)
	})

	t.Run("MeasurementReadyMessage for sweep step", func(t *testing.T) {
		gate := device.Gates[0]
		gatePortJSON := mustMarshalJSON(t, map[string]interface{}{
			"id":      gate.DACId,
			"channel": gate.Channel,
		})
		meterPortJSON := mustMarshalJSON(t, map[string]interface{}{
			"id":      device.CurrentMeter.InstrumentId,
			"channel": device.CurrentMeter.CurrentChannel,
		})

		readyMsg := MeasurementReadyMessage{
			Timestamp:    1707235200,
			Getters:      []string{meterPortJSON},
			Setters:      []string{gatePortJSON},
			Requirements: []string{},
			HasSet:       true,
			HasTrigger:   false,
			IsBuffered:   false,
			ProcessID:    12345,
			ChunkID:      0,
		}

		jsonBytes, err := json.Marshal(readyMsg)
		require.NoError(t, err)

		var parsed MeasurementReadyMessage
		err = json.Unmarshal(jsonBytes, &parsed)
		require.NoError(t, err)

		assert.Len(t, parsed.Getters, 1)
		assert.Len(t, parsed.Setters, 1)
		assert.True(t, parsed.HasSet)
	})

	t.Run("UploadDataMessage for sweep result", func(t *testing.T) {
		uploadMsg := UploadDataMessage{
			Timestamp: 1707235300,
			ProcessID: 12345,
			UnitHash:  67890,
			Channel:   "measurement.data.12345",
			Stream:    "MEASUREMENT_DATA",
		}

		jsonBytes, err := json.Marshal(uploadMsg)
		require.NoError(t, err)

		var parsed UploadDataMessage
		err = json.Unmarshal(jsonBytes, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "measurement.data.12345", parsed.Channel)
		assert.Equal(t, "MEASUREMENT_DATA", parsed.Stream)
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

// buildSweepWaveformData creates WaveformData for a 1D voltage sweep.
func buildSweepWaveformData(config VoltageSweepConfig) *WaveformData {
	// Generate voltage points
	voltages := make([][]float64, config.NumPoints)
	stepSize := (config.StopVoltage - config.StartVoltage) / float64(config.NumPoints-1)

	for i := 0; i < config.NumPoints; i++ {
		voltage := config.StartVoltage + float64(i)*stepSize
		voltages[i] = []float64{voltage}
	}

	return &WaveformData{
		RawTimeTrace: voltages,
		AxisDomains:  [][]LabelledDomainInfo{},
		TimeDomain: DomainBounds{
			Min: 0,
			Max: config.StepTimeMs * float64(config.NumPoints) / 1000.0, // Total time in seconds
		},
		Shape: []int{config.NumPoints},
	}
}

// testServerURL holds parsed host/port for mock server.
type testServerURL struct {
	host string
	port int
}

// parseTestServerURL parses a test server URL like "http://127.0.0.1:12345"
func parseTestServerURL(url string) testServerURL {
	// Remove scheme
	url = url[len("http://"):]
	
	// Split host:port
	colonIdx := len(url) - 1
	for colonIdx >= 0 && url[colonIdx] != ':' {
		colonIdx--
	}

	host := url[:colonIdx]
	portStr := url[colonIdx+1:]
	
	var port int
	json.Unmarshal([]byte(portStr), &port)

	return testServerURL{host: host, port: port}
}

// mustMarshalJSON marshals to JSON or fails the test.
func mustMarshalJSON(t *testing.T, v interface{}) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}
