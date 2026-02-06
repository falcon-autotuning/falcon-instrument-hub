// Package serverinterpreter provides instruction types for measurement chunking.
//
// These types mirror the Go Instruction and MeasurementInstructions classes
// from interpreter_daemon.py and instructions.py.
package serverinterpreter

import (
	"encoding/json"
	"fmt"
)

// Instruction represents a single measurement step with getters, setters, and requirements.
// This mirrors the Go Instruction dataclass.
type Instruction struct {
	// Getters are the InstrumentPorts to read from (JSON-serialized)
	Getters []string

	// Setters are the InstrumentPorts to write to (JSON-serialized)
	Setters []string

	// Requirements maps InstrumentPort (JSON) -> PropertyName -> PropertyValue
	Requirements map[string]map[string]interface{}

	// Buffered indicates if this is a buffered measurement step
	Buffered bool
}

// NewInstruction creates a new instruction with the given getters.
func NewInstruction(getters []string, buffered bool) *Instruction {
	return &Instruction{
		Getters:      getters,
		Setters:      make([]string, 0),
		Requirements: make(map[string]map[string]interface{}),
		Buffered:     buffered,
	}
}

// AddRequirement adds a requirement for an instrument.
func (i *Instruction) AddRequirement(instrumentJSON string, properties map[string]interface{}) {
	if i.Requirements == nil {
		i.Requirements = make(map[string]map[string]interface{})
	}
	i.Requirements[instrumentJSON] = properties
}

// AddSetter adds a setter to the instruction.
func (i *Instruction) AddSetter(setterJSON string) {
	i.Setters = append(i.Setters, setterJSON)
}

// RetrieveVoltageStates extracts voltage states from the requirements.
// Returns a map of InstrumentPort JSON -> voltage value.
func (i *Instruction) RetrieveVoltageStates() map[string]float64 {
	states := make(map[string]float64)
	for portJSON, props := range i.Requirements {
		if val, ok := props[SupportedProperties.VoltageState]; ok {
			if voltage, ok := val.(float64); ok {
				states[portJSON] = voltage
			}
		}
	}
	return states
}

// ContainsBufferedMeasurement checks if any requirement contains a staircase.
// Returns the number of divisions in the staircase, or 0 if none.
func (i *Instruction) ContainsBufferedMeasurement() int {
	for _, props := range i.Requirements {
		if staircase, ok := props[SupportedProperties.Staircase]; ok {
			if sc, ok := staircase.(StaircaseConfig); ok {
				return sc.NumSteps
			}
			// Try to extract from slice/array
			if arr, ok := staircase.([]interface{}); ok && len(arr) >= 2 {
				if numSteps, ok := arr[1].(float64); ok {
					return int(numSteps)
				}
			}
		}
	}
	return 0
}

// RetrieveBufferedVoltageStates extracts voltage states for each step of a buffered measurement.
func (i *Instruction) RetrieveBufferedVoltageStates(numDivisions int) []map[string]float64 {
	results := make([]map[string]float64, numDivisions)
	for idx := 0; idx < numDivisions; idx++ {
		results[idx] = make(map[string]float64)
	}

	for portJSON, props := range i.Requirements {
		if staircase, ok := props[SupportedProperties.Staircase]; ok {
			var sc StaircaseConfig
			switch v := staircase.(type) {
			case StaircaseConfig:
				sc = v
			case []interface{}:
				if len(v) >= 5 {
					sc = StaircaseConfig{
						StepWidthMs: toFloat64(v[0]),
						NumSteps:    int(toFloat64(v[1])),
						Offset:      toFloat64(v[2]),
						VStart:      toFloat64(v[3]),
						VStop:       toFloat64(v[4]),
					}
				}
			}

			if sc.NumSteps > 0 {
				step := (sc.VStop - sc.VStart) / float64(sc.NumSteps-1)
				for idx := 0; idx < numDivisions && idx < sc.NumSteps; idx++ {
					results[idx][portJSON] = sc.VStart + float64(idx)*step
				}
			}
		}
	}

	return results
}

// StaircaseConfig represents a staircase waveform configuration.
type StaircaseConfig struct {
	StepWidthMs float64 `json:"step_width_ms"` // Width of each step in milliseconds
	NumSteps    int     `json:"num_steps"`     // Number of steps
	Offset      float64 `json:"offset"`        // Offset value
	VStart      float64 `json:"v_start"`       // Starting voltage
	VStop       float64 `json:"v_stop"`        // Ending voltage
}

