// Package serverinterpreter provides data types for waveform and measurement processing.
package serverinterpreter

// WaveformData represents the extracted data from a falcon-core waveform.
type WaveformData struct {
	// RawTimeTrace is the compiled discrete space data [n_points, n_axes]
	RawTimeTrace [][]float64

	// AxisDomains contains the domain information for each axis
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
