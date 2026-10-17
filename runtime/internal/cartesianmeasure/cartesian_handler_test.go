//go:build cgo

package cartesianmeasure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentdomain"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdb"
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
	bufferConsumer
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

type handlerSettingsReader func(falconcore.SettingRequest) (falconcore.SettingResponse, error)

func (f handlerSettingsReader) Handle(req falconcore.SettingRequest) (falconcore.SettingResponse, error) {
	return f(req)
}

type handlerTestRecorder struct {
	insert measurementdb.InsertEntry
	entry  measurementdb.DatabaseEntry
	writes int
}

func (r *handlerTestRecorder) Insert(insert measurementdb.InsertEntry, entry measurementdb.DatabaseEntry) error {
	r.insert, r.entry = insert, entry
	r.writes++
	return nil
}

func (r *handlerTestRecorder) Writes() int { return r.writes }

func TestCartesianHandleEncodesSweepAndDecodesReadings(t *testing.T) {
	scriptDir := t.TempDir()
	// Discovery needs the filename; the fake runner supplies the acquisition result.
	require.NoError(t, os.WriteFile(
		filepath.Join(scriptDir, "measure_1D_buffered.lua"), nil, 0600,
	))
	logger, err := logging.NewLogger(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, logger.Close()) })

	ports := valid1DConnectedPorts()
	sampleSetting := testNumberOfSamplesPort("ADC", "analog", 0, scope.Local)
	sampleSetting.InstrumentType = ports.Meters[0].InstrumentType
	sampleSetting.Unit = falconcore.Dimensionless
	ports.Settings = append(ports.Settings, sampleSetting)
	ports.AllConnections = append(ports.AllConnections, sampleSetting)
	samplePort := InstrumentPortFromConnectedPort(sampleSetting)

	runner := &handlerTestRunner{
		registryTestRunner: &registryTestRunner{
			results: []DispatchResult{{
				ID: "measurement-1",
				Results: []instrumentserver.CallResult{{
					Instrument: "ADC", Group: "analog", Channel: 0, Verb: "ACQUIRE",
					Return: []instrumentserver.ReturnValue{{
						Name: "samples", Unit: "A",
						Value: instrumentserver.VariableValue{
							Value: instrumentserver.DataBuffer("meter-buffer"),
						},
					}},
				}},
			}},
		},
		// Two samples at each of the fixture's two sweep positions.
		bufferConsumer: bufferConsumer{"meter-buffer": {2, 4, 10, 14}},
		scriptPath:     scriptDir,
	}
	var settingsQueries []string
	settings := handlerSettingsReader(func(req falconcore.SettingRequest) (falconcore.SettingResponse, error) {
		settingsQueries = append(settingsQueries, req.Message)
		switch req.Message {
		case "capture-number-of-samples":
			require.Empty(t, runner.requests)
			assert.Equal(t, falconcore.Ports{samplePort}, req.Getters)
			return falconcore.SettingResponse{
				Getters: map[falconcore.InstrumentPort]falconcore.Quantity{
					samplePort: {Value: 2, Unit: falconcore.Dimensionless},
				},
			}, nil
		case "capture-source-voltages":
			require.Len(t, runner.requests, 1)
			assert.Empty(t, req.Getters)
		default:
			t.Fatalf("unexpected settings query: %q", req.Message)
		}
		return falconcore.SettingResponse{}, nil
	})
	recorder := &handlerTestRecorder{}
	handler := NewCartesianMeasurementHandler(settings, logger)

	response, err := handler.Handle(valid1DSweepRequest(t), runner, ports, nil, recorder)
	require.NoError(t, err)
	require.Len(t, runner.requests, 1)
	assert.Equal(t, "measure_1D_buffered", runner.requests[0].Script)
	variables := variablesByName(runner.requests[0].Variables)
	assert.Equal(t, int64(2), variables["numSteps"])
	assert.Equal(t, instrumentserver.InstrumentTargetArray{"ADC|analog|0"}, variables["getters"])
	assert.Equal(t, instrumentserver.InstrumentTargetArray{"DAC1|analog|1"}, variables["setters"])
	expectedDomain, err := (instrumentdomain.Domain{Min: -1, Max: 1}).Serialize()
	require.NoError(t, err)
	assert.Equal(t, instrumentserver.InstrumentDomainArray{expectedDomain}, variables["setVoltageDomains"])

	require.Len(t, response.Arrays, 1)
	assert.Equal(t, []float64{3, 12}, response.Arrays[0].Data)
	assert.Equal(t, []int{2}, response.Arrays[0].Shape)
	assert.Equal(t, falconcore.ConnectionName("O1"), response.Arrays[0].Connection.Name)
	assert.Equal(t, falconcore.Ampere, response.Arrays[0].Unit)
	assert.Equal(t, 1, recorder.Writes())
	assert.Equal(t, response.Arrays, recorder.entry.Ranges)
	assert.Equal(t, []int{2}, recorder.insert.Shape)
	assert.Equal(t, []string{"measurement-1"}, runner.released)
	assert.Equal(t, []string{"capture-number-of-samples", "capture-source-voltages"}, settingsQueries)
}
