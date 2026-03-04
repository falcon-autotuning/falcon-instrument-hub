// Package serverinterpreter provides waveform processing and chunking logic.
//
// This file implements the waveform chunking algorithm from the
// InterpreterDaemon.process_request() and chunk_instructions() methods.
package serverinterpreter

import (
	"fmt"
	"math"
	"sort"
)

// WaveformProcessor handles the breakdown of measurement requests into instructions.
type WaveformProcessor struct {
	config ConfigurationMap
}

// NewWaveformProcessor creates a new waveform processor.
func NewWaveformProcessor(config ConfigurationMap) *WaveformProcessor {
	return &WaveformProcessor{config: config}
}

// ProcessRequestResult contains the results of processing a measurement request.
type ProcessRequestResult struct {
	Instructions *MeasurementInstructions
	DataCount    int   // Number of chunks/measurements expected
	Shape        []int // Shape of the final data array
}

// WaveformData represents the extracted data from a falcon-core waveform.
// This is an intermediate representation for processing.
type WaveformData struct {
	// RawTimeTrace is the compiled discrete space data [n_points, n_axes]
	RawTimeTrace [][]float64

	// AxisDomains contains the domain information for each axis
	// Each axis has one or more coupled labelled domains
	AxisDomains [][]LabelledDomainInfo

	// TimeDomain contains the unit/time domain bounds
	TimeDomain DomainBounds

	// Shape is the shape of the waveform space
	Shape []int
}

// LabelledDomainInfo contains information about a labelled domain (knob).
type LabelledDomainInfo struct {
	LabelJSON    string       // InstrumentPort JSON (the knob)
	DomainBounds DomainBounds // The voltage bounds for this label
}

// DomainBounds represents the bounds of a domain.
type DomainBounds struct {
	Min float64
	Max float64
}

// Range returns the range of the domain.
func (d DomainBounds) Range() float64 {
	return d.Max - d.Min
}

// Transform transforms a normalized value [0,1] to the domain range.
func (d DomainBounds) Transform(normalizedValue float64) float64 {
	return d.Min + normalizedValue*d.Range()
}

// GetterInfo contains information about a getter (meter).
type GetterInfo struct {
	PortJSON string // InstrumentPort JSON
}

// ProcessWaveformData processes extracted waveform data into instructions.
// This mirrors the measurement request processor.
func (p *WaveformProcessor) ProcessWaveformData(
	waveform *WaveformData,
	getters []GetterInfo,
) (*ProcessRequestResult, error) {
	if waveform == nil || len(waveform.RawTimeTrace) == 0 {
		return nil, fmt.Errorf("no valid waveform data")
	}

	// Decide if buffered measurement is possible
	buffered := p.decideBuffered(waveform, getters)

	// Chunk the raw time trace
	chunks := p.chunkInstructions(waveform.RawTimeTrace, buffered)

	// Calculate step width from time domain
	stepWidthMs := int(math.Ceil(waveform.TimeDomain.Range() * 1000)) // milliseconds

	// Collect sample rates for getters
	sampleRates := p.collectSampleRates(getters)

	// Calculate number of samples per step for each getter
	numSamples := p.calculateNumSamplesPerStep(stepWidthMs, sampleRates)

	// Build instructions from chunks
	instructions := make([]*Instruction, 0, len(chunks))

	for _, chunk := range chunks {
		instruction := p.buildInstructionFromChunk(
			chunk,
			waveform,
			getters,
			stepWidthMs,
			numSamples,
			buffered,
		)
		instructions = append(instructions, instruction)
	}

	// If buffered, interject ramps between instructions
	if buffered {
		instructions = p.interjectRamps(instructions)
	}

	return &ProcessRequestResult{
		Instructions: NewMeasurementInstructions(instructions),
		DataCount:    len(chunks),
		Shape:        waveform.Shape,
	}, nil
}

