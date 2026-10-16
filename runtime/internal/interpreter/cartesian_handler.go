package interpreter

import (
	"os"
	"path/filepath"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/settingrouter"
)

type CartesianMeasurementHandler struct {
	settings SettingsReader
}

func (h *CartesianMeasurementHandler) CanHandle(
	req MeasurementRequest,
	ports *config.ConnectedPorts,
	scriptDir string,
) (bool, error) {
	if len(req.Waveforms) < 1 {
		return false, nil
	}
	// check that the ports in the req	are in the ports
	for _, getter := range req.Getters {
		if !settingrouter.PortExists(getter, ports) {
			return false, nil
		}
	}
	for _, axis := range req.Waveforms[0].Space.Axes {
		for _, domain := range axis {
			if !settingrouter.PortExists(domain.InstrumentPort, ports) {
				return false, nil
			}
		}
	}
	return true, nil
}

type DeviceStateReader interface {
	ReadSourceVoltages(
		ports *config.ConnectedPorts,
	) (
		map[falconcore.InstrumentPort]falconcore.Quantity,
		error,
	)
}

type ScriptDefinition struct {
	Name           string
	Dimensionality Dimensionality
}

var scriptDefinitions = []ScriptDefinition{
	{
		Name:           "measure_get_set",
		Dimensionality: Dim0,
	},
	{
		Name:           "measure_1D_buffered",
		Dimensionality: Dim1,
	},
	{
		Name:           "measure_2D_buffered",
		Dimensionality: Dim2,
	},
}

func (h *CartesianMeasurementHandler) Handle(
	req MeasurementRequest,
	dispatcher MeasurementRunner,
	ports *config.ConnectedPorts,
	tracker StateUpdater,
	recorder MeasurementRecorder,
) (MeasurementResponse, error) {
	var availableDimensions []Dimensionality
	for _, script := range scriptDefinitions {
		scriptPath := filepath.Join(
			dispatcher.ScriptsPath(),
			script.Name+".lua",
		)

		if _, err := os.Stat(scriptPath); err == nil {
			availableDimensions = append(
				availableDimensions,
				script.Dimensionality,
			)
		}
	}
	var err error
	var executor Executor
	// executor, err = BuildMeasurementRegistry(availableDimensions, dispatcher)
	plan, err := BuildPlan(req, ports, availableDimensions)
	if err != nil {
		return MeasurementResponse{}, err
	}
	var numberOfPoints *int = nil
	_, err = plan.Execute(
		executor, numberOfPoints,
	)
	if err != nil {
		return MeasurementResponse{}, err
	}
	var voltages map[falconcore.InstrumentPort]falconcore.Quantity
	voltages, err = h.readSourceVoltages(ports)
	_ = voltages
	if err != nil {
		return MeasurementResponse{}, err
	}
	for port, value := range voltages {
		tracker.UpdatePort(port.PseudoName.Name, value)
	}

	if numberOfPoints != nil {
		// TODO: perform averaging
	} else {
		// TODO: send warning message about no averaging
	}
	// TODO: ship to database
	// TODO: convert averaged outputs to measurement response

	return MeasurementResponse{}, nil
}
