package settings

import (
	"fmt"
	"strconv"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
)

type limitType struct {
	name            string
	reducer         config.Reducer
	characteristics map[instrumentcharacteristic.InstrumentCharacteristic]struct{}
	getValue        func(*config.IoType) string
}

type resolvedLimitCharacteristic struct {
	Characteristic *config.InstrumentCharacteristic
	IoType         *config.IoType
	Unit           falconcore.Unit
	Value          string
}

func findCharacteristic(
	port falconcore.InstrumentPort,
	instrumentMetadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
) (*config.InstrumentCharacteristic, error) {
	meta, ok := instrumentMetadata[settingrouter.InstrumentName(
		port.InstrumentName,
	)]
	if !ok {
		return nil, fmt.Errorf(
			"instrument metadata not found for %s",
			port.InstrumentName,
		)
	}

	for i := range meta.Characteristics {
		c := &meta.Characteristics[i]

		if c.Characteristic == port.Characteristic {
			return c, nil
		}
	}

	return nil, fmt.Errorf(
		"characteristic %v not found for instrument %s",
		port.Characteristic,
		port.InstrumentName,
	)
}

func resolveLimitCharacteristic(
	port falconcore.InstrumentPort,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	ports *config.ConnectedPorts,
	limit limitType,
) (*resolvedLimitCharacteristic, error) {
	if !settingrouter.PortExists(port, ports) {
		return nil, fmt.Errorf(
			"port %+v is not present in connected ports",
			port,
		)
	}

	if port.Access != access.Read &&
		port.Access != access.Readwrite {
		return nil, fmt.Errorf(
			"port %+v must have read access",
			port,
		)
	}

	if _, ok := limit.characteristics[port.Characteristic]; !ok {
		return nil, fmt.Errorf(
			"unsupported %s characteristic %v",
			limit.name,
			port.Characteristic,
		)
	}

	meta, ok := metadata[settingrouter.InstrumentName(
		port.InstrumentName,
	)]
	if !ok {
		return nil, fmt.Errorf(
			"instrument metadata not found for %s",
			port.InstrumentName,
		)
	}

	characteristic, err := findCharacteristic(
		port,
		metadata,
	)
	if err != nil {
		return nil, err
	}

	if characteristic.ReadCommand.Command == "" ||
		characteristic.ReadCommand.Reducer != limit.reducer {
		return nil, fmt.Errorf(
			"characteristic %v is not configured as %s getter",
			port.Characteristic,
			limit.name,
		)
	}

	if meta.API == nil {
		return nil, fmt.Errorf(
			"instrument %s has no API definition",
			port.InstrumentName,
		)
	}

	for i := range meta.API.IO {
		io := &meta.API.IO[i]

		if io.Name != characteristic.ReadCommand.ParameterName {
			continue
		}

		value := limit.getValue(io)

		if value == "" {
			return nil, fmt.Errorf(
				"io type %s has no %s value",
				io.Name,
				limit.name,
			)
		}

		unit, err := falconcore.SymbolToUnit(
			io.Unit,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"unsupported unit %q: %w",
				io.Unit,
				err,
			)
		}

		return &resolvedLimitCharacteristic{
			Characteristic: characteristic,
			IoType:         io,
			Unit:           unit,
			Value:          value,
		}, nil
	}

	return nil, fmt.Errorf(
		"parameter %s not found in API",
		characteristic.ReadCommand.ParameterName,
	)
}

func getLimitQuantity(
	port falconcore.InstrumentPort,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	ports *config.ConnectedPorts,
	limit limitType,
) (falconcore.Quantity, error) {
	resolved, err := resolveLimitCharacteristic(
		port,
		metadata,
		ports,
		limit,
	)
	if err != nil {
		return falconcore.Quantity{}, err
	}

	value, err := strconv.ParseFloat(
		resolved.Value,
		64,
	)
	if err != nil {
		return falconcore.Quantity{},
			fmt.Errorf(
				"parse %s value %q: %w",
				limit.name,
				resolved.Value,
				err,
			)
	}

	return falconcore.Quantity{
		Value: value,
		Unit:  port.Unit,
	}, nil
}