// decideBuffered determines if a buffered measurement should be used.
func (p *WaveformProcessor) decideBuffered(waveform *WaveformData, getters []GetterInfo) bool {
	// Check if all instruments support buffered measurements
	for _, domain := range waveform.AxisDomains {
		for _, labelledDomain := range domain {
			if !p.config.GetBoolProperty(
				labelledDomain.LabelJSON,
				SupportedProperties.SupportsBufferedMeasurements,
				false,
			) {
				return false
			}
		}
	}

	for _, getter := range getters {
		if !p.config.GetBoolProperty(
			getter.PortJSON,
			SupportedProperties.SupportsBufferedMeasurements,
			false,
		) {
			return false
		}
	}

	return true
}

// chunkInstructions breaks the raw time trace into chunks.
// For unbuffered: each row is a separate chunk.
// For buffered: chunks are split at staircase boundaries.
func (p *WaveformProcessor) chunkInstructions(rawTimeTrace [][]float64, buffered bool) [][][]float64 {
	if len(rawTimeTrace) == 0 {
		return nil
	}

	if !buffered {
		// Unbuffered: each row is a chunk of shape [1, n_axes]
		chunks := make([][][]float64, len(rawTimeTrace))
		for i, row := range rawTimeTrace {
			chunks[i] = [][]float64{row}
		}
		return chunks
	}

	// Buffered: find chunk boundaries where primary axis stops staircasing
	primaryAxis := make([]float64, len(rawTimeTrace))
	for i, row := range rawTimeTrace {
		if len(row) > 0 {
			primaryAxis[i] = row[0]
		}
	}

	// Determine dominant polarity
	dominantPolarity := 0.0
	diffSum := 0.0
	for i := 1; i < len(primaryAxis); i++ {
		diff := primaryAxis[i] - primaryAxis[i-1]
		if diff > 0 {
			diffSum += 1
		} else if diff < 0 {
			diffSum -= 1
		}
	}
	if diffSum > 0 {
		dominantPolarity = 1
	} else if diffSum < 0 {
		dominantPolarity = -1
	}

	// Find breaks where polarity changes
	var breaks []int
	for i := 1; i < len(primaryAxis); i++ {
		diff := primaryAxis[i] - primaryAxis[i-1]
		var currentPolarity float64
		if diff > 0 {
			currentPolarity = 1
		} else if diff < 0 {
			currentPolarity = -1
		}
		if currentPolarity != dominantPolarity && currentPolarity != 0 {
			breaks = append(breaks, i)
		}
	}

	// Split into chunks
	chunks := make([][][]float64, 0)
	start := 0
	for _, breakIdx := range breaks {
		if breakIdx > start {
			chunk := make([][]float64, breakIdx-start)
			copy(chunk, rawTimeTrace[start:breakIdx])
			chunks = append(chunks, chunk)
		}
		start = breakIdx
	}
	// Last chunk
	if start < len(rawTimeTrace) {
		chunk := make([][]float64, len(rawTimeTrace)-start)
		copy(chunk, rawTimeTrace[start:])
		chunks = append(chunks, chunk)
	}

	return chunks
}

// collectSampleRates gets sample rates for all getters.
func (p *WaveformProcessor) collectSampleRates(getters []GetterInfo) map[string]int {
	rates := make(map[string]int)
	for _, getter := range getters {
		rate := p.config.GetIntProperty(
			getter.PortJSON,
			SupportedProperties.SampleRate,
			DefaultSampleRate,
		)
		rates[getter.PortJSON] = rate
	}
	return rates
}

// calculateNumSamplesPerStep calculates samples per step for each meter.
func (p *WaveformProcessor) calculateNumSamplesPerStep(stepWidthMs int, sampleRates map[string]int) map[string]int {
	numSamples := make(map[string]int)
	for portJSON, rate := range sampleRates {
		// samples = stepWidth(ms) * rate(samples/sec) / 1000
		numSamples[portJSON] = int(math.Ceil(float64(stepWidthMs) * float64(rate) / 1000))
	}
	return numSamples
}