// MeasurementInstructions holds a sequence of instructions for a measurement.
type MeasurementInstructions struct {
	Instructions []*Instruction
}

// NewMeasurementInstructions creates a new measurement instructions container.
func NewMeasurementInstructions(instructions []*Instruction) *MeasurementInstructions {
	return &MeasurementInstructions{Instructions: instructions}
}

// Len returns the number of instructions.
func (m *MeasurementInstructions) Len() int {
	return len(m.Instructions)
}

// At returns the instruction at the given index.
func (m *MeasurementInstructions) At(idx int) *Instruction {
	if idx < 0 || idx >= len(m.Instructions) {
		return nil
	}
	return m.Instructions[idx]
}

// PendingMeasurement tracks a measurement that is waiting for data collection.
type PendingMeasurement struct {
	MeasurementID int64
	ExpectedCount int
	DataPath      string
	Shape         []int
	RequestJSON   string // Original MeasurementRequest JSON
	CollectedData []*DataEntry
	CreatedAt     int64
}

// IsComplete checks if all expected data has been collected.
func (p *PendingMeasurement) IsComplete() bool {
	return len(p.CollectedData) >= p.ExpectedCount
}

// CompletionPercentage returns the percentage of data collected.
func (p *PendingMeasurement) CompletionPercentage() float64 {
	if p.ExpectedCount == 0 {
		return 0
	}
	return float64(len(p.CollectedData)) / float64(p.ExpectedCount) * 100
}

// AddDataEntry adds a data entry to the pending measurement.
func (p *PendingMeasurement) AddDataEntry(entry *DataEntry) {
	p.CollectedData = append(p.CollectedData, entry)
}

// GetSortedChunkData returns data organized by chunk ID.
func (p *PendingMeasurement) GetSortedChunkData() map[int64]map[string][]float64 {
	result := make(map[int64]map[string][]float64)
	for _, entry := range p.CollectedData {
		if _, exists := result[entry.ChunkID]; !exists {
			result[entry.ChunkID] = make(map[string][]float64)
		}
		for portJSON, data := range entry.Data {
			result[entry.ChunkID][portJSON] = data
		}
	}
	return result
}

// DataEntry represents a single chunk of collected data.
type DataEntry struct {
	MeasurementID int64
	ChunkID       int64
	Data          map[string][]float64 // InstrumentPort JSON -> measured values
	Timestamp     int64
}

// InstrumentConfiguration holds configuration for an instrument.
type InstrumentConfiguration struct {
	Properties map[string]interface{} `json:"properties"`
}

// ConfigurationMap maps InstrumentPort JSON to its configuration.
type ConfigurationMap map[string]InstrumentConfiguration

// ParseConfigurationsJSON parses a configurations JSON string.
func ParseConfigurationsJSON(jsonStr string) (ConfigurationMap, error) {
	var raw map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse configurations: %w", err)
	}

	config := make(ConfigurationMap)
	for portJSON, props := range raw {
		config[portJSON] = InstrumentConfiguration{Properties: props}
	}
	return config, nil
}

// GetProperty retrieves a property value from the configuration.
func (c ConfigurationMap) GetProperty(portJSON, property string) (interface{}, bool) {
	if cfg, exists := c[portJSON]; exists {
		if val, ok := cfg.Properties[property]; ok {
			return val, true
		}
	}
	return nil, false
}

// GetFloatProperty retrieves a float property with a default value.
func (c ConfigurationMap) GetFloatProperty(portJSON, property string, defaultVal float64) float64 {
	if val, ok := c.GetProperty(portJSON, property); ok {
		return toFloat64(val)
	}
	return defaultVal
}

// GetIntProperty retrieves an int property with a default value.
func (c ConfigurationMap) GetIntProperty(portJSON, property string, defaultVal int) int {
	if val, ok := c.GetProperty(portJSON, property); ok {
		return int(toFloat64(val))
	}
	return defaultVal
}

// GetBoolProperty retrieves a bool property with a default value.
func (c ConfigurationMap) GetBoolProperty(portJSON, property string, defaultVal bool) bool {
	if val, ok := c.GetProperty(portJSON, property); ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultVal
}

// toFloat64 converts various numeric types to float64.
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	default:
		return 0
	}
}
