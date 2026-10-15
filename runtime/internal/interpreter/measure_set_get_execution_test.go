//go:build cgo

package interpreter

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrument"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/porttype"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdb"
)

func TestMeasureSetGetSingleVoltageAndReading(t *testing.T) {
	runSetGetCase(t, []float64{0.25}, []int{1}, []float64{0.012})
}

func TestMeasureSetGetP1P2P3AndO2O4(t *testing.T) {
	runSetGetCase(
		t,
		[]float64{0.1, 0.2, 0.3},
		[]int{2, 4},
		[]float64{0.012, 0.034},
	)
}

// These are handler unit tests: ISS is mocked using the new Lua script's
// argument order and call results. They do not execute Lua or contact hardware.
func runSetGetCase(
	t *testing.T,
	voltages []float64,
	channels []int,
	readings []float64,
) {
	t.Helper()
	request := newSetGetMeasurementRequest(t, voltages, channels)
	matches, err := (&measureGetSetHandler{}).CanHandle(request)
	require.NoError(t, err)
	require.True(t, matches)
	sweep, err := (&measure1DBufferedHandler{}).CanHandle(request)
	require.NoError(t, err)
	require.False(t, sweep, "fixed voltages must reach the DC handler")
	connected := &config.ConnectedPorts{}
	server := &setGetTestServer{
		samples: map[string][]float64{},
		owners:  map[string]string{},
	}
	var setters, getters instrumentserver.InstrumentTargetArray
	var expectedReads []instrumentserver.CallResult
	wantState := map[falconcore.ConnectionName]falconcore.Quantity{}
	addPort := func(name, id string, channel int, meter bool) string {
		entry := config.PortEntry{
			InstrumentName: id,
			ChannelGroup:   "analog",
			Channel:        channel,
			InstrumentType: instrument.DcVoltageSource,
			Role:           porttype.PortTypeKnob,
			Access:         access.Write,
			Unit:           falconcore.Volt,
		}
		if meter {
			entry.InstrumentType, entry.Role, entry.Access = instrument.Voltmeter, porttype.PortTypeMeter, access.Read
		}
		port := config.ConnectedPort{
			PortEntry:  entry,
			DeviceName: falconcore.ConnectionName(name),
			PortName: config.PortName(
				fmt.Sprintf("%s.analog%d.voltage", id, channel),
			),
		}
		connected.AllConnections = append(
			connected.AllConnections,
			port,
		)
		if meter {
			connected.Meters = append(connected.Meters, port)
		} else {
			connected.Knobs = append(connected.Knobs, port)
		}
		target, err := (instrumenttarget.Target{Instrument: id, Group: "analog", Channel: channel}).Serialize()
		require.NoError(t, err)
		return target
	}
	call := func(id, verb string, channel int) instrumentserver.CallResult {
		return instrumentserver.CallResult{
			Instrument: id,
			Group:      "analog",
			Channel:    int64(channel),
			Verb:       verb,
		}
	}
	for i, voltage := range voltages {
		name := fmt.Sprintf("P%d", i+1)
		setters = append(setters, addPort(name, "Source1", i+1, false))
		wantState[falconcore.ConnectionName(name)] = falconcore.Quantity{
			Value: voltage,
			Unit:  falconcore.Volt,
		}
		server.results = append(
			server.results,
			call("Source1", "SET_VOLTAGE", i+1),
		)
	}
	for i, channel := range channels {
		name := fmt.Sprintf("O%d", channel)
		getters = append(
			getters,
			addPort(name, "Meter1", channel, true),
		)
		server.results = append(
			server.results,
			call(
				"Meter1",
				"SET_SAMPLE_RATE",
				channel,
			),
			call("Meter1", "SET_BINS", channel),
		)
		reading := call("Meter1", "GET_DATAPOINT", channel)
		reading.Return = []instrumentserver.ReturnValue{
			{
				Name: "voltage",
				Unit: "mV",
				Value: instrumentserver.VariableValue{
					Value: instrumentserver.DataBuffer(
						name,
					),
				},
			},
		}
		server.samples[name] = []float64{
			readings[i] * 1000,
		} // Verify mV-to-V conversion.
		expectedReads = append(expectedReads, reading)
	}
	server.results = append(server.results, expectedReads...)
	variable := func(name string, value any) instrumentserver.MeasureVariable {
		return instrumentserver.MeasureVariable{
			Name:  name,
			Value: instrumentserver.VariableValue{Value: value},
		}
	}
	// One sample in a one-second request gives a nominal sample rate of 1
	// Hz.
	server.wantVariables = []instrumentserver.MeasureVariable{
		variable("getters", getters),
		variable("numPoints", int64(1)),
		variable("setVoltages", instrumentserver.DoubleArray(voltages)),
		variable("setters", setters),
	}
	scripts := filepath.Join(
		"..",
		"..",
		"..",
		"test_data",
		"measurement-scripts",
		"lua-measurement-examples",
	)
	server.wantScript = filepath.Join(scripts, "measure_set_get.lua")
	require.FileExists(t, server.wantScript)
	d := dispatcher.NewMeasurementDispatcher(server, server, scripts)
	state := setGetTestState{}
	setupMeasurementDB(t)
	recorder := &trackedMeasurementRecorder{}

	response, err := (&measureGetSetHandler{}).Handle(
		request,
		d,
		nil,
		connected,
		state,
		recorder,
	)
	require.NoError(t, err)
	require.NotNil(t, response)
	defer response.Close()
	require.Equal(t, 1, server.calls)
	require.Equal(
		t,
		wantState,
		map[falconcore.ConnectionName]falconcore.Quantity(state),
	)
	require.Empty(
		t,
		server.owners,
		"every returned buffer must be consumed or released",
	)
	require.Empty(t, server.samples, "every reading must be consumed")
	require.Positive(
		t,
		recorder.Writes(),
		"store the measurement through the recorder",
	)
	catalog, err := measurementdb.Search(measurementdb.SearchQuery{})
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	require.Len(t, catalog[0].Setters, len(voltages))
	require.Len(t, catalog[0].Getters, len(channels))
	stored, err := measurementdb.NewHDF5Store().Read(catalog[0].FilePath)
	require.NoError(t, err)
	require.Len(t, stored.Ranges, len(readings))
	require.NotEmpty(t, stored.Metadata["request"])
	for i, channel := range channels {
		require.Equal(
			t,
			falconcore.ConnectionName(fmt.Sprintf("O%d", channel)),
			stored.Ranges[i].Connection.Name,
		)
		require.InDeltaSlice(
			t,
			[]float64{readings[i]},
			stored.Ranges[i].Data,
			1e-12,
		)
	}
	arrays, err := response.Handle().Arrays()
	require.NoError(t, err)
	defer arrays.Close()
	count, err := arrays.Size()
	require.NoError(t, err)
	require.EqualValues(t, len(channels), count)
	for i, channel := range channels {
		array, err := arrays.At(uint64(i))
		require.NoError(t, err)
		defer array.Close()
		label, err := array.Connection()
		require.NoError(t, err)
		defer label.Close()
		name, err := label.Name()
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("O%d", channel), name)
		values, err := array.Data()
		require.NoError(t, err)
		require.InDeltaSlice(t, []float64{readings[i]}, values, 1e-12)
	}
}

