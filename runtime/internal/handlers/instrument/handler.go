package instrument

import (
	"fmt"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
)

// NewHandler creates a new instrument handler. It builds the port library
// from cfg.InstrumentAPIPaths and connects ports using cfg.WireMap.
func NewHandler(
	logger *logging.Logger,
	cfg *config.Config,
) (*Handler, error) {
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
				logger.Error(HandlerName, fmt.Sprintf("wiremap connection warnings: %v", err))
			}
			portConnections = connected
		} else {
			logger.Info(HandlerName, "no wiremap provided; port connections will be empty")
		}
	} else {
		logger.Info(HandlerName, "no instrument API paths configured; port connections will be empty")
	}

	return &Handler{
		PortConnections: portConnections,
	}, nil
}
