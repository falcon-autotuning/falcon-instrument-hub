//go:build cgo

// Package measurementdecode turns ISS instrument-call returns into logical
// numeric measurement arrays.
package measurementdecode

import (
	"fmt"
	"math"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
)

// BufferConsumer reads and releases a DataBuffer owned by one measurement.
// MeasurementDispatcher satisfies this interface.
type BufferConsumer interface {
	ConsumeMeasurementSamples(
		measurementID string,
		bufferID string,
	) ([]float64, error)
}
type Target instrumenttarget.Target

// SeriesSpec describes how to find and shape one getter's results.
//
// Every matching call is read in ISS execution order. Scalar values contribute
// one sample, DataBuffers contribute all their samples, and DataBufferArrays
// contribute their buffers in array order. Shape declares the meaning of the
// concatenated samples; the decoder does not infer it from buffer lengths.
type SeriesSpec struct {
	Target     Target
	Verb       string
	ReturnName string
	Unit       falconcore.Unit
	Shape      []int
}

// Series is one normalized, unit-converted result. Data is row-major and its
// length equals the product of Shape.
type Series struct {
	Target Target
	Unit   falconcore.Unit
	Data   []float64
	Shape  []int
}

// Decode extracts one result series for every specification, preserving spec
// order. Buffer values are consumed as part of decoding. Callers should still
// release all buffers for the measurement after Decode returns so buffers left
// behind by an error are cleaned up.
func Decode(
	consumer BufferConsumer,
	result dispatcher.MeasurementResult,
	specs []SeriesSpec,
) ([]Series, error) {
	if result.Err != nil {
		return nil, result.Err
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf(
			"measurement decoder has no series specifications",
		)
	}

	decoded := make([]Series, 0, len(specs))
	for i, spec := range specs {
		series, err := decodeSeries(consumer, result, spec)
		if err != nil {
			return nil, fmt.Errorf("decode series %d: %w", i, err)
		}
		decoded = append(decoded, series)
	}
	return decoded, nil
}

func decodeSeries(
	consumer BufferConsumer,
	result dispatcher.MeasurementResult,
	spec SeriesSpec,
) (Series, error) {
	expected, err := shapeSize(spec.Shape)
	if err != nil {
		return Series{}, err
	}
	if spec.Verb == "" {
		return Series{}, fmt.Errorf("measurement verb is empty")
	}
	if spec.ReturnName == "" {
		return Series{}, fmt.Errorf("return name is empty")
	}

	values := make([]float64, 0, expected)
	matchedCalls := 0
	for _, call := range result.Results {
		if !matches(call, spec) {
			continue
		}
		matchedCalls++

		returned, err := namedReturn(call.Return, spec.ReturnName)
		if err != nil {
			return Series{}, fmt.Errorf(
				"%s on %s: %w",
				spec.Verb,
				targetName(spec.Target),
				err,
			)
		}
		chunk, err := returnSamples(consumer, result.ID, returned)
		if err != nil {
			return Series{}, fmt.Errorf(
				"%s on %s return %q: %w",
				spec.Verb,
				targetName(spec.Target),
				spec.ReturnName,
				err,
			)
		}
		if err := convertSamples(
			chunk,
			returned.Unit,
			spec.Unit,
		); err != nil {
			return Series{}, fmt.Errorf(
				"%s on %s return %q: %w",
				spec.Verb,
				targetName(spec.Target),
				spec.ReturnName,
				err,
			)
		}
		values = append(values, chunk...)
	}

	if matchedCalls == 0 {
		return Series{}, fmt.Errorf(
			"no %s calls found for %s",
			spec.Verb,
			targetName(spec.Target),
		)
	}
	if len(values) != expected {
		return Series{}, fmt.Errorf(
			"received %d samples for shape %v, want %d",
			len(values),
			spec.Shape,
			expected,
		)
	}

	return Series{
		Target: spec.Target,
		Unit:   spec.Unit,
		Data:   values,
		Shape:  append([]int(nil), spec.Shape...),
	}, nil
}

func matches(call instrumentserver.CallResult, spec SeriesSpec) bool {
	return call.Instrument == spec.Target.Instrument &&
		call.Group == spec.Target.Group &&
		int(call.Channel) == spec.Target.Channel &&
		call.Verb == spec.Verb
}

