package instrument

import (
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
)

const HandlerName = "INSTRUMENT_HANDLER"

// Handler holds port connections built from instrument APIs and the wiremap.
// ISS owns instrument lifecycle and execution state.
type Handler struct {
	mutex sync.RWMutex
	// PortConnections maps physical device gates to instrument channel capabilities.
	PortConnections []ports.ConnectedPort
}

// CollectPortProperties partitions PortConnections into knobs (outputs) and
// meters (inputs). Settings are excluded.
func (h *Handler) CollectPortProperties() (knobs, meters []ports.ConnectedPort) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	for _, cp := range h.PortConnections {
		switch {
		case cp.IsKnob():
			knobs = append(knobs, cp)
		case cp.IsMeter():
			meters = append(meters, cp)
		}
	}
	return
}
