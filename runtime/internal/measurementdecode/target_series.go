//go:build cgo

package measurementdecode

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
)

// TargetSeriesSpec describes a getter without tying it to an instrument ISA
// verb or return field. The expected shape is supplied by the measurement plan
// and the shared number of samples per waveform position.
type TargetSeriesSpec struct {
	Target      Target
	Unit        falconcore.Unit
	Shape       []int
	BuffersOnly bool
}

// DecodeTargets combines returns for each requested target in call order.
// Returns with an incompatible or unknown unit are ignored (for example,
// configuration commands on the same instrument). A compatible return must
// contain a numeric value or buffer. The final sample count must match Shape.
func DecodeTargets(
	consumer BufferConsumer,
	result dispatcher.MeasurementResult,
	specs []TargetSeriesSpec,
) ([]Series, error) {
	if result.Err != nil {
		return nil, result.Err
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("measurement decoder has no target specifications")
	}
	out := make([]Series, 0, len(specs))
	for _, spec := range specs {
		expected, err := shapeSize(spec.Shape)
		if err != nil {
			return nil, err
		}
		values := make([]float64, 0, expected)
		for _, call := range result.Results {
			if call.Instrument != spec.Target.Instrument ||
				call.Group != spec.Target.Group ||
				int(call.Channel) != spec.Target.Channel {
				continue
			}
			for _, returned := range call.Return {
				if returned.Unit == "" ||
					convertSamples(nil, returned.Unit, spec.Unit) != nil {
					continue
				}
				if spec.BuffersOnly {
					switch returned.Value.Value.(type) {
					case instrumentserver.DataBuffer, instrumentserver.DataBufferArray:
					default:
						continue
					}
				}
				chunk, err := returnSamples(consumer, result.ID, returned)
				if err != nil {
					return nil, fmt.Errorf("target %s: %w", targetName(spec.Target), err)
				}
				if err := convertSamples(chunk, returned.Unit, spec.Unit); err != nil {
					return nil, fmt.Errorf("target %s: %w", targetName(spec.Target), err)
				}
				values = append(values, chunk...)
			}
		}
		if len(values) != expected {
			return nil, fmt.Errorf("target %s returned %d samples, shape %v requires %d",
				targetName(spec.Target), len(values), spec.Shape, expected)
		}
		out = append(out, Series{
			Target: spec.Target,
			Unit:   spec.Unit,
			Data:   values,
			Shape:  append([]int(nil), spec.Shape...),
		})
	}
	return out, nil
}