type setGetTestState map[falconcore.ConnectionName]falconcore.Quantity

func (s setGetTestState) UpdatePort(
	port falconcore.ConnectionName,
	value falconcore.Quantity,
) error {
	s[port] = value
	return nil
}

type setGetTestServer struct {
	wantScript    string
	wantVariables []instrumentserver.MeasureVariable
	results       []instrumentserver.CallResult
	samples       map[string][]float64
	owners        map[string]string
	calls         int
}

func (s *setGetTestServer) Measure(
	path string,
	variables []instrumentserver.MeasureVariable,
) ([]instrumentserver.CallResult, error) {
	s.calls++
	if path != s.wantScript ||
		!reflect.DeepEqual(variables, s.wantVariables) {
		return nil, fmt.Errorf(
			"wrong Lua invocation: got %s with %v; want %s with %v",
			path,
			variables,
			s.wantScript,
			s.wantVariables,
		)
	}
	return s.results, nil
}

func (s *setGetTestServer) RegisterBuffer(owner, id string) error {
	s.owners[id] = owner
	return nil
}

func (s *setGetTestServer) ConsumeSamples(owner, id string) ([]float64, error) {
	if s.owners[id] != owner || s.samples[id] == nil {
		return nil, fmt.Errorf(
			"unexpected buffer %q for measurement %q",
			id,
			owner,
		)
	}
	values := s.samples[id]
	delete(s.owners, id)
	delete(s.samples, id)
	return values, nil
}

func (s *setGetTestServer) ReleaseRequestor(owner string) error {
	for id, actual := range s.owners {
		if actual == owner {
			delete(s.owners, id)
		}
	}
	return nil
}
