package instrument

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

// NewHandler creates a new instrument handler
func NewHandler(
	logger *logging.Logger,
	natsURL string,
	nc *nats.Conn,
	cfg *config.Config,
	pythonInterpreter string,
) (*Handler, error) {
	portProcessor, err := NewPortProcessor(logger, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create port processor: %w", err)
	}

	return &Handler{
		logger:            logger,
		natsURL:           natsURL,
		nc:                nc,
		Instruments:       make(map[string]*InstrumentProcess),
		subscriptions:     make([]*nats.Subscription, 0),
		portProcessor:     portProcessor,
		pythonInterpreter: pythonInterpreter,
	}, nil
}

// GetActiveInstruments returns a list of currently running instruments
func (h *Handler) GetActiveInstruments() []string {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	names := make([]string, 0, len(h.Instruments))
	for name := range h.Instruments {
		names = append(names, name)
	}
	return names
}
