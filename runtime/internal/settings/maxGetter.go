package settings

import (
	"fmt"
	"strconv"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
)

var maxCharacteristics = map[instrumentcharacteristic.InstrumentCharacteristic]struct{}{
	instrumentcharacteristic.InstrumentCharacteristicMaxSampleRate:       {},
	instrumentcharacteristic.InstrumentCharacteristicMaxSourceVoltage:    {},
	instrumentcharacteristic.InstrumentCharacteristicMaxVoltageRampSlope: {},
	instrumentcharacteristic.InstrumentCharacteristicMaxSourceCurrent:    {},
	instrumentcharacteristic.InstrumentCharacteristicMaxCurrentRampSlope: {},
	instrumentcharacteristic.InstrumentCharacteristicMaxSourceFrequency:  {},
	instrumentcharacteristic.InstrumentCharacteristicMaxSinkFrequency:    {},
	instrumentcharacteristic.InstrumentCharacteristicMaxAcAmplitude:      {},
	instrumentcharacteristic.InstrumentCharacteristicMaxNumberOfSamples:  {},
	instrumentcharacteristic.InstrumentCharacteristicMaxMagnetStrength:   {},
}

type MaxCharacteristicGetter struct{}

func NewMaxCharacteristicGetter() *MaxCharacteristicGetter {
	return &MaxCharacteristicGetter{}
}

func (g *MaxCharacteristicGetter) Name() string {
	return "MaxCharacteristicGetter"
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

func supportsMaxGetter(
	c config.InstrumentCharacteristic,
) bool {
	return c.ReadCommand.Command != "" &&
		c.ReadCommand.Reducer == config.ReducerMax
}

type resolvedMaxCharacteristic struct {
	Characteristic *config.InstrumentCharacteristic
	IoType         *config.IoType
	Unit           falconcore.Unit
}

func resolveMaxCharacteristic(
	port falconcore.InstrumentPort,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	ports *config.ConnectedPorts,
) (*resolvedMaxCharacteristic, error) {
	if !settingrouter.PortExists(port, ports) {
		return nil, fmt.Errorf(
			"port %+v is not present in connected ports",
			port,
		)
	}
	if !(port.Access == access.Read || port.Access == access.Readwrite) {
		return nil, fmt.Errorf("port %+v must have read access", port)
	}

	if _, ok := maxCharacteristics[port.Characteristic]; !ok {
		return nil, fmt.Errorf(
			"unsupported max characteristic %v",
			port.Characteristic,
		)
	}

	meta, ok := metadata[settingrouter.InstrumentName(port.InstrumentName)]
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

	if !supportsMaxGetter(*characteristic) {
		return nil, fmt.Errorf(
			"characteristic %v is not configured as max getter",
			port.Characteristic,
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

		if io.Max == "" {
			return nil, fmt.Errorf(
				"io type %s has no max value",
				io.Name,
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

		return &resolvedMaxCharacteristic{
			Characteristic: characteristic,
			IoType:         io,
			Unit:           unit,
		}, nil
	}

	return nil, fmt.Errorf(
		"parameter %s not found in API",
		characteristic.ReadCommand.ParameterName,
	)
}

func (g *MaxCharacteristicGetter) CanGet(
	port falconcore.InstrumentPort,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	ports *config.ConnectedPorts,
) bool {
	_, err := resolveMaxCharacteristic(
		port,
		metadata,
		ports,
	)

	return err == nil
}

func (g *MaxCharacteristicGetter) Get(
	port falconcore.InstrumentPort,
	dispatcher *dispatcher.MeasurementDispatcher,
	wiremap config.WireMap,
	ports *config.ConnectedPorts,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
) (falconcore.Quantity, error) {
	resolved, err := resolveMaxCharacteristic(
		port,
		metadata,
		ports,
	)
	if err != nil {
		return falconcore.Quantity{}, err
	}

	value, err := strconv.ParseFloat(
		resolved.IoType.Max,
		64,
	)
	if err != nil {
		return falconcore.Quantity{},
			fmt.Errorf(
				"parse max value %q: %w",
				resolved.IoType.Max,
				err,
			)
	}

	return falconcore.Quantity{
		Value: value,
		Unit:  port.Unit,
	}, nil
}

var _ settingrouter.GetterHandler = (*MaxCharacteristicGetter)(nil)