// buildInstructionFromChunk builds an instruction from a chunk of data.
func (p *WaveformProcessor) buildInstructionFromChunk(
	chunk [][]float64,
	waveform *WaveformData,
	getters []GetterInfo,
	stepWidthMs int,
	numSamples map[string]int,
	buffered bool,
) *Instruction {
	getterJSONs := make([]string, len(getters))
	for i, g := range getters {
		getterJSONs[i] = g.PortJSON
	}

	instruction := NewInstruction(getterJSONs, buffered)
	numXPoints := len(chunk)

	// Add meter requirements
	for _, getter := range getters {
		props := p.generateMeterProperties(
			stepWidthMs,
			numSamples[getter.PortJSON],
			buffered,
			numXPoints,
		)
		// Find the similar port for number_of_samples property
		instruction.AddRequirement(getter.PortJSON, props)
	}

	// Add knob requirements for each axis dimension
	for dimIdx, axisDomains := range waveform.AxisDomains {
		bufferedDimension := buffered && dimIdx == 0

		// Extract the raw space for this dimension from the chunk
		rawSpace := make([]float64, len(chunk))
		for i, row := range chunk {
			if dimIdx < len(row) {
				rawSpace[i] = row[dimIdx]
			}
		}

		for _, labelledDomain := range axisDomains {
			props := p.generateKnobProperties(
				waveform.TimeDomain,
				rawSpace,
				labelledDomain.DomainBounds,
				stepWidthMs,
				bufferedDimension,
				buffered,
				numXPoints,
			)
			instruction.AddRequirement(labelledDomain.LabelJSON, props)
			instruction.AddSetter(labelledDomain.LabelJSON)
		}
	}

	return instruction
}

// generateMeterProperties generates properties for a meter instrument.
func (p *WaveformProcessor) generateMeterProperties(
	stepWidthMs int,
	numSamples int,
	buffered bool,
	numXPoints int,
) map[string]interface{} {
	props := make(map[string]interface{})

	if !buffered {
		props[SupportedProperties.Timeout] = TimeoutScaleFactor * float64(stepWidthMs) / 1000
		props[SupportedProperties.NumberOfSamples] = numSamples
	} else {
		props[SupportedProperties.Timeout] = (TimeoutScaleFactor - 1 + float64(numXPoints)) * float64(stepWidthMs) / 1000
		props[SupportedProperties.NumberOfSamples] = numSamples * numXPoints
	}

	return props
}

// generateKnobProperties generates properties for a knob instrument.
func (p *WaveformProcessor) generateKnobProperties(
	unitDomain DomainBounds,
	rawSpace []float64,
	domain DomainBounds,
	stepWidthMs int,
	bufferedDimension bool,
	buffered bool,
	numXPoints int,
) map[string]interface{} {
	props := make(map[string]interface{})

	if len(rawSpace) == 0 {
		return props
	}

	// Transform the first value using the domain
	vStart := domain.Transform(rawSpace[0])

	if !bufferedDimension && !buffered {
		// Standard unbuffered
		props[SupportedProperties.VoltageState] = vStart
		props[SupportedProperties.Timeout] = TimeoutScaleFactor * float64(stepWidthMs) / 1000
	} else if bufferedDimension {
		// Buffered dimension - create staircase
		vStop := domain.Transform(rawSpace[len(rawSpace)-1])
		props[SupportedProperties.Staircase] = StaircaseConfig{
			StepWidthMs: float64(stepWidthMs),
			NumSteps:    len(rawSpace),
			Offset:      0,
			VStart:      vStart,
			VStop:       vStop,
		}
		props[SupportedProperties.Timeout] = (TimeoutScaleFactor - 1 + float64(numXPoints)) * float64(stepWidthMs) / 1000
	} else {
		// Buffered but not buffered dimension
		props[SupportedProperties.VoltageState] = vStart
		props[SupportedProperties.Timeout] = TimeoutScaleFactor * float64(stepWidthMs) / 1000
	}

	return props
}

// interjectRamps adds ramp instructions between measurement instructions.
func (p *WaveformProcessor) interjectRamps(instructions []*Instruction) []*Instruction {
	if len(instructions) == 0 {
		return instructions
	}

	newInstructions := make([]*Instruction, 0, len(instructions)*2)
	newInstructions = append(newInstructions, instructions[0])

	for i := 1; i < len(instructions); i++ {
		instruction := instructions[i]
		rampInstruction := p.createRampInstruction(instruction)
		if rampInstruction != nil {
			newInstructions = append(newInstructions, rampInstruction)
		}
		newInstructions = append(newInstructions, instruction)
	}

	return newInstructions
}

