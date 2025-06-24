package measure

import (
	"encoding/json"
	"strings"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
)

// handleExecuting processes EXECUTING messages from instruments
func (h *MeasurementReadyHandler) handleExecuting(msg *nats.Msg) {
	var executing api.Executing
	h.log.Debug("Received %s: %s", ExecutingMessage, string(msg.Data))
	if err := json.Unmarshal(msg.Data, &executing); err != nil {
		h.log.Error("Failed to unmarshal %s: %v", ExecutingMessage, err)
		return
	}

	measurementID := instrument.MeasurementID{
		ProcessId: instrument.ID(executing.ProcessId),
		ChunkId:   instrument.ID(executing.ChunkId),
	}
	if measurementID.ProcessId == 0 {
		h.log.Error("ProcessId not found in EXECUTING message")
		return
	}

	if measurementID.ChunkId == 0 {
		h.log.Error("ChunkId not found in EXECUTING message")
		return
	}

	// Subject format: "EXECUTING.<instrument_name>"
	subjectParts := strings.Split(msg.Subject, ".")
	if len(subjectParts) < 2 {
		h.log.Error("Invalid EXECUTING subject format: %s", msg.Subject)
		return
	}
	instrumentName := instrument.Name(subjectParts[1])

	h.log.Debug("Processing EXECUTING from instrument: %s for %+v",
		instrumentName,
		measurementID,
	)

	scheduler, err := h.selectScheduler(measurementID)
	if err != nil {
		h.log.Error("Error selecting scheduler: %v", err)
		return
	}
	if scheduler.SetterDeployment.Contains(instrumentName) {
		h.log.Info(
			"instrument %s has been triggered and is running for %+v",
			instrumentName,
			measurementID,
		)
		return
	}
	if !scheduler.GetterDeployment.Contains(instrumentName) {
		h.log.Error("Instrument %s not found in getter instruments for %+v",
			instrumentName,
			measurementID,
		)
		return
	}
	h.mutex.Lock()
	scheduler.TriggeredGetterChecklist[instrumentName] = true
	if !scheduler.gettersAreTriggered() {
		h.mutex.Unlock()
		h.log.Debug(
			"Marked getter instrument %s as triggered for %+v but the instrument was not ready to be triggered",
			instrumentName,
			measurementID,
		)
		return
	}
	scheduler.resetGettersTriggered()
	h.mutex.Unlock()
	h.log.Debug("Marked getter instrument %s as triggered for %+v",
		instrumentName,
		measurementID,
	)
	h.log.Debug("Reset triggered getter checklist for %+v", measurementID)
	go h.handleAllGettersTriggered(
		measurementID,
		scheduler.MasterTriggerInstruments,
	)
}
