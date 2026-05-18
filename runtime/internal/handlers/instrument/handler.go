package instrument

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
	"github.com/nats-io/nats.go"
)

// NewHandler creates a new instrument handler. It builds the port library
// from cfg.InstrumentAPIPaths and connects ports using cfg.WireMap.
func NewHandler(
	logger *logging.Logger,
	natsURL string,
	nc *nats.Conn,
	cfg *config.Config,
) (*Handler, error) {
	Log := NewLogWrapper(logger, HandlerName)

	var portConnections []ports.ConnectedPort

	if len(cfg.InstrumentAPIPaths) > 0 {
		apis, err := ports.ParseInstrumentAPIs(cfg.InstrumentAPIPaths)
		if err != nil {
			return nil, fmt.Errorf("failed to load instrument APIs: %w", err)
		}

		lib := ports.BuildPortLibrary(apis)

		if cfg.WireMap != nil {
			// Convert config.WireMap to map[string]string for ConnectWireMap.
			wireMapStr := make(map[string]string, len(*cfg.WireMap))
			for k, v := range *cfg.WireMap {
				wireMapStr[string(k)] = string(v)
			}
			connected, err := ports.ConnectWireMap(wireMapStr, lib)
			if err != nil {
				Log.Error(fmt.Sprintf("wiremap connection warnings: %v", err))
			}
			portConnections = connected
		} else {
			Log.Info("no wiremap provided; port connections will be empty")
		}
	} else {
		Log.Info("no instrument API paths configured; port connections will be empty")
	}

	h := &Handler{
		logger:          logger,
		Log:             Log,
		natsURL:         natsURL,
		nc:              nc,
		Instruments:     make(map[Name]*InstrumentProcess),
		subscriptions:   make([]*nats.Subscription, 0),
		PortConnections: portConnections,
	}
	return h, nil
}

// Subscribe is a no-op for ISS-based instruments (no NATS lifecycle needed).
func (h *Handler) Subscribe(nc *nats.Conn) error {
	h.nc = nc
	return nil
}

// Unsubscribe is a no-op for ISS-based instruments.
func (h *Handler) Unsubscribe() error {
	return nil
}

// GetActiveInstruments returns a list of currently registered instruments.
func (h *Handler) GetActiveInstruments() []Name {
	h.Log.Debug("Fetching all the active instruments")
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	names := make([]Name, 0, len(h.Instruments))
	for name := range h.Instruments {
		names = append(names, name)
	}
	return names
}