func namedReturn(
	returns []instrumentserver.ReturnValue,
	name string,
) (instrumentserver.ReturnValue, error) {
	var found instrumentserver.ReturnValue
	count := 0
	for _, returned := range returns {
		if returned.Name == name {
			found = returned
			count++
		}
	}
	if count == 0 {
		return instrumentserver.ReturnValue{}, fmt.Errorf(
			"missing return %q",
			name,
		)
	}
	if count > 1 {
		return instrumentserver.ReturnValue{}, fmt.Errorf(
			"contains %d returns named %q",
			count,
			name,
		)
	}
	return found, nil
}

func returnSamples(
	consumer BufferConsumer,
	measurementID string,
	returned instrumentserver.ReturnValue,
) ([]float64, error) {
	switch value := returned.Value.Value.(type) {
	case float64:
		return []float64{value}, nil
	case int64:
		return []float64{float64(value)}, nil
	case instrumentserver.DataBuffer:
		return consumeBuffer(consumer, measurementID, string(value))
	case instrumentserver.DataBufferArray:
		values := make([]float64, 0)
		for _, bufferID := range value {
			chunk, err := consumeBuffer(
				consumer,
				measurementID,
				bufferID,
			)
			if err != nil {
				return nil, err
			}
			values = append(values, chunk...)
		}
		return values, nil
	default:
		return nil, fmt.Errorf(
			"unsupported value type %T",
			returned.Value.Value,
		)
	}
}

func consumeBuffer(
	consumer BufferConsumer,
	measurementID string,
	bufferID string,
) ([]float64, error) {
	if bufferID == "" {
		return nil, fmt.Errorf("buffer ID is empty")
	}
	if consumer == nil {
		return nil, fmt.Errorf("buffer consumer is nil")
	}
	values, err := consumer.ConsumeMeasurementSamples(
		measurementID,
		bufferID,
	)
	if err != nil {
		return nil, fmt.Errorf("consume buffer %q: %w", bufferID, err)
	}
	return values, nil
}

func convertSamples(
	values []float64,
	fromSymbol string,
	to falconcore.Unit,
) error {
	from, err := falconcore.SymbolToUnit(fromSymbol)
	if err != nil {
		return fmt.Errorf("read return unit %q: %w", fromSymbol, err)
	}

	if from == to {
		return validateFinite(values)
	}

	fromUnit, err := from.NewSymbolUnit()
	if err != nil {
		return fmt.Errorf("create return unit %q: %w", fromSymbol, err)
	}
	defer fromUnit.Close()
	toUnit, err := to.NewSymbolUnit()
	if err != nil {
		return fmt.Errorf("create requested unit: %w", err)
	}
	defer toUnit.Close()

	compatible, err := fromUnit.IsCompatibleWith(toUnit)
	if err != nil {
		return fmt.Errorf("compare return and requested units: %w", err)
	}
	if !compatible {
		return fmt.Errorf(
			"return unit %q is incompatible with requested unit %d",
			fromSymbol,
			to,
		)
	}
	for i, value := range values {
		converted, err := fromUnit.ConvertValueTo(value, toUnit)
		if err != nil {
			return fmt.Errorf("convert sample %d: %w", i, err)
		}
		values[i] = converted
	}
	return validateFinite(values)
}

func validateFinite(values []float64) error {
	for i, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("sample %d is not finite", i)
		}
	}
	return nil
}

func shapeSize(shape []int) (int, error) {
	if len(shape) == 0 {
		return 0, fmt.Errorf("series shape is empty")
	}
	maxInt := int(^uint(0) >> 1)
	size := 1
	for axis, length := range shape {
		if length <= 0 {
			return 0, fmt.Errorf(
				"shape axis %d has non-positive length %d",
				axis,
				length,
			)
		}
		if size > maxInt/length {
			return 0, fmt.Errorf(
				"series shape %v is too large",
				shape,
			)
		}
		size *= length
	}
	return size, nil
}

func targetName(target Target) string {
	return fmt.Sprintf(
		"%s/%s/%d",
		target.Instrument,
		target.Group,
		target.Channel,
	)
}
