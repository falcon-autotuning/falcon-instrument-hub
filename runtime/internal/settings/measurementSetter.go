package settings

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/access"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentcharacteristic"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumentserver"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/instrumenttarget"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
)

type MeasurementCharacteristicSetter struct{}

func NewMeasurementCharacteristicSetter() *MeasurementCharacteristicSetter {
	return &MeasurementCharacteristicSetter{}
}

func (s *MeasurementCharacteristicSetter) Name() string {
	return "MeasurementCharacteristicSetter"
}

func lookupSetterDefinition(
	characteristic instrumentcharacteristic.InstrumentCharacteristic,
) (*SetterCharacteristicDefinition, bool) {
	def, ok := setterCharacteristicDefinitions[characteristic]
	if !ok {
		return nil, false
	}

	return &def, true
}

func (s *MeasurementCharacteristicSetter) CanSet(
	port falconcore.InstrumentPort,
	value falconcore.Quantity,
	ports *config.ConnectedPorts,
	scriptDir string,
) bool {
	_ = value

	if !settingrouter.PortExists(
		port,
		ports,
	) {
		return false
	}

	if port.Access != access.Write &&
		port.Access != access.Readwrite {
		return false
	}

	def, ok := lookupSetterDefinition(
		port.Characteristic,
	)
	if !ok {
		return false
	}

	scriptPath := filepath.Join(
		scriptDir,
		def.Script,
	)

	info, err := os.Stat(scriptPath)

	return err == nil && !info.IsDir()
}

func (s *MeasurementCharacteristicSetter) Set(
	port falconcore.InstrumentPort,
	value falconcore.Quantity,
	disp settingrouter.MeasurementRunner,
	ports *config.ConnectedPorts,
) error {
	def, ok := lookupSetterDefinition(
		port.Characteristic,
	)
	if !ok {
		return fmt.Errorf(
			"unsupported characteristic %v",
			port.Characteristic,
		)
	}

	resolvedPort, ok := settingrouter.ResolvePort(
		port,
		ports,
	)
	if !ok {
		return fmt.Errorf(
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
		return fmt.Errorf(
			"serialize instrument target: %w",
			err,
		)
	}

	encodedValue, err := def.Encode(
		value,
	)
	if err != nil {
		return fmt.Errorf(
			"encode value: %w",
			err,
		)
	}

	request := dispatcher.MeasurementRequest{
		Script: def.Script,
		Variables: []instrumentserver.MeasureVariable{
			{
				Name: "setter",
				Value: instrumentserver.VariableValue{
					Value: instrumentserver.InstrumentTarget(
						target,
					),
				},
			},
			{
				Name:  def.ValueName,
				Value: encodedValue,
			},
		},
	}

	results := disp.RunAll(
		[]dispatcher.MeasurementRequest{
			request,
		},
	)

	if len(results) != 1 {
		return fmt.Errorf(
			"expected one result got %d",
			len(results),
		)
	}

	if results[0].Err != nil {
		return results[0].Err
	}

	return nil
}

var _ settingrouter.SetterHandler = (*MeasurementCharacteristicSetter)(nil)
