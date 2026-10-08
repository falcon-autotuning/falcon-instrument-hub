//go:build cgo

package interpreter

import (
	"fmt"
	"math"
	"strings"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/interpreter/measurementresult"
)

const measure1DBufferedName = "measure_1D_buffered"

type measure1DBufferedHandler struct{}

func (*measure1DBufferedHandler) Name() string { return measure1DBufferedName }

func (*measure1DBufferedHandler) CanHandle(
	req *FalconMeasurementRequest,
) (bool, error) {
	parsed, err := parseMeasureGetSetRequest(req)
	if err != nil || parsed == nil {
		return false, err
	}
	defer parsed.Close()
	return len(parsed.voltages) > 1, nil
}

func sweepSampleRate(req *FalconMeasurementRequest, count int) (int64, error) {
	timeDomain, err := req.Handle().TimeDomain()
	if err != nil {
		return 0, fmt.Errorf("read sweep time domain: %w", err)
	}
	defer timeDomain.Close()
	interval, err := timeDomain.Domain()
	if err != nil {
		return 0, fmt.Errorf("read sweep time interval: %w", err)
	}
	defer interval.Close()
	duration, err := interval.Range()
	if err != nil {
		return 0, fmt.Errorf("read sweep duration: %w", err)
	}
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, fmt.Errorf(
			"sweep duration must be finite and positive",
		)
	}
	rate := math.Ceil(float64(count) / duration)
	if rate < 1 {
		rate = 1
	}
	if math.IsInf(rate, 0) || rate > float64(1<<31-1) {
		return 0, fmt.Errorf("sweep sample rate is out of range")
	}
	return int64(rate), nil
}

func sweepResultValue(
	d *dispatcher.MeasurementDispatcher,
	measurementID string,
	returned instrumentserver.ReturnValue,
	getter config.ConnectedPort,
	getterPort *instrumentport.Handle,
	connected *config.ConnectedPorts,
) (float64, error) {
	if err := measurementresult.Validate(
		returned.Value,
		getter,
		connected,
	); err != nil {
		return 0, err
	}
	if returned.Name != "stream" {
		return 0, fmt.Errorf(
			"stream output is %q, want stream",
			returned.Name,
		)
	}
	bufferID, ok := returned.Value.Value.(instrumentserver.DataBuffer)
	if !ok || bufferID == "" {
		return 0, fmt.Errorf("stream output is not a DataBuffer")
	}
	buffer, err := d.ConsumeMeasurementBuffer(
		measurementID,
		string(bufferID),
	)
	if err != nil {
		return 0, fmt.Errorf("consume stream buffer: %w", err)
	}
	if buffer.Metadata.InstrumentName != getter.InstrumentName {
		return 0, fmt.Errorf(
			"stream buffer belongs to instrument %q, want %q",
			buffer.Metadata.InstrumentName,
			getter.InstrumentName,
		)
	}
	if len(buffer.Values) != 1 {
		return 0, fmt.Errorf(
			"stream buffer contains %d samples, want 1",
			len(buffer.Values),
		)
	}
	value := buffer.Values[0]
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("stream sample is not finite")
	}

	resultUnits, err := SymbolUnitFromString(returned.Unit)
	if err != nil {
		return 0, fmt.Errorf(
			"read stream unit %q: %w",
			returned.Unit,
			err,
		)
	}
	defer resultUnits.Close()
	getterUnits, err := getterPort.Units()
	if err != nil {
		return 0, fmt.Errorf("read getter units: %w", err)
	}
	defer getterUnits.Close()
	compatible, err := resultUnits.IsCompatibleWith(getterUnits)
	if err != nil {
		return 0, fmt.Errorf("compare stream and getter units: %w", err)
	}
	if !compatible {
		return 0, fmt.Errorf(
			"stream unit %q is incompatible with getter",
			returned.Unit,
		)
	}
	converted, err := resultUnits.ConvertValueTo(value, getterUnits)
	if err != nil {
		return 0, fmt.Errorf(
			"convert stream sample to getter units: %w",
			err,
		)
	}
	if math.IsNaN(converted) || math.IsInf(converted, 0) {
		return 0, fmt.Errorf("converted stream sample is not finite")
	}
	return converted, nil
}