// createRampInstruction creates a ramp instruction to transition to the next measurement.
func (p *WaveformProcessor) createRampInstruction(nextInstruction *Instruction) *Instruction {
	requirements := make(map[string]map[string]interface{})
	var setters []string

	for portJSON, props := range nextInstruction.Requirements {
		// Only process setters with staircase
		isInSetters := false
		for _, s := range nextInstruction.Setters {
			if s == portJSON {
				isInSetters = true
				break
			}
		}
		if !isInSetters {
			continue
		}

		if staircase, ok := props[SupportedProperties.Staircase]; ok {
			var vStart float64
			switch sc := staircase.(type) {
			case StaircaseConfig:
				vStart = sc.VStart
			case []interface{}:
				if len(sc) >= 4 {
					vStart = toFloat64(sc[3])
				}
			}

			// Get slope from configuration
			slope := p.config.GetFloatProperty(portJSON, SupportedProperties.Slope, DefaultSlope)
			vStop := vStart // Ramp goes to the start of next staircase

			// Calculate timeout based on slope
			timeout := math.Abs(vStop-vStart) / slope

			requirements[portJSON] = map[string]interface{}{
				SupportedProperties.VoltageState: vStart,
				SupportedProperties.Timeout:      TimeoutScaleFactor * timeout,
			}
			setters = append(setters, portJSON)
		}
	}

	if len(requirements) == 0 {
		return nil
	}

	ramp := &Instruction{
		Getters:      nil, // Ramps don't have getters
		Setters:      setters,
		Requirements: requirements,
		Buffered:     true,
	}

	return ramp
}

// ChunkDataAligner helps align collected data chunks.
type ChunkDataAligner struct{}

// DivideToSubChunks divides each chunk of data into sub-chunks.
// In standard measurements, this is a no-op.
func (a *ChunkDataAligner) DivideToSubChunks(
	chunkData map[int64]map[string][]float64,
	numberOfBins int,
) []map[string][]float64 {
	var result []map[string][]float64

	// Process chunks in sorted order
	chunkIDs := make([]int64, 0, len(chunkData))
	for id := range chunkData {
		chunkIDs = append(chunkIDs, id)
	}
	sort.Slice(chunkIDs, func(i, j int) bool { return chunkIDs[i] < chunkIDs[j] })

	for _, chunkID := range chunkIDs {
		collected := chunkData[chunkID]

		// Divide each port's data into numberOfBins divisions
		dividedChunks := make(map[string][][]float64)
		for portJSON, data := range collected {
			dividedChunks[portJSON] = evenDivisions(data, numberOfBins)
		}

		// Unravel the divided chunks
		for i := 0; i < numberOfBins; i++ {
			subChunk := make(map[string][]float64)
			for portJSON, divisions := range dividedChunks {
				if i < len(divisions) {
					subChunk[portJSON] = divisions[i]
				}
			}
			result = append(result, subChunk)
		}
	}

	return result
}

// evenDivisions divides data into n even divisions.
func evenDivisions(data []float64, n int) [][]float64 {
	if n <= 0 || len(data) == 0 {
		return nil
	}

	divisionSize := len(data) / n
	remainder := len(data) % n

	result := make([][]float64, n)
	start := 0

	for i := 0; i < n; i++ {
		size := divisionSize
		if i < remainder {
			size++
		}
		end := start + size
		if end > len(data) {
			end = len(data)
		}
		result[i] = make([]float64, end-start)
		copy(result[i], data[start:end])
		start = end
	}

	return result
}

// CalculateDataPointsPerQueue calculates expected data points per queue.
func CalculateDataPointsPerQueue(shape []int, dataCount int) (int, error) {
	product := 1
	for _, dim := range shape {
		product *= dim
	}

	if dataCount == 0 || product%dataCount != 0 {
		return 0, fmt.Errorf("uneven division: %d / %d", product, dataCount)
	}

	return product / dataCount, nil
}

// AverageSubChunk computes the average of a sub-chunk of data.
func AverageSubChunk(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}
