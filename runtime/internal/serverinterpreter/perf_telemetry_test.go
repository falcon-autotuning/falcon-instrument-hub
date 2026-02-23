package serverinterpreter

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerfTelemetry_StartStop(t *testing.T) {
	var logged []string
	pt := NewPerfTelemetry(func(msg string) {
		logged = append(logged, msg)
	})

	stop := pt.Start("TestOp", map[string]string{"key": "val"})
	time.Sleep(1 * time.Millisecond)
	stop()

	assert.Len(t, logged, 1)
	assert.Contains(t, logged[0], "[perf] TestOp")
	assert.Contains(t, logged[0], "key=val")

	spans := pt.RecentSpans()
	assert.Len(t, spans, 1)
	assert.Equal(t, "TestOp", spans[0].Operation)
	assert.Greater(t, spans[0].DurationMs, 0.0)
}

func TestPerfTelemetry_RingBuffer(t *testing.T) {
	pt := NewPerfTelemetry(nil)

	// Fill more than DefaultRingSize
	for i := 0; i < DefaultRingSize+10; i++ {
		stop := pt.Start("op", nil)
		stop()
	}

	spans := pt.RecentSpans()
	// Ring keeps at most DefaultRingSize entries
	assert.Len(t, spans, DefaultRingSize)
}

func TestPerfTelemetry_SpansByOperation(t *testing.T) {
	pt := NewPerfTelemetry(nil)

	for i := 0; i < 5; i++ {
		stop := pt.Start("A", nil)
		stop()
	}
	for i := 0; i < 3; i++ {
		stop := pt.Start("B", nil)
		stop()
	}

	assert.Len(t, pt.SpansByOperation("A"), 5)
	assert.Len(t, pt.SpansByOperation("B"), 3)
	assert.Len(t, pt.SpansByOperation("C"), 0)
}

func TestPerfTelemetry_SummaryJSON(t *testing.T) {
	pt := NewPerfTelemetry(nil)

	for i := 0; i < 3; i++ {
		stop := pt.Start("sweep", nil)
		time.Sleep(1 * time.Millisecond)
		stop()
	}

	summary, err := pt.SummaryJSON()
	require.NoError(t, err)
	assert.Contains(t, summary, `"sweep"`)
	assert.Contains(t, summary, `"count": 3`)

	// Sanity: avg_ms should be positive
	assert.True(t, strings.Contains(summary, "avg_ms"))
}

func TestPerfTelemetry_NilLog(t *testing.T) {
	pt := NewPerfTelemetry(nil)
	stop := pt.Start("noop", nil)
	stop()

	assert.Len(t, pt.RecentSpans(), 1)
}
