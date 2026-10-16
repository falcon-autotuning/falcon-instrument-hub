package interpreter

import (
	"fmt"
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

func averageData(
	data []float64,
	points int,
) ([]float64, error) {
	if points <= 0 {
		return nil, fmt.Errorf(
			"invalid averaging count %d",
			points,
		)
	}

	if len(data)%points != 0 {
		return nil, fmt.Errorf(
			"sample count %d is not divisible by averaging count %d",
			len(data),
			points,
		)
	}

	out := make(
		[]float64,
		0,
		len(data)/points,
	)

	for i := 0; i < len(data); i += points {
		var sum float64

		for j := range points {
			sum += data[i+j]
		}

		out = append(
			out,
			sum/float64(points),
		)
	}

	return out, nil
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
	numberOfPoints := 0
	var output PrimitiveMeasurementOutput
	output, err = plan.Execute(
		executor, &numberOfPoints,
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

	_, getterPorts, err := buildGetters(
		req.Getters,
		ports,
	)
	if numberOfPoints > 0 {
		for target, rawdata := range output.Getters {
			// NOTE: This averaging is currently ignoring the PortTransform which allows for proper averaging specification via analytic functions
			averaged, err := averageData(
				rawdata,
				numberOfPoints,
			)
			if err != nil {
				return MeasurementResponse{}, fmt.Errorf(
					"failed to average getter %q: %w",
					getterPorts[target].DefaultName,
					err,
				)
			}

			output.Getters[target] = averaged
		}
	} else {
		// TODO: send warning message about no averaging
	}
	// TODO: ship to database
	// TODO: convert averaged outputs to measurement response

	return MeasurementResponse{}, nil
}
