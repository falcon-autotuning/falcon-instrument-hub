//go:build cgo

package cartesianmeasure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCartesianCanHandle(t *testing.T) {
	tests := []struct {
		name   string
		change func(*MeasurementRequest)
		want   bool
	}{
		{"connected ports", nil, true},
		{"no waveform", func(req *MeasurementRequest) {
			req.Waveforms = nil
		}, false},
		{"unknown getter", func(req *MeasurementRequest) {
			req.Getters[0].DefaultName = "missing"
		}, false},
		{"unknown setter", func(req *MeasurementRequest) {
			req.Waveforms[0].Space.Axes[0][0].InstrumentPort.DefaultName = "missing"
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid1DSweepRequest(t)
			if tt.change != nil {
				tt.change(&req)
			}

			got, err := (&CartesianMeasurementHandler{}).CanHandle(
				req, valid1DConnectedPorts(), "",
			)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAverageData(t *testing.T) {
	got, err := averageData([]float64{2, 4, 10, 14}, 2)
	require.NoError(t, err)
	assert.Equal(t, []float64{3, 12}, got)

	_, err = averageData([]float64{2, 4, 10}, 2)
	require.ErrorContains(t, err, "not divisible")

	_, err = averageData([]float64{2}, 0)
	require.ErrorContains(t, err, "invalid averaging count")
}

type handlerTestRunner struct {
	*registryTestRunner
	scriptPath string
}

func (r *handlerTestRunner) ScriptsPath() string { return r.scriptPath }

func TestCartesianHandleNoAvailableScript(t *testing.T) {
	runner := &handlerTestRunner{
		registryTestRunner: &registryTestRunner{},
		scriptPath:         t.TempDir(),
	}

	_, err := (&CartesianMeasurementHandler{}).Handle(
		valid1DSweepRequest(
			t,
		),
		runner,
		valid1DConnectedPorts(),
		nil,
		nil,
	)
	require.ErrorContains(t, err, "no measurement dimensions requested")
	assert.Empty(t, runner.requests)
}
