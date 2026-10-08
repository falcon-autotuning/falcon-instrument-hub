//go:build cgo

// Package measurementresult provides shared checks for ISS measurement returns.
package measurementresult

import (
	"fmt"
	"strings"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
)

// Validate requires sample_rate and bins settings for buffer returns on the
// getter channel. It checks their presence, not their current values, and does
// not read or release buffers. Non-buffer values need no additional checks
// here.
func Validate(
	value instrumentserver.VariableValue,
	getter config.ConnectedPort,
	connected *config.ConnectedPorts,
) error {
	switch raw := value.Value.(type) {
	case instrumentserver.DataBuffer:
		if raw == "" {
			return fmt.Errorf(
				"measurement returned an empty DataBuffer ID",
			)
		}
	case instrumentserver.DataBufferArray:
		for _, id := range raw {
			if id == "" {
				return fmt.Errorf(
					"measurement returned an empty DataBuffer ID",
				)
			}
		}
	default:
		return nil
	}

	if connected == nil {
		return fmt.Errorf(
			"cannot validate DataBuffer settings: connected ports are nil",
		)
	}
	var sampleRate, bins bool
	for _, setting := range connected.AllConnections {
		if !setting.IsSetting() ||
			setting.InstrumentName != getter.InstrumentName ||
			setting.ChannelGroup != getter.ChannelGroup ||
			setting.Channel != getter.Channel {
			continue
		}
		// Port names retain the API IO suffix (or characteristic
		// identifier).
		sampleRate = sampleRate ||
			strings.HasSuffix(
				string(setting.PortName),
				".sample_rate",
			)
		bins = bins ||
			strings.HasSuffix(string(setting.PortName), ".bins")
	}
	if !sampleRate || !bins {
		return fmt.Errorf(
			"DataBuffer getter %q requires sample_rate and bins settings (sample_rate present: %t, bins present: %t)",
			getter.PortName,
			sampleRate,
			bins,
		)
	}
	return nil
}
