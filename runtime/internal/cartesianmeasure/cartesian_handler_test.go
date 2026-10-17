//go:build cgo

package cartesianmeasure

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/scope"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
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
	run        func([]DispatchRequest) []DispatchResult
}

func (r *handlerTestRunner) ScriptsPath() string { return r.scriptPath }

func (r *handlerTestRunner) RunAll(requests []DispatchRequest) []DispatchResult {
	r.requests = append(r.requests, requests...)
	if r.run != nil {
		return r.run(requests)
	}
	return r.results
}

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

// Keep instrument execution, settings, and database writes local to each test.
type handlerTestFixture struct {
	handler  *CartesianMeasurementHandler
	runner   *handlerTestRunner
	recorder *handlerTestRecorder
	queries  []string
}

func newHandlerTestFixture(t *testing.T, script string, ports *config.ConnectedPorts, counts ...int) *handlerTestFixture {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, script+".lua"), nil, 0600))
	logger, err := logging.NewLogger(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, logger.Close()) })
	f := &handlerTestFixture{
		runner: &handlerTestRunner{
			registryTestRunner: &registryTestRunner{},
			bufferConsumer:     bufferConsumer{},
			scriptPath:         dir,
		},
		recorder: &handlerTestRecorder{},
	}
	samples := make(map[falconcore.InstrumentPort]falconcore.Quantity)
	for i, count := range counts {
		meter := ports.Meters[i]
		setting := testNumberOfSamplesPort(meter.InstrumentName, meter.ChannelGroup, meter.Channel, scope.Local)
		setting.PortName = config.PortName(fmt.Sprintf("%s.%s.%d.samples", meter.InstrumentName, meter.ChannelGroup, meter.Channel))
		setting.InstrumentType, setting.Unit = meter.InstrumentType, falconcore.Dimensionless
		ports.Settings = append(ports.Settings, setting)
		ports.AllConnections = append(ports.AllConnections, setting)
		samples[InstrumentPortFromConnectedPort(setting)] = falconcore.Quantity{Value: float64(count), Unit: falconcore.Dimensionless}
	}
	f.handler = NewCartesianMeasurementHandler(handlerSettingsReader(func(req falconcore.SettingRequest) (falconcore.SettingResponse, error) {
		f.queries = append(f.queries, req.Message)
		switch req.Message {
		case "capture-number-of-samples":
			require.NotEmpty(t, counts, "Dim0-only execution should not query sample counts")
			require.Empty(t, f.runner.requests)
			assert.Len(t, req.Getters, len(samples))
			for _, port := range req.Getters {
				require.Contains(t, samples, port)
			}
			return falconcore.SettingResponse{Getters: samples}, nil
		case "capture-source-voltages":
			assert.Empty(t, req.Getters)
		default:
			t.Fatalf("unexpected settings query: %q", req.Message)
		}
		return falconcore.SettingResponse{}, nil
	}), logger)
	return f
}

func handlerTestReading(channel int64, value any) instrumentserver.CallResult {
	return instrumentserver.CallResult{
		Instrument: "ADC", Group: "analog", Channel: channel, Verb: "ACQUIRE",
		Return: []instrumentserver.ReturnValue{{
			Name: "samples", Unit: "A", Value: instrumentserver.VariableValue{Value: value},
		}},
	}
}

func handlerTest2DRequest(t *testing.T) MeasurementRequest {
	t.Helper()
	req := valid2DSweepRequest(t)
	space := &req.Waveforms[0].Space
	// Two positions on axis 0, three on axis 1, with one setter per axis.
	space.Space = validCompiledUnitSpace(t, []falconcore.Discretizer{
		{Type: falconcore.CartesianDiscretization, Delta: 1},
		{Type: falconcore.CartesianDiscretization, Delta: 0.5},
	}, falconcore.Domain{LesserBound: 0, GreaterBound: 1, LesserBoundContained: true, GreaterBoundContained: true})
	space.Axes[0] = space.Axes[0][:1]
	for i := range space.Axes {
		space.Axes[i][0].Domain = falconcore.Domain{LesserBound: 0, GreaterBound: 1}
	}
	return req
}

