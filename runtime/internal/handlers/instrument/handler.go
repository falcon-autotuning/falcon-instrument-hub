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
) (*Handler, error) {
	Log := NewLogWrapper(logger, HandlerName)
	portProcessor, err := NewPortProcessor(logger, Log, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create port processor: %w", err)
	}
	h := &Handler{
		logger:        logger,
		Log:           Log,
		natsURL:       natsURL,
		nc:            nc,
		Instruments:   make(map[Name]*InstrumentProcess),
		subscriptions: make([]*nats.Subscription, 0),
		portProcessor: portProcessor,
	}
	return h, nil
}

// GetActiveInstruments returns a list of currently registered instruments
func (h *Handler) GetActiveInstruments() []Name {
	h.Log.Debug("Fetching all the active instrument")
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	names := make([]Name, 0, len(h.Instruments))
	for name := range h.Instruments {
		names = append(names, name)
	}
	return names
}
