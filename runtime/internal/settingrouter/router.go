package settingrouter

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/falconcore"
)

// InstrumentName is the name of each instrument
type InstrumentName string

type InstrumentMetadata struct {
	Characteristics []config.InstrumentCharacteristic
	ConfigFile      *config.InstrumentConfigFile
	API             *config.InstrumentAPI
}

// GetterHandler retrieves the current value of a single connected port.
//
// Getter handlers are responsible for:
//
//   - determining whether they can read a given port
//   - performing any measurements required to obtain a value
//   - converting instrument results into a Falcon Quantity
//
// Handlers are evaluated in registration order. The first handler whose
// CanGet() method returns true is responsible for producing the value.
//
// Getter handlers are intentionally decoupled from the router and may be
// implemented in any package that can import settingrouter.
//
// The only requirement is that the type satisfy the GetterHandler
// interface and be registered in NewRouter().
type GetterHandler interface {
	Name() string

	CanGet(
		port falconcore.InstrumentPort,
		metadata map[InstrumentName]InstrumentMetadata,
		ports *config.ConnectedPorts,
		scriptDir string,
	) bool

	Get(
		port falconcore.InstrumentPort,
		dispatcher *dispatcher.MeasurementDispatcher,
		ports *config.ConnectedPorts,
		instrumentMetadata map[InstrumentName]InstrumentMetadata,
	) (falconcore.Quantity, error)
}

// SetterHandler applies a value to a single connected port.
//
// Setter handlers are responsible for:
//
//   - determining whether they can write a given port
//   - translating a Falcon Quantity into instrument operations
//   - executing any scripts required to perform the write
//
// Handlers are evaluated in registration order. The first handler whose
// CanSet() method returns true is responsible for performing the write.
//
// Setter handlers are intentionally decoupled from the router and may be
// implemented in any package that can import settingrouter.
//
// The only requirement is that the type satisfy the SetterHandler
// interface and be registered in NewRouter().
type SetterHandler interface {
	Name() string

	CanSet(
		port falconcore.InstrumentPort,
		value falconcore.Quantity,
		ports *config.ConnectedPorts,
		scriptDir string,
	) bool

	Set(
		port falconcore.InstrumentPort,
		value falconcore.Quantity,
		dispatcher *dispatcher.MeasurementDispatcher,
		ports *config.ConnectedPorts,
	) error
}

// Router coordinates execution of Falcon setting requests.
//
// Unlike the measurement interpreter, settings are processed as a
// collection of independent operations rather than a single workflow.
//
// A setting request may contain:
//
//   - zero or more setter operations
//   - zero or more getter operations
//
// The router first applies all requested setter operations and then
// evaluates all requested getter operations. Getter results are collected
// and assembled into a FalconSettingResponse.
//
// Individual operations are delegated to registered SetterHandler and
// GetterHandler implementations.
type Router struct {
	dispatcher *dispatcher.MeasurementDispatcher

	setters []SetterHandler
	getters []GetterHandler

	scriptDir string

	ports              *config.ConnectedPorts
	instrumentMetadata map[InstrumentName]InstrumentMetadata
}

// RegisterSetter adds a setter handler to the router.
//
// Handlers are evaluated in registration order.
func (r *Router) RegisterSetter(
	handler SetterHandler,
) *Router {
	r.setters = append(
		r.setters,
		handler,
	)

	return r
}

// RegisterGetter adds a getter handler to the router.
//
// Handlers are evaluated in registration order.
func (r *Router) RegisterGetter(
	handler GetterHandler,
) *Router {
	r.getters = append(
		r.getters,
		handler,
	)

	return r
}

// NewRouter constructs a Router with the supplied measurement dispatcher,
// and connected-port catalog.
//
// This is the central registration point for all setting handlers.
//
// Getter and setter handlers may be implemented anywhere in the codebase.
// A typical organization is:
//
//	internal/settingrouter/
//	    router.go
//
//	internal/settings/
//	    get_voltage.go
//	    set_voltage.go
//	    get_current.go
//	    set_current.go
//
// Each file would expose a constructor returning either a GetterHandler
// or SetterHandler implementation and be registered below.
func NewRouter(
	dispatcher *dispatcher.MeasurementDispatcher,
	ports *config.ConnectedPorts,
	instrumentMetadata map[InstrumentName]InstrumentMetadata,
) *Router {
	scriptDir := ""

	if dispatcher != nil {
		scriptDir = dispatcher.ScriptsPath()
	}
	r := &Router{
		dispatcher:         dispatcher,
		ports:              ports,
		instrumentMetadata: instrumentMetadata,
		scriptDir:          scriptDir,
	}

	// Production handler registrations go here.
	//
	// r.RegisterSetter(NewVoltageSetter())
	// r.RegisterGetter(NewVoltageGetter())

	return r
}

// set routes a single setter operation to the first compatible
// SetterHandler.
//
// Returns an error if no registered handler can write the supplied port.
func (r *Router) set(
	port falconcore.InstrumentPort,
	value falconcore.Quantity,
	ports *config.ConnectedPorts,
) error {
	for _, handler := range r.setters {
		if handler.CanSet(port, value, ports, r.scriptDir) {
			return handler.Set(
				port,
				value,
				r.dispatcher,
				r.ports,
			)
		}
	}

	return fmt.Errorf(
		"no setter handler for %+v",
		port,
	)
}

// get routes a single getter operation to the first compatible
// GetterHandler.
//
// Returns the measured value of the requested port or an error if no
// handler can service the request.
func (r *Router) get(
	port falconcore.InstrumentPort,
	ports *config.ConnectedPorts,
) (falconcore.Quantity, error) {
	for _, handler := range r.getters {
		if handler.CanGet(port, r.instrumentMetadata, ports, r.scriptDir) {
			return handler.Get(
				port,
				r.dispatcher,
				r.ports,
				r.instrumentMetadata,
			)
		}
	}

	return falconcore.Quantity{},
		fmt.Errorf(
			"no getter handler for %+v",
			port,
		)
}

// Handle executes a Falcon setting request.
//
// Processing occurs in two phases:
//
//  1. All setter operations are executed.
//  2. All getter operations are evaluated.
//
// Getter results are collected into a FalconSettingResponse.
//
// Processing setters before getters ensures that any requested reads
// observe the final state established by the writes contained within the
// same request.
func (r *Router) Handle(
	req falconcore.SettingRequest,
) (falconcore.SettingResponse, error) {
	for port, value := range req.Setters {
		if err := r.set(port, value, r.ports); err != nil {
			return falconcore.SettingResponse{}, err
		}
	}
	out := falconcore.SettingResponse{
		Getters: make(
			map[falconcore.InstrumentPort]falconcore.Quantity,
		),
	}
	for _, port := range req.Getters {
		value, err := r.get(port, r.ports)
		if err != nil {
			return out, err
		}

		out.Getters[port] = value
	}

	return out, nil
}