func assertHandler2DOutput(t *testing.T, f *handlerTestFixture, response MeasurementResponse) {
	t.Helper()
	assert.Equal(t, 1, f.recorder.Writes())
	assert.Equal(t, []int{2, 3}, f.recorder.insert.Shape)
	require.Len(t, response.Arrays, 1)
	assert.Equal(t, []float64{0, 1, 2, 10, 11, 12}, response.Arrays[0].Data)
	assert.Equal(t, []int{2, 3}, response.Arrays[0].Shape)
	assert.Equal(t, falconcore.ConnectionName("O1"), response.Arrays[0].Connection.Name)
	assert.Equal(t, response.Arrays, f.recorder.entry.Ranges)
}

func TestCartesianHandle2DUsingDim1(t *testing.T) {
	ports := valid2DConnectedPorts()
	f := newHandlerTestFixture(t, "measure_1D_buffered", ports, 1)
	f.runner.bufferConsumer = bufferConsumer{"row-0": {0, 1, 2}, "row-1": {10, 11, 12}}
	f.runner.run = func(requests []DispatchRequest) []DispatchResult {
		require.Len(t, requests, 1)
		buffer := fmt.Sprintf("row-%d", len(f.runner.requests)-1)
		return []DispatchResult{{ID: "measurement-1", Results: []instrumentserver.CallResult{
			handlerTestReading(0, instrumentserver.DataBuffer(buffer)),
		}}}
	}
	response, err := f.handler.Handle(handlerTest2DRequest(t), f.runner, ports, nil, f.recorder)
	require.NoError(t, err)
	require.Len(t, f.runner.requests, 2)
	for i, request := range f.runner.requests {
		assert.Equal(t, "measure_1D_buffered", request.Script)
		vars := variablesByName(request.Variables)
		assert.Equal(t, int64(3), vars["numSteps"])
		assert.Equal(t, instrumentserver.InstrumentTargetArray{"DAC1|analog|1", "DAC3|analog|3"}, vars["setters"])
		fixed, err := (instrumentdomain.Domain{Min: float64(i), Max: float64(i)}).Serialize()
		require.NoError(t, err)
		swept, err := (instrumentdomain.Domain{Min: 0, Max: 1}).Serialize()
		require.NoError(t, err)
		assert.Equal(t, instrumentserver.InstrumentDomainArray{fixed, swept}, vars["setVoltageDomains"])
	}
	assert.Equal(t, []string{"capture-number-of-samples", "capture-source-voltages"}, f.queries)
	assertHandler2DOutput(t, f, response)
}

func TestCartesianHandle2DUsingDim0(t *testing.T) {
	ports := valid2DConnectedPorts()
	f := newHandlerTestFixture(t, "measure_get_set", ports)
	f.runner.run = func(requests []DispatchRequest) []DispatchResult {
		require.Len(t, requests, 1)
		i := len(f.runner.requests) - 1
		return []DispatchResult{{ID: "measurement-1", Results: []instrumentserver.CallResult{
			handlerTestReading(0, float64((i/3)*10+i%3)),
		}}}
	}
	response, err := f.handler.Handle(handlerTest2DRequest(t), f.runner, ports, nil, f.recorder)
	require.NoError(t, err)
	require.Len(t, f.runner.requests, 6)
	for i, request := range f.runner.requests {
		assert.Equal(t, "measure_get_set", request.Script)
		vars := variablesByName(request.Variables)
		assert.Equal(t, instrumentserver.InstrumentTargetArray{"DAC1|analog|1", "DAC3|analog|3"}, vars["setters"])
		assert.Equal(t, instrumentserver.DoubleArray{float64(i / 3), float64(i%3) / 2}, vars["setVoltages"])
	}
	assert.Equal(t, []string{"capture-source-voltages"}, f.queries)
	assertHandler2DOutput(t, f, response)
}

