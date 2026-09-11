package instrument

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
)

// ResolveConnectedPort resolves a logical device name plus IO/capability name
// to exactly one connected instrument port.
func (h *Handler) ResolveConnectedPort(deviceName, ioTypeName, role string) (ports.ConnectedPort, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	var matches []ports.ConnectedPort
	for _, cp := range h.PortConnections {
		if cp.DeviceName == deviceName && cp.IoTypeName == ioTypeName && cp.Role == role {
			matches = append(matches, cp)
		}
	}

	switch len(matches) {
	case 0:
		return ports.ConnectedPort{}, fmt.Errorf(
			"no connected port for device_name=%q io_type_name=%q role=%q",
			deviceName,
			ioTypeName,
			role,
		)
	case 1:
		return matches[0], nil
	default:
		return ports.ConnectedPort{}, fmt.Errorf(
			"ambiguous connected port for device_name=%q io_type_name=%q role=%q: %d matches",
			deviceName,
			ioTypeName,
			role,
			len(matches),
		)
	}
}
