//go:build cgo

package interpreter

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/autotuner-interfaces/contexts/acquisitioncontext"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/measurementresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdb"
)

const (
	measureGetSetHandlerName = "measure_set_get"
	measureSetVoltageCommand = "SET_VOLTAGE"
	measureGetCommand        = "GET_DATAPOINT"
	measureSetInput          = "voltage"
	measureGetOutput         = "voltage"
)

// measureGetSetHandler sets fixed source voltages, then reads each voltmeter.
// The Lua script defines the instrument call order; this handler supplies its
// arguments, checks the results, stores the readings, and updates source state.
type measureGetSetHandler struct{}

// Name returns the Lua script name (without the .lua extension).
func (*measureGetSetHandler) Name() string { return measureGetSetHandlerName }

// CanHandle returns true for supported DC requests. Unsupported shapes return
// (false, nil); malformed requests return an error. No hardware is contacted.
func (*measureGetSetHandler) CanHandle(
	req *FalconMeasurementRequest,
) (bool, error) {
	parsed, err := parseMeasureGetSetRequest(req)
	if err != nil || parsed == nil {
		return false, err
	}
	defer parsed.Close()
	return true, nil
}

// resolveSetGetTargets maps requested ports to the native targets passed to
// Lua.
// It requires the voltage capability and rejects repeated physical channels.
// Returned ports and serialized targets have the same order as the input.
func resolveSetGetTargets(
	ports []*instrumentport.Handle,
	connected *config.ConnectedPorts,
) ([]config.ConnectedPort, instrumentserver.InstrumentTargetArray, error) {
	resolved := make([]config.ConnectedPort, 0, len(ports))
	targets := make(instrumentserver.InstrumentTargetArray, 0, len(ports))
	seen := map[string]bool{}
	for _, port := range ports {
		physical, err := connected.ResolveConnectedPort(port)
		if err != nil {
			return nil, nil, err
		}
		if !strings.HasSuffix(string(physical.PortName), ".voltage") {
			return nil, nil, fmt.Errorf(
				"set/get requires voltage capabilities, got %q",
				physical.PortName,
			)
		}
		target, err := (instrumenttarget.Target{
			Instrument: physical.InstrumentName, Group: physical.ChannelGroup, Channel: physical.Channel,
		}).Serialize()
		if err != nil {
			return nil, nil, err
		}
		if seen[target] {
			return nil, nil, fmt.Errorf(
				"duplicate set/get target %q",
				target,
			)
		}
		seen[target] = true
		resolved = append(resolved, physical)
		targets = append(targets, target)
	}
	return resolved, targets, nil
}

// measureGetSetValue reads one GET_DATAPOINT output, either a scalar or a
// single-sample DataBuffer. d consumes buffers owned by measurementID. getter
// supplies the requested units. It returns the converted reading, or an error
// for a missing/invalid sample, incompatible units, or a buffer access failure.
func measureGetSetValue(
	d *dispatcher.MeasurementDispatcher,
	measurementID string,
	measured instrumentserver.ReturnValue,
	getter *instrumentport.Handle,
) (float64, error) {
	if measured.Name != measureGetOutput {
		return 0, fmt.Errorf(
			"datapoint output is %q, want %q",
			measured.Name,
			measureGetOutput,
		)
	}
	var value float64
	switch raw := measured.Value.Value.(type) {
	case float64:
		value = raw
	case int64:
		value = float64(raw)
	case instrumentserver.DataBuffer:
		if raw == "" {
			return 0, fmt.Errorf("datapoint buffer ID is empty")
		}
		samples, err := d.ConsumeMeasurementSamples(
			measurementID,
			string(raw),
		)
		if err != nil {
			return 0, err
		}
		if len(samples) != 1 {
			return 0, fmt.Errorf(
				"datapoint buffer has %d samples, want 1",
				len(samples),
			)
		}
		value = samples[0]
	default:
		return 0, fmt.Errorf("unsupported datapoint type %T", raw)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("datapoint must be finite")
	}
	units, err := SymbolUnitFromString(measured.Unit)
	if err != nil {
		return 0, err
	}
	defer units.Close()
	getterUnits, err := getter.Units()
	if err != nil {
		return 0, err
	}
	defer getterUnits.Close()
	compatible, err := units.IsCompatibleWith(getterUnits)
	if err != nil {
		return 0, err
	}
	if !compatible {
		return 0, fmt.Errorf(
			"datapoint unit %q is incompatible with getter",
			measured.Unit,
		)
	}
	value, err = units.ConvertValueTo(value, getterUnits)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("converted datapoint must be finite")
	}
	return value, nil
}