func TestCartesianHandleDifferentGetterSampleCounts(t *testing.T) {
	req, ports := valid1DSweepRequest(t), valid1DConnectedPorts()
	second := ports.Meters[0]
	second.Channel, second.PortName, second.DeviceName = 1, "second-meter", "O2"
	second.Handle.Name, second.Unit = "O2", falconcore.Nanoampere
	ports.Meters = append(ports.Meters, second)
	ports.AllConnections = append(ports.AllConnections, second)
	req.Getters = append(req.Getters, InstrumentPortFromConnectedPort(second))
	f := newHandlerTestFixture(t, "measure_1D_buffered", ports, 2, 3)
	f.runner.bufferConsumer = bufferConsumer{"o1": {2, 4, 10, 14}, "o2": {1, 2, 3, 4, 5, 6}}
	secondReading := handlerTestReading(1, instrumentserver.DataBuffer("o2"))
	secondReading.Return[0].Unit = "nA"
	// Return the getters in reverse order to ensure matching uses the target.
	f.runner.results = []DispatchResult{{ID: "measurement-1", Results: []instrumentserver.CallResult{
		secondReading, handlerTestReading(0, instrumentserver.DataBuffer("o1")),
	}}}
	response, err := f.handler.Handle(req, f.runner, ports, nil, f.recorder)
	require.NoError(t, err)
	require.Len(t, response.Arrays, 2)
	arrays := make(map[falconcore.ConnectionName]falconcore.LabelledMeasuredArray)
	for _, array := range response.Arrays {
		arrays[array.Connection.Name] = array
		assert.Equal(t, []int{2}, array.Shape)
	}
	assert.Equal(t, []float64{3, 12}, arrays["O1"].Data)
	assert.Equal(t, []float64{2, 5}, arrays["O2"].Data)
	assert.Equal(t, falconcore.Ampere, arrays["O1"].Unit)
	assert.Equal(t, falconcore.Nanoampere, arrays["O2"].Unit)
	assert.Equal(t, 1, f.recorder.Writes())
	assert.Equal(t, response.Arrays, f.recorder.entry.Ranges)
}

func TestCartesianHandleStopsAfterPrimitiveFailure(t *testing.T) {
	ports := valid2DConnectedPorts()
	f := newHandlerTestFixture(t, "measure_get_set", ports)
	failure := errors.New("instrument acquisition failed")
	f.runner.run = func(requests []DispatchRequest) []DispatchResult {
		if len(f.runner.requests) == 2 {
			return []DispatchResult{{ID: "measurement-1", Err: failure}}
		}
		return []DispatchResult{{ID: "measurement-1", Results: []instrumentserver.CallResult{handlerTestReading(0, 1.0)}}}
	}
	response, err := f.handler.Handle(handlerTest2DRequest(t), f.runner, ports, nil, f.recorder)
	require.ErrorIs(t, err, failure)
	assert.Len(t, f.runner.requests, 2)
	assert.Len(t, f.runner.released, 2)
	assert.Empty(t, f.queries)
	assert.Empty(t, response.Arrays)
	assert.Zero(t, f.recorder.Writes())
}

func TestCartesianHandleSettingsFailurePreventsDispatch(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "query error"
		if missing {
			name = "missing getter count"
		}
		t.Run(name, func(t *testing.T) {
			ports := valid1DConnectedPorts()
			f := newHandlerTestFixture(t, "measure_1D_buffered", ports, 2)
			failure := errors.New("settings unavailable")
			f.handler.settings = handlerSettingsReader(func(req falconcore.SettingRequest) (falconcore.SettingResponse, error) {
				assert.Equal(t, "capture-number-of-samples", req.Message)
				if missing {
					return falconcore.SettingResponse{}, nil
				}
				return falconcore.SettingResponse{}, failure
			})
			response, err := f.handler.Handle(valid1DSweepRequest(t), f.runner, ports, nil, f.recorder)
			if missing {
				require.ErrorContains(t, err, "missing sample count")
			} else {
				require.ErrorIs(t, err, failure)
			}
			assert.Empty(t, f.runner.requests)
			assert.Empty(t, response.Arrays)
			assert.Zero(t, f.recorder.Writes())
		})
	}
}

func TestCartesianHandleRejectsWrongBufferLength(t *testing.T) {
	ports := valid1DConnectedPorts()
	f := newHandlerTestFixture(t, "measure_1D_buffered", ports, 2)
	f.runner.bufferConsumer["short"] = []float64{1, 2, 3} // Four samples required.
	f.runner.results = []DispatchResult{{ID: "measurement-1", Results: []instrumentserver.CallResult{
		handlerTestReading(0, instrumentserver.DataBuffer("short")),
	}}}
	response, err := f.handler.Handle(valid1DSweepRequest(t), f.runner, ports, nil, f.recorder)
	require.ErrorContains(t, err, "returned 3 samples, shape [2 2] requires 4")
	assert.Len(t, f.runner.requests, 1)
	assert.Equal(t, []string{"measurement-1"}, f.runner.released)
	assert.Equal(t, []string{"capture-number-of-samples"}, f.queries)
	assert.Empty(t, response.Arrays)
	assert.Zero(t, f.recorder.Writes())
}
