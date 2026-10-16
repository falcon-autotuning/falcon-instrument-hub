//go:build cgo

package interpreter

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/devicestate"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/measurementdb"
)

// MeasurementRecorder represents the mandatory database upload required by
// every measurment workflow.
type MeasurementRecorder interface {
	Insert(
		insert measurementdb.InsertEntry,
		entry measurementdb.DatabaseEntry,
	) (measurementdb.CatalogEntry, error)

	Writes() int
}

type trackedMeasurementRecorder struct {
	writes int
}

var measurementInsert = measurementdb.Insert

func (t *trackedMeasurementRecorder) Insert(
	insert measurementdb.InsertEntry,
	entry measurementdb.DatabaseEntry,
) (measurementdb.CatalogEntry, error) {
	out, err := measurementInsert(insert, entry)
	if err != nil {
		return measurementdb.CatalogEntry{}, err
	}

	t.writes++

	return out, nil
}

func (t *trackedMeasurementRecorder) Writes() int {
	return t.writes
}

var _ MeasurementRecorder = (*trackedMeasurementRecorder)(nil)

// StateUpdater represents the mandatory state update required by every
// measurment workflow.
type StateUpdater interface {
	UpdatePort(
		port falconcore.ConnectionName,
		value falconcore.Quantity,
	) error
}

type trackedStateUpdater struct {
	manager *devicestate.DeviceStateManager
	updates int
}

func (t *trackedStateUpdater) UpdatePort(
	port falconcore.ConnectionName,
	value falconcore.Quantity,
) error {
	err := t.manager.UpdatePort(port, value)
	if err != nil {
		return err
	}

	t.updates++
	return nil
}

var _ StateUpdater = (*trackedStateUpdater)(nil)

type (
	DispatchRequest = dispatcher.MeasurementRequest
	DispatchResult  = dispatcher.MeasurementResult
)

type MeasurementRunner interface {
	RunAll(
		[]DispatchRequest,
	) []DispatchResult
}

var _ MeasurementRunner = (*dispatcher.MeasurementDispatcher)(nil)

type (
	MeasurementRequest  = falconcore.MeasurementRequest
	MeasurementResponse = falconcore.MeasurementResponse
)

// MeasurementHandler represents a single measurement workflow.
//
// A handler is responsible for:
//
//   - determining whether it can process an incoming request
//   - extracting parameters from the request
//   - executing any required measurements
//   - constructing the final response
//   - storing the data in the falconcore database
//
// The first matching handler is selected; register specific workflows first.
//
// Measurement handlers are intentionally decoupled from the router and may
// live in any package that can import the interpreter package.
//
// The only requirement is that the type satisfy the
// MeasurementHandler interface and be registered in NewRouter().
type MeasurementHandler interface {
	// Name returns a human-readable identifier used for logging.
	Name() string

	// CanHandle determines whether this handler can process the request.
	CanHandle(
		req MeasurementRequest,
		ports *config.ConnectedPorts,
		scriptDir string,
	) (bool, error)

	// Handle executes the measurement workflow.
	Handle(
		req MeasurementRequest,
		dispatcher MeasurementRunner,
		ports *config.ConnectedPorts,
		tracker StateUpdater,
		recorder MeasurementRecorder,
	) (MeasurementResponse, error)
}

// Router dispatches Falcon measurement requests to the appropriate
// measurement handler.
//
// The router is intentionally static:
//
//   - all handlers are known at compile time
//   - handlers are registered during construction
//   - no runtime plugin mechanism exists
//
// Handlers are checked in registration order until one matches.
type Router struct {
	dispatcher MeasurementRunner

	handlers []MeasurementHandler

	scriptDir string
	ports     *config.ConnectedPorts
	settings  SettingsReader
}

// NewRouter constructs a router using the supplied dispatcher.
//
// This is the single location where measurement handlers are registered.
//
// Handlers may be implemented anywhere in the codebase. They do not need
// to live in the interpreter package. A common organization would be:
//
//	internal/interpreter/
//	    router.go
//
//	internal/measurements/
//	    measure_illumination.go
//	    get_many_voltages.go
//	    charge_scan.go
//	    pinch_off.go
//
// Each file would expose a constructor:
//
//	func NewMeasureIlluminationHandler() MeasurementHandler
//
// and would be registered below.
func NewRouter(
	dispatcher MeasurementRunner,
	ports *config.ConnectedPorts,
	settings SettingsReader,
) *Router {
	return &Router{
		dispatcher: dispatcher,
		ports:      ports,
		handlers:   []MeasurementHandler{},
		settings:   settings,
	}
}

// Handle routes a measurement request to the first matching handler.
//
// If multiple handlers could process a request, the earliest registered
// handler wins. For this reason handlers should either:
//
//   - match mutually-exclusive request types, or
//   - be ordered from most-specific to least-specific.
//
// Returns an error when no handler claims responsibility for the request.
func (r *Router) Handle(
	req MeasurementRequest,
) (MeasurementResponse, error) {
	for _, handler := range r.handlers {
		pass, err := handler.CanHandle(req, r.ports, r.scriptDir)
		if err != nil {
			return MeasurementResponse{}, fmt.Errorf(
				"error while evaluating handler %q: %w",
				handler.Name(),
				err,
			)
		}

		if pass {
			tracker := &trackedStateUpdater{
				manager: devicestate.Manager(),
			}

			if tracker.manager == nil {
				return MeasurementResponse{}, fmt.Errorf(
					"device state manager not initialized",
				)
			}
			if _, err := measurementdb.Instance(); err != nil {
				return MeasurementResponse{}, fmt.Errorf(
					"measurement database not initialized: %w",
					err,
				)
			}

			recorder := &trackedMeasurementRecorder{}

			resp, err := handler.Handle(
				req,
				r.dispatcher,
				r.ports,
				tracker,
				recorder,
			)
			if err != nil {
				return MeasurementResponse{}, err
			}

			if tracker.updates == 0 {
				return MeasurementResponse{}, fmt.Errorf(
					"handler %q completed without updating device state",
					handler.Name(),
				)
			}
			if recorder.Writes() == 0 {
				return MeasurementResponse{}, fmt.Errorf(
					"handler %q completed without writing measurement data",
					handler.Name(),
				)
			}
			return resp, nil
		}
	}

	return MeasurementResponse{}, fmt.Errorf(
		"no measurement handler matched request",
	)
}
