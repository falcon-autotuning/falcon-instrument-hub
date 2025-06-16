package measure

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/config"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	MeasurementReadyHandlerName                         = "MEASUREMENT_READY_HANDLER"
	arm                         instrument.PropertyName = "ARM"
)

// SubscriptionConfig defines a subscription configuration
type SubscriptionConfig struct {
	Subject     string
	HandlerFunc func(*nats.Msg)
	SubField    **nats.Subscription // pointer to the subscription field in the handler
	Name        string              // for logging
}

var (
	ArmedMessage            = api.GetCommandName(api.Armed{})
	ExecutingMessage        = api.GetCommandName(api.Executing{})
	MeasurementReadyMessage = api.GetCommandName(api.MeasurementReady{})
	ProcessDataMessage      = api.GetCommandName(api.ProcessData{})
	TriggerMessage          = api.GetCommandName(api.Trigger{})
	ReturnDataMessage       = api.GetCommandName(api.ReturnData{})
	UploadDataMessage       = api.GetCommandName(api.UploadData{})
	GetMessage              = api.GetCommandName(api.Get{})
)

// MeasurementScheduler tracks measurements waiting for RETURN_DATA
type MeasurementScheduler struct {
	ID                       instrument.MeasurementID    // Combined ProcessId and ChunkId
	GetterPorts              []instrument.JsonPort       // Original getter ports
	SetterPorts              []instrument.JsonPort       // Original setter ports
	GetterInstruments        []instrument.Name           // Instruments that need to be armed
	SetterInstruments        []instrument.Name           // Instruments that need to be armed
	MasterTriggerInstruments []instrument.Name           // Master instruments for hardware trigger
	ReceivedReturns          int                         // Number of RETURN_DATA messages received
	ExpectedReturns          int                         // Expected number of RETURN_DATA messages
	Results                  map[instrument.JsonPort]any // Port -> Data mapping
	ReadyChecklist           map[instrument.Name]bool    // Setter instrument -> ready status
	TriggeredGetterChecklist map[instrument.Name]bool    // Getter instrument -> triggered status
}

func (ms *MeasurementScheduler) registerReadySetter(
	name instrument.Name,
) error {
	if !slices.Contains(ms.SetterInstruments, name) {
		return fmt.Errorf(
			"instrument %s not found in setter instruments. Available setter instruments: %v",
			name,
			ms.SetterInstruments,
		)
	}
	ms.ReadyChecklist[name] = true
	return nil
}

func (ms *MeasurementScheduler) settersAreReady() bool {
	for _, name := range ms.SetterInstruments {
		if !ms.ReadyChecklist[name] {
			return false
		}
	}
	return true
}

func (ms *MeasurementScheduler) resetSettersReadiness() {
	for name := range ms.ReadyChecklist {
		ms.ReadyChecklist[name] = false
	}
}

func (ms *MeasurementScheduler) gettersAreTriggered() bool {
	for _, name := range ms.GetterInstruments {
		if !ms.TriggeredGetterChecklist[name] {
			return false
		}
	}
	return true
}

func (ms *MeasurementScheduler) resetGettersTriggered() {
	for name := range ms.TriggeredGetterChecklist {
		ms.TriggeredGetterChecklist[name] = false
	}
}

func (ms *MeasurementScheduler) containsGetter(port instrument.JsonPort) bool {
	return slices.Contains(ms.GetterPorts, port)
}

func (ms *MeasurementScheduler) storeData(port instrument.JsonPort, data any) {
	ms.Results[port] = data
	ms.ReceivedReturns++
}

func (ms *MeasurementScheduler) allDataHere() bool {
	return ms.ReceivedReturns == ms.ExpectedReturns
}

type Instructions struct {
	Setter   instrument.JsonPort       `json:"setter"`
	Property []instrument.PropertyName `json:"property"`
	Values   []any                     `json:"values"`
}

type InstrumentInstructions struct {
	Name            instrument.Name
	SetInstructions []instrument.SetInstruction
}

// append adds a new instruction to the list
func (ii *InstrumentInstructions) append(in Instructions) {
	ii.SetInstructions = append(ii.SetInstructions, in.separate()...)
}

// peek returns the first instruction without removing it
func (ii *InstrumentInstructions) peek() *instrument.SetInstruction {
	return &ii.SetInstructions[0]
}

// arm will add an arm instruction to the end of the lists
func (ii *InstrumentInstructions) arm() {
	// any Instructions for the instrument will work as a surrogate
	newii := Instructions{
		Setter:   ii.peek().Name,
		Property: []instrument.PropertyName{arm},
		Values:   []any{true},
	}
	// TODO: seperate the arm instruction from the get go

	ii.append(newii)
}

// MeasurementReadyHandler handles MEASUREMENT_READY requests
type MeasurementReadyHandler struct {
	logger              *logging.Logger
	log                 *instrument.LogWrapper
	nc                  *nats.Conn
	subscription        *nats.Subscription
	armedSub            *nats.Subscription
	executingSub        *nats.Subscription
	returnDataSub       *nats.Subscription
	instrumentHandler   *instrument.Handler
	config              *config.Config
	measurementStack    *MeasurementStack
	currentMeasurement  *MeasurementStackItem
	isProcessing        bool
	getResults          map[instrument.ID]map[instrument.JsonPort]any
	schedulers          map[instrument.ID]map[instrument.ID]*MeasurementScheduler // ProcessId -> ChunkId -> Scheduler
	pendingMeasurements map[instrument.ID]*MeasurementScheduler
	pendingGets         map[instrument.ID]any
	NextChunkId         int64 // Unique identifier for the next chunk
	mutex               sync.RWMutex
}

// separate converts the Instructions into a slice of SetInstruction
func (in *Instructions) separate() []instrument.SetInstruction {
	var instructions []instrument.SetInstruction
	for i, property := range in.Property {
		instructions = append(instructions, instrument.SetInstruction{
			Name:     in.Setter,
			Property: property,
			Value:    in.Values[i],
		})
	}
	return instructions
}

// fromJson loads instructions from a JSON string
func (in *Instructions) fromJson(jsonStr string) error {
	err1 := json.Unmarshal([]byte(jsonStr), &in)
	// marshal cycling the Setter to ensure it is a valid JsonPort
	fixed_bytes, err2 := json.Marshal(in.Setter)
	err3 := json.Unmarshal(fixed_bytes, &in.Setter)
	if err1 == nil && err2 == nil && err3 == nil {
		return nil
	}
	var errorMsgs []string
	if err1 != nil {
		errorMsgs = append(errorMsgs, fmt.Sprintf("unmarshal error: %v", err1))
	}
	if err2 != nil {
		errorMsgs = append(errorMsgs, fmt.Sprintf("marshal error: %v", err2))
	}
	if err3 != nil {
		errorMsgs = append(errorMsgs, fmt.Sprintf("remarshal error: %v", err3))
	}
	return fmt.Errorf("failed to process instruction: %s",
		strings.Join(errorMsgs, "; "),
	)
}
