// Package serverinterpreter provides the SweepAxis abstraction for
// generalised 1-D sweeps along arbitrary directions in voltage space.
//
// A SweepAxis describes a parametric line through an N-dimensional voltage
// space.  At parameter t ∈ [0, 1] the voltage vector is:
//
//	V(t) = V_start + t · (V_stop − V_start)
//
// Special cases:
//
//   - Single-gate sweep: one GateEndpoint, e.g. sweep P1 from −1 V to 0 V.
//   - Diagonal sweep:    two (or more) GateEndpoints that move together.
//     Example: sweep [P1, P2] from [−1, −0.5] to [0, 0.5].
//
// The SweepAxis is independent of the measurement type – it works for plain
// 1D sweeps, N-averaged sweeps, and the fast axis of 2D sweeps.
package serverinterpreter

import (
	"fmt"
	"math"
)

// GateEndpoint defines the start and stop voltage for one gate along a sweep axis.
type GateEndpoint struct {
	// Gate is the logical gate name (e.g. "P1", "B1")
	Gate string `json:"gate"`

	// Instrument is the DAC instrument ID (e.g. "QDAC1")
	Instrument string `json:"instrument"`

	// Channel is the DAC channel number
	Channel int `json:"channel"`

	// StartV is the voltage at the axis origin (t = 0)
	StartV float64 `json:"start_v"`

	// StopV is the voltage at the axis end (t = 1)
	StopV float64 `json:"stop_v"`
}

// Span returns |StopV − StartV|.
func (g GateEndpoint) Span() float64 {
	return math.Abs(g.StopV - g.StartV)
}

// VoltageAt returns the gate voltage at normalised position t ∈ [0,1].
func (g GateEndpoint) VoltageAt(t float64) float64 {
	return g.StartV + t*(g.StopV-g.StartV)
}

// SweepAxis describes a parametric line through N-dimensional voltage space.
//
// Examples:
//
//	Single-gate sweep:
//		SweepAxis{
//			Label: "P1 sweep",
//			Gates: []GateEndpoint{{Gate: "P1", Instrument: "QDAC1", Channel: 1, StartV: -1, StopV: 0}},
//			NumPoints: 101,
//		}
//
//	Diagonal sweep (P1 and P2 move together):
//		SweepAxis{
//			Label: "Detuning axis",
//			Gates: []GateEndpoint{
//				{Gate: "P1", Instrument: "QDAC1", Channel: 1, StartV: -0.5, StopV: 0.5},
//				{Gate: "P2", Instrument: "QDAC1", Channel: 2, StartV:  0.5, StopV: -0.5},
//			},
//			NumPoints: 101,
//		}
type SweepAxis struct {
	// Label is a human-readable name for the axis (e.g. "P1 sweep", "detuning axis")
	Label string `json:"label"`

	// Gates is the ordered set of gate endpoints.
	// Len == 1 ⇒ single-gate sweep.
	// Len >  1 ⇒ multi-gate (diagonal / virtual) sweep.
	Gates []GateEndpoint `json:"gates"`

	// NumPoints is the number of equally-spaced sample points along the axis.
	NumPoints int `json:"num_points"`
}

// Dimension returns the number of gates involved (1 = scalar, >1 = vector).
func (a SweepAxis) Dimension() int {
	return len(a.Gates)
}

// IsScalar returns true for a single-gate sweep.
func (a SweepAxis) IsScalar() bool {
	return len(a.Gates) == 1
}

// PrimaryGate returns the first gate (convenience for scalar sweeps).
func (a SweepAxis) PrimaryGate() GateEndpoint {
	return a.Gates[0]
}

// VoltagesAt returns the voltage vector at normalised parameter t ∈ [0,1].
func (a SweepAxis) VoltagesAt(t float64) map[string]float64 {
	v := make(map[string]float64, len(a.Gates))
	for _, g := range a.Gates {
		v[g.Gate] = g.VoltageAt(t)
	}
	return v
}

// ParameterValues returns the NumPoints equally-spaced values of t ∈ [0,1].
func (a SweepAxis) ParameterValues() []float64 {
	if a.NumPoints <= 1 {
		return []float64{0}
	}
	ts := make([]float64, a.NumPoints)
	for i := 0; i < a.NumPoints; i++ {
		ts[i] = float64(i) / float64(a.NumPoints-1)
	}
	return ts
}

// PrimaryVoltages returns the voltage array for the Primary (first) gate.
// This is the natural x-axis for plotting scalar sweeps.
func (a SweepAxis) PrimaryVoltages() []float64 {
	g := a.PrimaryGate()
	vs := make([]float64, a.NumPoints)
	for i, t := range a.ParameterValues() {
		vs[i] = g.VoltageAt(t)
	}
	return vs
}

// AllVoltageVectors returns [numPoints][numGates] – the full voltage table.
// Useful for sending multi-gate set-voltage commands.
func (a SweepAxis) AllVoltageVectors() []map[string]float64 {
	table := make([]map[string]float64, a.NumPoints)
	for i, t := range a.ParameterValues() {
		table[i] = a.VoltagesAt(t)
	}
	return table
}

// Validate checks the axis configuration for obvious errors.
func (a SweepAxis) Validate() error {
	if len(a.Gates) == 0 {
		return fmt.Errorf("sweep axis %q has no gates", a.Label)
	}
	if a.NumPoints < 2 {
		return fmt.Errorf("sweep axis %q needs at least 2 points, got %d", a.Label, a.NumPoints)
	}
	seen := make(map[string]bool)
	for _, g := range a.Gates {
		if g.Gate == "" {
			return fmt.Errorf("sweep axis %q: gate name is empty", a.Label)
		}
		if g.Instrument == "" {
			return fmt.Errorf("sweep axis %q: instrument for gate %s is empty", a.Label, g.Gate)
		}
		if seen[g.Gate] {
			return fmt.Errorf("sweep axis %q: duplicate gate %s", a.Label, g.Gate)
		}
		seen[g.Gate] = true
	}
	return nil
}

// ScalarAxis is a convenience constructor for a single-gate sweep.
//
//	axis := ScalarAxis("P1", "QDAC1", 1, -1.0, 0.0, 101)
func ScalarAxis(gate, instrument string, channel int, startV, stopV float64, numPoints int) SweepAxis {
	return SweepAxis{
		Label: gate + " sweep",
		Gates: []GateEndpoint{
			{Gate: gate, Instrument: instrument, Channel: channel, StartV: startV, StopV: stopV},
		},
		NumPoints: numPoints,
	}
}

// DetuningAxis is a convenience constructor for a 2-gate opposing sweep
// (common in double quantum dot experiments).
//
//	axis := DetuningAxis("P1", "QDAC1", 1, -0.5, 0.5, "P2", "QDAC1", 2, 0.5, -0.5, 101)
func DetuningAxis(
	gate1, inst1 string, ch1 int, start1, stop1 float64,
	gate2, inst2 string, ch2 int, start2, stop2 float64,
	numPoints int,
) SweepAxis {
	return SweepAxis{
		Label: fmt.Sprintf("%s/%s detuning", gate1, gate2),
		Gates: []GateEndpoint{
			{Gate: gate1, Instrument: inst1, Channel: ch1, StartV: start1, StopV: stop1},
			{Gate: gate2, Instrument: inst2, Channel: ch2, StartV: start2, StopV: stop2},
		},
		NumPoints: numPoints,
	}
}
