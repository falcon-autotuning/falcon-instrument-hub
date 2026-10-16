package interpreter

import "github.com/falcon-autotuning/instrument-server/runtime/internal/config"

type CartesianMeasurementHandler struct {
	executor Executor
}

func (h *CartesianMeasurementHandler) CanHandle(
	req MeasurementRequest,
	ports *config.ConnectedPorts,
	scriptDir string,
) (bool, error)

// TODO: check that the ports in the req	are in the ports
// return parseDimensionality(req) != Dim0, nil

func (h *CartesianMeasurementHandler) Handle(
	req MeasurementRequest,
	dispatcher MeasurementRunner,
	ports *config.ConnectedPorts,
	tracker StateUpdater,
	recorder MeasurementRecorder,
) (MeasurementResponse, error) {
	var availableDimensions []Dimensionality
	// TODO: buildup the availableDimensions by checking script names
	var err error
	var executor Executor
	executor, err = BuildMeasurementRegistry(availableDimensions, dispatcher)
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

	// TODO: update the state
	// TODO: perform averaging
	// TODO: ship to database on go routine

	// 	return primitiveOutputToResponse(
	// 		req,
	// 		output,
	// 		tracker,
	// 		recorder,
	// 	)
	return MeasurementResponse{}, nil
}
