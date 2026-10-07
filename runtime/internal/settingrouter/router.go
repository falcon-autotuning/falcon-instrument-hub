//go:build cgo

package settingrouter

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/dispatcher"
)

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
		port ConnectedPort,
	) bool

	Get(
		port ConnectedPort,
		dispatcher *dispatcher.MeasurementDispatcher,
		wiremap config.WireMap,
		ports *config.ConnectedPorts,
	) (Quantity, error)
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
		port ConnectedPort,
		value Quantity,
	) bool

	Set(
		port ConnectedPort,
		value Quantity,
		dispatcher *dispatcher.MeasurementDispatcher,
		wiremap config.WireMap,
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

	wiremap config.WireMap
	ports   *config.ConnectedPorts
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
// wiremap, and connected-port catalog.
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
	wiremap config.WireMap,
	ports *config.ConnectedPorts,
) *Router {
	r := &Router{
		dispatcher: dispatcher,
		wiremap:    wiremap,
		ports:      ports,
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
	port ConnectedPort,
	value Quantity,
) error {
	for _, handler := range r.setters {
		if handler.CanSet(port, value) {
			return handler.Set(
				port,
				value,
				r.dispatcher,
				r.wiremap,
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
	port ConnectedPort,
) (Quantity, error) {
	for _, handler := range r.getters {
		if handler.CanGet(port) {
			return handler.Get(
				port,
				r.dispatcher,
				r.wiremap,
				r.ports,
			)
		}
	}

	return Quantity{},
		fmt.Errorf(
			"no getter handler for %+v",
			port,
		)
}

func closeQuantities(
	values map[ConnectedPort]Quantity,
) {
	for _, q := range values {
		_ = q.Close()
	}
}

func closePorts(
	ports []ConnectedPort,
) {
	for _, p := range ports {
		if p.FalconPort != nil {
			_ = p.FalconPort.Close()
		}
	}
}

func closePortMap(
	ports map[ConnectedPort]Quantity,
) {
	for p := range ports {
		if p.FalconPort != nil {
			_ = p.FalconPort.Close()
		}
	}
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
	req *FalconSettingRequest,
) (*FalconSettingResponse, error) {
	setters, err := req.Setters()
	if err != nil {
		return nil, err
	}
	defer closeQuantities(setters)
	defer closePortMap(setters)

	for port, value := range setters {
		if err := r.set(port, value); err != nil {
			return nil, err
		}
	}

	getters, err := req.Getters()
	if err != nil {
		return nil, err
	}
	defer closePorts(getters)

	results := map[ConnectedPort]Quantity{}
	defer closeQuantities(results)

	for _, port := range getters {
		value, err := r.get(port)
		if err != nil {
			return nil, err
		}

		results[port] = value
	}

	return NewFalconSettingResponse(results)
}
