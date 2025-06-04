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
	SetupInstrumentSubject       = SetupInstrumentCommand + ".external.*"
	DestroyInstrumentSubject     = DestroyInstrumentCommand + ".external.*"
	ConfirmInitializationSubject = ConfirmInitializationCommand + ".*"
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
	instruments   map[string]*InstrumentProcess
	mutex         sync.RWMutex
	subscriptions []*nats.Subscription
}

// subscriptionConfig represents a subscription configuration
type subscriptionConfig struct {
	subject string
	handler nats.MsgHandler
	name    string
}
