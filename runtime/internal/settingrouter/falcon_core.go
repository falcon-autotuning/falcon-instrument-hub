//go:build cgo

package settingrouter

import (
	"fmt"

	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingrequest"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/communications/messages/settingresponse"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/generic/mapinstrumentportquantity"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/instrument-interfaces/names/instrumentport"
	"github.com/falcon-autotuning/falcon-core-libs/go/falcon-core/math/quantity"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
)

// Special ConnectedPort that can hold on to an allocated insturmentportHandle for indexing
type ConnectedPort struct {
	config.ConnectedPort

	// Owned Falcon handle.
	// Caller is responsible for Close().
	FalconPort *instrumentport.Handle
}

// FalconSettingRequest wraps a falcon-core settingrequest handle
// and provides convenience methods for extracting instrument information.
type FalconSettingRequest struct {
	handle *settingrequest.Handle
	ports  *config.ConnectedPorts
}

// NewFalconSettingRequestFromJSON deserializes a settingrequest from JSON
// using the falcon-core API.
func NewFalconSettingRequestFromJSON(
	jsonStr string,
	ports *config.ConnectedPorts,
) (*FalconSettingRequest, error) {
	handle, err := settingrequest.FromJSON(jsonStr)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to deserialize settingrequest from JSON: %w",
			err,
		)
	}

	return &FalconSettingRequest{
		handle: handle,
		ports:  ports,
	}, nil
}

// Close releases the underlying falcon-core handle.
// Must be called when done with the request.
func (r *FalconSettingRequest) Close() error {
	if r.handle != nil {
		return r.handle.Close()
	}
	return nil
}

func (r *FalconSettingRequest) Getters() (
	[]ConnectedPort,
	error,
) {
	getters, err := r.handle.Getters()
	if err != nil {
		return nil, err
	}
	size, err := getters.Size()
	if err != nil {
		return nil, fmt.Errorf("failed to find port len: %w", err)
	}
	result := make([]ConnectedPort, 0, size)

	for i := range size {
		port, err := getters.At(i)
		if err != nil {
			return nil, fmt.Errorf("failed to find a port at location %d: %w", i, err)
		}
		cp, err := r.ports.ResolveConnectedPort(port)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to resolve getter port: %w",
				err,
			)
		}

		result = append(result, ConnectedPort{
			ConnectedPort: cp,
			FalconPort:    port,
		})
	}

	return result, nil
}

type Quantity struct {
	Handle *quantity.Handle
}

func (q Quantity) Value() (float64, error) {
	if q.Handle == nil {
		return 0, fmt.Errorf("quantity handle is nil")
	}
	return q.Handle.Value()
}

func (q Quantity) UnitSymbol() (string, error) {
	if q.Handle == nil {
		return "", fmt.Errorf("quantity handle is nil")
	}

	u, err := q.Handle.Unit()
	if err != nil {
		return "", err
	}

	return u.Symbol()
}

func (q Quantity) Close() error {
	if q.Handle == nil {
		return nil
	}
	return q.Handle.Close()
}

// Setters returns owned quantity handles.
// The caller is responsible for closing all returned quantities.
func (r *FalconSettingRequest) Setters() (
	map[ConnectedPort]Quantity,
	error,
) {
	setters, err := r.handle.Setters()
	if err != nil {
		return nil, err
	}

	size, err := setters.Size()
	if err != nil {
		return nil, fmt.Errorf("failed to find setters len: %w", err)
	}
	keys, err := setters.Keys()
	if err != nil {
		return nil, fmt.Errorf("failed to unpack all the setters: %w", err)
	}

	result := make(map[ConnectedPort]Quantity)

	for i := range size {
		setter, err := keys.At(i)
		if err != nil {
			return nil, fmt.Errorf("failed to find the key at %d: %w", i, err)
		}
		quantity, err := setters.At(setter)
		if err != nil {
			return nil, fmt.Errorf("failed to find the matching instrumentport at %d: %w", i, err)
		}

		cp, err := r.ports.ResolveConnectedPort(setter)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to resolve setter port: %w",
				err,
			)
		}

		result[ConnectedPort{
			ConnectedPort: cp,
			FalconPort:    setter,
		}] = Quantity{
			Handle: quantity,
		}
	}

	return result, nil
}

// Handle returns the underlying settingrequest.Handle for direct API access.
func (r *FalconSettingRequest) Handle() *settingrequest.Handle {
	return r.handle
}

// ToJSON serializes the settingrequest to JSON using the falcon-core API.
func (r *FalconSettingRequest) ToJSON() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.ToJSON()
}

// Message returns the message string from the request.
func (r *FalconSettingRequest) Message() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.Message()
}

// FalconSettingResponse wraps a falcon-core settingresponse handle.
type FalconSettingResponse struct {
	handle *settingresponse.Handle
}

// NewFalconSettingResponseFromJSON deserializes a settingresponse from JSON.
func NewFalconSettingResponseFromJSON(jsonStr string) (*FalconSettingResponse, error) {
	handle, err := settingresponse.FromJSON(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize settingresponse from JSON: %w", err)
	}

	return &FalconSettingResponse{handle: handle}, nil
}

// Close releases the underlying handle.
func (r *FalconSettingResponse) Close() error {
	if r.handle != nil {
		return r.handle.Close()
	}
	return nil
}

// Handle returns the underlying settingresponse.Handle.
func (r *FalconSettingResponse) Handle() *settingresponse.Handle {
	return r.handle
}

// ToJSON serializes the settingresponse to JSON.
func (r *FalconSettingResponse) ToJSON() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.ToJSON()
}

// Message returns the message string from the response.
func (r *FalconSettingResponse) Message() (string, error) {
	if r.handle == nil {
		return "", fmt.Errorf("handle is nil")
	}
	return r.handle.Message()
}

func NewFalconSettingResponse(
	values map[ConnectedPort]Quantity,
) (*FalconSettingResponse, error) {
	setters, err := mapinstrumentportquantity.NewEmpty()
	if err != nil {
		return nil, fmt.Errorf(
			"failed to create setter map: %w",
			err,
		)
	}

	for port, q := range values {

		if port.FalconPort == nil {
			return nil, fmt.Errorf(
				"connected port %q is missing falcon instrument port",
				port.PortName,
			)
		}
		ownedQ, err := quantity.Copy(q.Handle)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to copy quantity for setting response: %w",
				err,
			)
		}

		if err := setters.InsertOrAssign(
			port.FalconPort,
			ownedQ,
		); err != nil {
			_ = ownedQ.Close()
			return nil, fmt.Errorf(
				"failed to insert setter for %q: %w",
				port.PortName,
				err,
			)
		}
	}

	handle, err := settingresponse.New(
		"full setting response",
		setters,
	)
	defer setters.Close()
	if err != nil {
		return nil, err
	}

	return &FalconSettingResponse{
		handle: handle,
	}, nil
}