func (*measure1DBufferedHandler) Handle(
	req *FalconMeasurementRequest,
	measurementDispatcher *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	connected *config.ConnectedPorts,
) (*FalconMeasurementResponse, error) {
	parsed, err := parseMeasureGetSetRequest(req)
	if err != nil {
		return nil, err
	}
	if parsed == nil || len(parsed.voltages) < 2 {
		return nil, fmt.Errorf(
			"request does not match %s",
			measure1DBufferedName,
		)
	}
	defer parsed.Close()
	if measurementDispatcher == nil || connected == nil {
		return nil, fmt.Errorf(
			"sweep dispatcher or connected ports are nil",
		)
	}

	setter, err := connected.ResolveConnectedPort(parsed.setter)
	if err != nil {
		return nil, fmt.Errorf("resolve sweep setter: %w", err)
	}
	if !strings.HasSuffix(string(setter.PortName), ".voltage") {
		return nil, fmt.Errorf(
			"sweep requires a voltage setter, resolved %q",
			setter.PortName,
		)
	}
	getter, err := connected.ResolveConnectedPort(parsed.getter)
	if err != nil {
		return nil, fmt.Errorf("resolve sweep getter: %w", err)
	}
	if !strings.HasSuffix(string(getter.PortName), ".stream") {
		return nil, fmt.Errorf(
			"sweep requires a stream getter, resolved %q",
			getter.PortName,
		)
	}
	setterTarget, err := (instrumenttarget.Target{
		Instrument: setter.InstrumentName, Group: setter.ChannelGroup, Channel: setter.Channel,
	}).Serialize()
	if err != nil {
		return nil, fmt.Errorf("serialize sweep setter: %w", err)
	}
	getterTarget, err := (instrumenttarget.Target{
		Instrument: getter.InstrumentName, Group: getter.ChannelGroup, Channel: getter.Channel,
	}).Serialize()
	if err != nil {
		return nil, fmt.Errorf("serialize sweep getter: %w", err)
	}
	sampleRate, err := sweepSampleRate(req, len(parsed.voltages))
	if err != nil {
		return nil, err
	}

	results := measurementDispatcher.RunAll(
		[]dispatcher.MeasurementRequest{{
			Script: measure1DBufferedName,
			Variables: []instrumentserver.MeasureVariable{
				{
					Name: "setter",
					Value: instrumentserver.VariableValue{
						Value: instrumentserver.InstrumentTarget(
							setterTarget,
						),
					},
				},
				{
					Name: "getter",
					Value: instrumentserver.VariableValue{
						Value: instrumentserver.InstrumentTarget(
							getterTarget,
						),
					},
				},
				{
					Name: "voltages",
					Value: instrumentserver.VariableValue{
						Value: instrumentserver.DoubleArray(
							parsed.voltages,
						),
					},
				},
				{
					Name: "sampleRate",
					Value: instrumentserver.VariableValue{
						Value: sampleRate,
					},
				},
			},
		}},
	)
	if len(results) != 1 {
		return nil, fmt.Errorf(
			"sweep returned %d measurement results, want 1",
			len(results),
		)
	}
	result := results[0]
	defer measurementDispatcher.ReleaseMeasurementBuffers(result.ID)
	if result.Err != nil {
		return nil, result.Err
	}
	if len(result.Results) != 2+2*len(parsed.voltages) {
		return nil, fmt.Errorf(
			"sweep returned %d instrument calls, want %d",
			len(result.Results),
			2+2*len(parsed.voltages),
		)
	}
	if err := validateMeasureGetSetCall(
		result.Results[0],
		getter,
		"SET_SAMPLE_RATE",
		0,
	); err != nil {
		return nil, err
	}
	if err := validateMeasureGetSetCall(
		result.Results[1],
		getter,
		"SET_BINS",
		0,
	); err != nil {
		return nil, err
	}
	values := make([]float64, len(parsed.voltages))
	for i := range values {
		if err := validateMeasureGetSetCall(
			result.Results[2+2*i],
			setter,
			measureSetVoltageCommand,
			0,
		); err != nil {
			return nil, fmt.Errorf("sweep point %d: %w", i, err)
		}
		stream := result.Results[3+2*i]
		if err := validateMeasureGetSetCall(
			stream,
			getter,
			"MEASURE_STREAM",
			1,
		); err != nil {
			return nil, fmt.Errorf("sweep point %d: %w", i, err)
		}
		values[i], err = sweepResultValue(
			measurementDispatcher,
			result.ID,
			stream.Return[0],
			getter,
			parsed.getter,
			connected,
		)
		if err != nil {
			return nil, fmt.Errorf("sweep point %d: %w", i, err)
		}
	}
	return newMeasureGetSetResponse(values, parsed.getter)
}
