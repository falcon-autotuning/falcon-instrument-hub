package settings

import (
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
)

var minCharacteristics = map[instrumentcharacteristic.InstrumentCharacteristic]struct{}{
	instrumentcharacteristic.InstrumentCharacteristicMinSampleRate:       {},
	instrumentcharacteristic.InstrumentCharacteristicMinSourceVoltage:    {},
	instrumentcharacteristic.InstrumentCharacteristicMinVoltageRampSlope: {},
	instrumentcharacteristic.InstrumentCharacteristicMinSourceCurrent:    {},
	instrumentcharacteristic.InstrumentCharacteristicMinCurrentRampSlope: {},
	instrumentcharacteristic.InstrumentCharacteristicMinSourceFrequency:  {},
	instrumentcharacteristic.InstrumentCharacteristicMinSinkFrequency:    {},
	instrumentcharacteristic.InstrumentCharacteristicMinAcAmplitude:      {},
	instrumentcharacteristic.InstrumentCharacteristicMinNumberOfSamples:  {},
	instrumentcharacteristic.InstrumentCharacteristicMinMagnetStrength:   {},
}

var minLimit = limitType{
	name:            "min",
	reducer:         config.ReducerMin,
	characteristics: minCharacteristics,
	getValue: func(io *config.IoType) string {
		return io.Min
	},
}

type MinCharacteristicGetter struct{}

func NewMinCharacteristicGetter() *MinCharacteristicGetter {
	return &MinCharacteristicGetter{}
}

func (g *MinCharacteristicGetter) Name() string {
	return "MinCharacteristicGetter"
}

func (g *MinCharacteristicGetter) CanGet(
	port falconcore.InstrumentPort,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	ports *config.ConnectedPorts,
	scriptDir string,
) bool {
	_, err := resolveLimitCharacteristic(
		port,
		metadata,
		ports,
		minLimit,
	)

	return err == nil
}

func (g *MinCharacteristicGetter) Get(
	port falconcore.InstrumentPort,
	_ *dispatcher.MeasurementDispatcher,
	ports *config.ConnectedPorts,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
) (falconcore.Quantity, error) {
	return getLimitQuantity(
		port,
		metadata,
		ports,
		minLimit,
	)
}

var _ settingrouter.GetterHandler = (*MinCharacteristicGetter)(nil)
