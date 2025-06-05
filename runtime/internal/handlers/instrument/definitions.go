package instrument

import (
	"context"
	"embed"
	"os"
	"os/exec"
	"sync"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
	"github.com/nats-io/nats.go"
)

// Handler Names
const (
	HandlerName = "INSTRUMENT_HANDLER"
)

// File Paths
const (
	ScriptsDir                 = "./"
	LaunchInstrumentScriptName = "launch_instrument_daemon.py"
)

// Process Management
const (
	GracefulShutdownTimeout = 5 // seconds
)

//go:embed launch_instrument_daemon.py
var embeddedScript embed.FS

var (
	SetupInstrumentCommand       = api.GetCommandName(api.SetupInstrument{})
	DestroyInstrumentCommand     = api.GetCommandName(api.DestroyInstrument{})
	ConfirmInitializationCommand = api.GetCommandName(
		api.ConfirmInitialization{},
	)
	UpdateDaemonPropertyCommand = api.GetCommandName(
		api.UpdateDaemonProperty{},
	)
	SetCommand                   = api.GetCommandName(api.Set{})
	SetupInstrumentSubject       = SetupInstrumentCommand + ".external.*"
	DestroyInstrumentSubject     = DestroyInstrumentCommand + ".external.*"
	ConfirmInitializationSubject = ConfirmInitializationCommand + ".*"
	UpdateDaemonPropertySubject  = UpdateDaemonPropertyCommand + ".instrument-server"
)

// InstrumentProcess represents a running instrument daemon
type InstrumentProcess struct {
	Name          string
	Process       *os.Process
	Cmd           *exec.Cmd
	Cancel        context.CancelFunc
	Ports         map[string]any
	Configuration map[string]any
	Initialized   bool
}

// Handler handles instrument setup and destruction
type Handler struct {
	logger        *logging.Logger
	natsURL       string
	nc            *nats.Conn
	Instruments   map[string]*InstrumentProcess
	mutex         sync.RWMutex
	subscriptions []*nats.Subscription
	portProcessor *PortProcessor
}

// subscriptionConfig represents a subscription configuration
type subscriptionConfig struct {
	subject string
	handler nats.MsgHandler
	name    string
}

// CollectPortProperties collects port properties from all active instruments
func (h *Handler) CollectPortProperties() (knobs, meters []string) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	if h.portProcessor != nil {
		return h.portProcessor.CollectPortProperties(h.Instruments)
	}
	return nil, nil
}