// Handle executes req using measurementDispatcher and the connected-port
// catalog. tracker records applied source voltages; recorder stores the
// labelled readings. The WireMap parameter is unused because connected already
// resolves the wiring.
//
// Lua receives getters, numPoints, sampleRate, setVoltages, setters, in that
// order. numPoints is 1 for GET_DATAPOINT; sampleRate is ceil(1 / duration in
// seconds), at least 1 Hz. This is a nominal setting, not a guaranteed
// wall-clock duration.
// The response contains one one-element array per getter, in request order and
// getter units. The caller must Close it. Failures return (nil, error) and do
// not
// undo voltages already applied to instruments.
func (*measureGetSetHandler) Handle(
	req *FalconMeasurementRequest,
	measurementDispatcher *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	connected *config.ConnectedPorts,
	tracker StateUpdater,
	recorder MeasurementRecorder,
) (*FalconMeasurementResponse, error) {
	parsed, err := parseMeasureGetSetRequest(req)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, fmt.Errorf(
			"request does not match %s",
			measureGetSetHandlerName,
		)
	}
	defer parsed.Close()
	if measurementDispatcher == nil || connected == nil || tracker == nil ||
		recorder == nil {
		return nil, fmt.Errorf(
			"set/get requires a dispatcher, connected ports, state updater, and recorder",
		)
	}
	setters, setterTargets, err := resolveSetGetTargets(
		parsed.setters,
		connected,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve setters: %w", err)
	}
	getters, getterTargets, err := resolveSetGetTargets(
		parsed.getters,
		connected,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve getters: %w", err)
	}
	sampleRate, err := sweepSampleRate(req, 1)
	if err != nil {
		return nil, err
	}
	requestJSON, err := req.ToJSON()
	if err != nil {
		return nil, err
	}
	variable := func(name string, value any) instrumentserver.MeasureVariable {
		return instrumentserver.MeasureVariable{
			Name:  name,
			Value: instrumentserver.VariableValue{Value: value},
		}
	}
	results := measurementDispatcher.RunAll(
		[]dispatcher.MeasurementRequest{{
			Script: measureGetSetHandlerName,
			Variables: []instrumentserver.MeasureVariable{
				variable(
					"getters",
					getterTargets,
				),
				variable("numPoints", int64(1)),
				variable(
					"sampleRate",
					sampleRate,
				),
				variable(
					"setVoltages",
					instrumentserver.DoubleArray(
						parsed.voltages,
					),
				),
				variable("setters", setterTargets),
			},
		}},
	)
	for _, result := range results {
		defer measurementDispatcher.ReleaseMeasurementBuffers(result.ID)
	}
	if len(results) != 1 {
		return nil, fmt.Errorf(
			"set/get returned %d jobs, want 1",
			len(results),
		)
	}
	result := results[0]
	if result.Err != nil {
		return nil, result.Err
	}
	if len(result.Results) != len(setters)+3*len(getters) {
		return nil, fmt.Errorf(
			"set/get returned %d calls, want %d",
			len(result.Results),
			len(setters)+3*len(getters),
		)
	}
	for i, setter := range setters {
		if err := tracker.UpdatePort(
			setter.DeviceName,
			falconcore.Quantity{
				Value: parsed.voltages[i],
				Unit:  falconcore.Volt,
			},
		); err != nil {
			return nil, fmt.Errorf("update setter state: %w", err)
		}
	}
	ranges := make(falconcore.LabelledMeasuredArrays, 0, len(getters))
	for i, getter := range getters {

		reading := result.Results[len(setters)+2*len(getters)+i]

		value, err := measureGetSetValue(
			measurementDispatcher,
			result.ID,
			reading.Return[0],
			parsed.getters[i],
		)
		if err != nil {
			return nil, fmt.Errorf(
				"read %s: %w",
				getter.DeviceName,
				err,
			)
		}
		context, err := acquisitioncontext.NewFromPort(
			parsed.getters[i],
		)
		if err != nil {
			return nil, err
		}
		label, err := falconcore.AcquisitionContextFromFalcon(context)
		context.Close()
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, falconcore.LabelledMeasuredArray{
			AcquisitionContext: label,
			MeasuredArray: falconcore.MeasuredArray{
				Data:  []float64{value},
				Shape: []int{1},
			},
		})
	}
	arrays, err := ranges.NewFalconLabelledMeasuredArrays()
	if err != nil {
		return nil, err
	}
	defer arrays.Close()
	response, err := measurementresponse.New(arrays)
	if err != nil {
		return nil, err
	}
	if err := storeSetGetMeasurement(
		recorder,
		result.ID,
		requestJSON,
		setters,
		getters,
		parsed.voltages,
		ranges,
	); err != nil {
		response.Close()
		return nil, err
	}
	return &FalconMeasurementResponse{handle: response}, nil
}

