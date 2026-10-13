package settings

import (
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"

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

var maxLimit = limitType{
	name:            "max",
	reducer:         config.ReducerMax,
	characteristics: maxCharacteristics,
	getValue: func(io *config.IoType) string {
		return io.Max
	},
}

type MaxCharacteristicGetter struct{}

func NewMaxCharacteristicGetter() *MaxCharacteristicGetter {
	return &MaxCharacteristicGetter{}
}

func (g *MaxCharacteristicGetter) Name() string {
	return "MaxCharacteristicGetter"
}

func (g *MaxCharacteristicGetter) CanGet(
	port falconcore.InstrumentPort,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	ports *config.ConnectedPorts,
) bool {
	_, err := resolveLimitCharacteristic(
		port,
		metadata,
		ports,
		maxLimit,
	)

	return err == nil
}

func (g *MaxCharacteristicGetter) Get(
	port falconcore.InstrumentPort,
	_ *dispatcher.MeasurementDispatcher,
	_ config.WireMap,
	ports *config.ConnectedPorts,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
) (falconcore.Quantity, error) {
	return getLimitQuantity(
		port,
		metadata,
		ports,
		maxLimit,
	)
}

var _ settingrouter.GetterHandler = (*MaxCharacteristicGetter)(nil)
