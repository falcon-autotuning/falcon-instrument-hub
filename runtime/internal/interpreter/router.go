//go:build cgo

package interpreter

import (
	"fmt"
)

// MeasurementHandler represents a single measurement workflow.
//
// A handler is responsible for:
//
//   - determining whether it can process an incoming request
//   - extracting parameters from the request
//   - executing any required measurements
//   - constructing the final response
//
// Handlers are evaluated in registration order. The first handler whose
// CanHandle() method returns true will process the request.
//
// Measurement handlers are intentionally decoupled from the router and may
// live in any package that can import the interpreter package.
//
// The only requirement is that the type satisfy the
// MeasurementHandler interface and be registered in NewRouter().
type MeasurementHandler interface {
	// Name returns a human-readable identifier used for logging.
	Name() string

	// CanHandle determines whether this handler should process the request.
	CanHandle(
		req *FalconMeasurementRequest,
	) (bool, error)

	// Handle executes the measurement workflow.
	Handle(
		req *FalconMeasurementRequest,
		dispatcher *MeasurementDispatcher,
	) (*FalconMeasurementResponse, error)
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
// Handlers are checked sequentially until one matches.
type Router struct {
	dispatcher *MeasurementDispatcher

	handlers []MeasurementHandler
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
	dispatcher *MeasurementDispatcher,
) *Router {
	return &Router{
		dispatcher: dispatcher,

		handlers: []MeasurementHandler{
			// TODO: Register handlers here:
			//
			// measure_illumination(),
			// get_many_voltages(),
		},
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
	req *FalconMeasurementRequest,
) (*FalconMeasurementResponse, error) {
	for _, handler := range r.handlers {
		pass, err := handler.CanHandle(req)
		if err != nil {
			return nil, fmt.Errorf(
				"error while evaluating handler %q: %w",
				handler.Name(),
				err,
			)
		}
		if pass {
			return handler.Handle(
				req,
				r.dispatcher,
			)
		}
	}

	return nil, fmt.Errorf(
		"no measurement handler matched request",
	)
}