// storeSetGetMeasurement saves one DC reading per getter and the applied source
// voltages. There are no swept axes, so the HDF5 domain/shape lists are empty;
// each measured range has shape [1]. The original request is kept as metadata.
// It returns any database error to the caller instead of reporting success.
func storeSetGetMeasurement(
	recorder MeasurementRecorder,
	id, requestJSON string,
	setters, getters []config.ConnectedPort,
	voltages []float64,
	ranges falconcore.LabelledMeasuredArrays,
) error {
	wiremap := func(port config.ConnectedPort) measurementdb.Wiremap {
		return measurementdb.Wiremap{
			Connection: port.DeviceName,
			Instruments: []measurementdb.Instrument{
				{
					Name:         port.InstrumentName,
					ChannelGroup: port.ChannelGroup,
					Channel:      port.Channel,
				},
			},
		}
	}
	insert := measurementdb.InsertEntry{
		CoreEntry: measurementdb.CoreEntry{
			MeasurementID:   id,
			MeasurementName: measureGetSetHandlerName,
		},
		Shape: []int{1},
	}
	for i, setter := range setters {
		voltage := falconcore.Quantity{
			Value: voltages[i],
			Unit:  falconcore.Volt,
		}
		insert.Setters = append(insert.Setters, measurementdb.SweepInfo{
			Wiremap: wiremap(
				setter,
			),
			Axis:   -1,
			Points: 1,
			Start:  voltage,
			End:    voltage,
		})
	}
	for i, getter := range getters {
		insert.Getters = append(
			insert.Getters,
			measurementdb.GetterInfo{
				Wiremap: wiremap(getter),
				Unit:    ranges[i].Unit,
			},
		)
	}
	_, err := recorder.Insert(insert, falconcore.DatabaseEntry{
		Shape:        falconcore.AxesInt{},
		UnitDomain:   falconcore.AxesControlArray{},
		DomainLabels: falconcore.AxesCoupledLabelledDomain{},
		Ranges:       ranges,
		Metadata: falconcore.Metadata{
			"measurement_id": id,
			"request":        requestJSON,
		},
		Title:     measureGetSetHandlerName + "_" + id,
		Timestamp: int(time.Now().Unix()),
	})
	if err != nil {
		return fmt.Errorf("store set/get measurement: %w", err)
	}
	return nil
}
