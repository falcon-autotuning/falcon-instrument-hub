package measure

import (
	"encoding/json"
	"strings"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
)

// handleArmed processes ARMED messages from instruments
func (h *MeasurementReadyHandler) handleArmed(msg *nats.Msg) {
	var armed api.Armed
	h.log.Debug("Received %s", ArmedMessage)
	if err := json.Unmarshal(msg.Data, &armed); err != nil {
		h.log.Error("Failed to unmarshal %s: %v", ArmedMessage, err)
		return
	}

	measurementID := instrument.MeasurementID{
		ProcessId: instrument.ID(armed.ProcessId),
		ChunkId:   instrument.ID(armed.ChunkId),
	}
	if measurementID.ProcessId == 0 {
		h.log.Error("ProcessId not found in %s message", ArmedMessage)
		return
	}

	if measurementID.ChunkId == 0 {
		h.log.Error("ChunkId not found in %s message", ArmedMessage)
		return
	}

	// Subject format: ARMED.<instrument_name>
	subjectParts := strings.Split(msg.Subject, ".")
	if len(subjectParts) < 2 {
		h.log.Error("Invalid %s subject format: %s", ArmedMessage, msg.Subject)
		return
	}
	instrumentName := instrument.Name(subjectParts[1])
	h.log.Debug("Processing %s from instrument: %s for %+v",
		ArmedMessage,
		instrumentName,
		measurementID,
	)

	// Update ready checklist for the specific scheduler
	scheduler, err := h.selectScheduler(measurementID)
	if err != nil {
		h.log.Error("Error selecting scheduler: %v", err)
		return
	}

	h.mutex.Lock()
	err = scheduler.registerReadyRequirement(instrumentName)
	if err != nil {
		h.mutex.Unlock()
		h.log.Error("error registering ready ports: %v", err.Error())
		return
	}
	if !scheduler.requirementsAreSatisfied() {
		h.mutex.Unlock()
		h.log.Debug("Marked instrument %s as ready for %+v",
			instrumentName,
			measurementID,
		)
		h.log.Debug("Requirements are not satisfied yet")
		return
	}
	scheduler.resetRequiredReadiness()
	h.mutex.Unlock()

	getters := scheduler.GetterInstruments()
	h.log.Debug("Marked instrument %s as ready for %+v",
		instrumentName,
		measurementID,
	)
	h.log.Info("All setter instruments marked as ready for %+v", measurementID)
	h.log.Debug("Reset ready checklist for %+v", measurementID)
	go h.triggerGetterInstruments(measurementID, getters)
}
