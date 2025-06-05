package instrument

import (
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

// NewHandler creates a new instrument handler
func NewHandler(
	logger *logging.Logger,
	natsURL string,
	nc *nats.Conn,
) *Handler {
	return &Handler{
		logger:      logger,
		natsURL:     natsURL,
		nc:          nc,
		instruments: make(map[string]*InstrumentProcess),
	}
}

// GetActiveInstruments returns a list of currently running instruments
func (h *Handler) GetActiveInstruments() []string {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	names := make([]string, 0, len(h.instruments))
	for name := range h.instruments {
		names = append(names, name)
	}
	return names
}
