// Package serverinterpreter provides lightweight performance telemetry for
// measurement orchestration, database I/O, and script dispatching.
//
// Usage:
//
//	t := NewPerfTelemetry(func(m string) { log.Println(m) })
//
//	stop := t.Start("Execute2DSweep", map[string]string{"id": req.MeasurementID})
//	defer stop()
//
// Every completed span is recorded in an in-memory ring buffer (last N
// entries) and optionally forwarded to a LogFunc for structured logging.
package serverinterpreter

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// DefaultRingSize is the number of recent spans kept in memory.
const DefaultRingSize = 256

// PerfSpan is a single completed timing span.
type PerfSpan struct {
	Operation  string            `json:"operation"`
	Labels     map[string]string `json:"labels,omitempty"`
	StartTime  time.Time         `json:"start_time"`
	Duration   time.Duration     `json:"duration_ns"`
	DurationMs float64           `json:"duration_ms"`
}

// LogFunc is called with a human-readable log line when a span completes.
type LogFunc func(string)

// PerfTelemetry collects timing measurements for key code paths.
type PerfTelemetry struct {
	logFn LogFunc
	mu    sync.Mutex
	spans []PerfSpan
	pos   int // ring position
	full  bool
}

// NewPerfTelemetry creates a new telemetry collector.
// logFn may be nil if you only want to query spans programmatically.
func NewPerfTelemetry(logFn LogFunc) *PerfTelemetry {
	return &PerfTelemetry{
		logFn: logFn,
		spans: make([]PerfSpan, DefaultRingSize),
	}
}

// Start begins a timed span. Call the returned function to stop the timer
// and record the span.
//
//	stop := t.Start("ExecuteAveragedAxisSweep", map[string]string{"gate": "P1"})
//	// ... do work ...
//	stop()
func (pt *PerfTelemetry) Start(operation string, labels map[string]string) func() {
	start := time.Now()
	return func() {
		dur := time.Since(start)
		span := PerfSpan{
			Operation:  operation,
			Labels:     labels,
			StartTime:  start,
			Duration:   dur,
			DurationMs: float64(dur.Nanoseconds()) / 1e6,
		}
		pt.record(span)
	}
}

// record stores a span in the ring buffer and logs it.
func (pt *PerfTelemetry) record(s PerfSpan) {
	pt.mu.Lock()
	pt.spans[pt.pos] = s
	pt.pos = (pt.pos + 1) % len(pt.spans)
	if pt.pos == 0 {
		pt.full = true
	}
	pt.mu.Unlock()

	if pt.logFn != nil {
		msg := fmt.Sprintf("[perf] %s took %.2fms", s.Operation, s.DurationMs)
		if len(s.Labels) > 0 {
			msg += " " + fmtLabels(s.Labels)
		}
		pt.logFn(msg)
	}
}

// RecentSpans returns the most recent spans in chronological order.
func (pt *PerfTelemetry) RecentSpans() []PerfSpan {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if !pt.full {
		out := make([]PerfSpan, pt.pos)
		copy(out, pt.spans[:pt.pos])
		return out
	}

	out := make([]PerfSpan, len(pt.spans))
	copy(out, pt.spans[pt.pos:])
	copy(out[len(pt.spans)-pt.pos:], pt.spans[:pt.pos])
	return out
}

// SpansByOperation returns recent spans filtered by operation name.
func (pt *PerfTelemetry) SpansByOperation(operation string) []PerfSpan {
	all := pt.RecentSpans()
	var filtered []PerfSpan
	for _, s := range all {
		if s.Operation == operation {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// SummaryJSON returns a JSON summary string of per-operation statistics.
func (pt *PerfTelemetry) SummaryJSON() (string, error) {
	all := pt.RecentSpans()
	type opStat struct {
		Count   int     `json:"count"`
		TotalMs float64 `json:"total_ms"`
		AvgMs   float64 `json:"avg_ms"`
		MinMs   float64 `json:"min_ms"`
		MaxMs   float64 `json:"max_ms"`
	}

	stats := map[string]*opStat{}
	for _, s := range all {
		st, ok := stats[s.Operation]
		if !ok {
			st = &opStat{MinMs: s.DurationMs, MaxMs: s.DurationMs}
			stats[s.Operation] = st
		}
		st.Count++
		st.TotalMs += s.DurationMs
		if s.DurationMs < st.MinMs {
			st.MinMs = s.DurationMs
		}
		if s.DurationMs > st.MaxMs {
			st.MaxMs = s.DurationMs
		}
	}
	for _, st := range stats {
		if st.Count > 0 {
			st.AvgMs = st.TotalMs / float64(st.Count)
		}
	}

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func fmtLabels(labels map[string]string) string {
	s := "{"
	first := true
	for k, v := range labels {
		if !first {
			s += ", "
		}
		s += k + "=" + v
		first = false
	}
	return s + "}"
}
