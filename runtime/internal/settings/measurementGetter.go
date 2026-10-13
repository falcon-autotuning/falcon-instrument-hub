package settings

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
)

type MeasurementCharacteristicGetter struct{}

func NewMeasurementCharacteristicGetter() *MeasurementCharacteristicGetter {
	return &MeasurementCharacteristicGetter{}
}

func (g *MeasurementCharacteristicGetter) Name() string {
	return "MeasurementCharacteristicGetter"
}

func lookupDefinition(
	characteristic instrumentcharacteristic.InstrumentCharacteristic,
) (*CharacteristicDefinition, bool) {
	def, ok := characteristicDefinitions[characteristic]

	if !ok {
		return nil, false
	}

	return &def, true
}

func (g *MeasurementCharacteristicGetter) CanGet(
	port falconcore.InstrumentPort,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
	ports *config.ConnectedPorts,
	scriptDir string,
) bool {
	if !settingrouter.PortExists(
		port,
		ports,
	) {
		return false
	}

	def, ok := lookupDefinition(
		port.Characteristic,
	)

	if !ok {
		return false
	}

	meta, ok := metadata[settingrouter.InstrumentName(
		port.InstrumentName,
	)]

	if !ok || meta.API == nil {
		return false
	}

	scriptPath := filepath.Join(scriptDir, def.Script)
	info, err := os.Stat(scriptPath)
	return err == nil && !info.IsDir()
}

func (g *MeasurementCharacteristicGetter) Get(
	port falconcore.InstrumentPort,
	disp settingrouter.MeasurementRunner,
	ports *config.ConnectedPorts,
	metadata map[settingrouter.InstrumentName]settingrouter.InstrumentMetadata,
) (falconcore.Quantity, error) {
	def, ok := lookupDefinition(
		port.Characteristic,
	)
	if !ok {
		return falconcore.Quantity{},
			fmt.Errorf(
				"unsupported characteristic %v",
				port.Characteristic,
			)
	}

	_, ok = metadata[settingrouter.InstrumentName(
		port.InstrumentName,
	)]
	if !ok {
		return falconcore.Quantity{},
			fmt.Errorf(
				"metadata missing for %s",
				port.InstrumentName,
			)
	}

	resolvedPort, ok := settingrouter.ResolvePort(
		port,
		ports,
	)
	if !ok {
		return falconcore.Quantity{},
			fmt.Errorf(
				"unable to resolve port %+v",
				port,
			)
	}

	target, err := instrumenttarget.Target{
		Instrument: resolvedPort.InstrumentName,
		Group:      resolvedPort.ChannelGroup,
		Channel:    resolvedPort.Channel,
	}.Serialize()
	if err != nil {
		return falconcore.Quantity{},
			fmt.Errorf(
				"serialize instrument target: %w",
				err,
			)
	}

	request := dispatcher.MeasurementRequest{
		Script: def.Script,
		Variables: []instrumentserver.MeasureVariable{
			{
				Name: "getter",
				Value: instrumentserver.VariableValue{
					Value: instrumentserver.InstrumentTarget(
						target,
					),
				},
			},
		},
	}

	results := disp.RunAll(
		[]dispatcher.MeasurementRequest{
			request,
		},
	)

	if len(results) != 1 {
		return falconcore.Quantity{},
			fmt.Errorf(
				"expected one result got %d",
				len(results),
			)
	}

	return def.Decode(
		results[0],
		port.Unit,
	)
}
