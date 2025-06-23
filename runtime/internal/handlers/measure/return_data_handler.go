package measure

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/handlers/instrument"
)

// handleReturnData processes RETURN_DATA responses from buffered measurements
func (h *MeasurementReadyHandler) handleReturnData(msg *nats.Msg) {
	var returnData api.ReturnData
	h.log.Debug("Received %s", ReturnDataMessage)
	if err := json.Unmarshal(msg.Data, &returnData); err != nil {
		h.log.Error("Failed to unmarshal %s: %v", ReturnDataMessage, err)
		return
	}

	// Extract instrument name from subject (RETURN_DATA.<instrument_name>)
	subjectParts := strings.Split(msg.Subject, ".")
	if len(subjectParts) < 2 {
		h.log.Error(
			"Invalid %s subject format: %s",
			ReturnDataMessage,
			msg.Subject,
		)
		return
	}
	instrumentName := instrument.Name(subjectParts[1])
	measurementID := instrument.MeasurementID{
		ProcessId: instrument.ID(returnData.ProcessId),
		ChunkId:   instrument.ID(returnData.ChunkId),
	}
	if measurementID.ProcessId == 0 {
		h.log.Error("ProcessId not found in %s message", ReturnDataMessage)
		return
	}

	if measurementID.ChunkId == 0 {
		h.log.Error("ChunkId not found in %s message", ReturnDataMessage)
		return
	}

	h.log.Debug(
		"Processing %s: property=%s, index=%d, measurementID=%+v",
		ReturnDataMessage,
		returnData.Property,
		returnData.Index,
		measurementID,
	)

	// Find the port directly using instrument name, property, and index
	jsonPort, err := h.instrumentHandler.FindPortByInstrumentPropertyIndex(
		instrumentName,
		instrument.PropertyName(returnData.Property),
		instrument.Index(strconv.FormatInt(returnData.Index, 10)),
	)
	if err != nil {
		h.log.Error(
			"Failed to find port for %s (property: %s, index: %d, measurementID: %+v): %v",
			ReturnDataMessage,
			returnData.Property,
			returnData.Index,
			measurementID,
			err,
		)
		return
	}
	port := instrument.PortObject{}
	err = port.FromInterface(jsonPort)
	if err != nil {
		h.log.Error(
			"Failed to convert port to object for %s",
			jsonPort,
		)
	}

	h.log.Debug(
		"Found port '%s' for %s (property: %s, index: %d, measurementID: %+v)",
		port.DefaultName,
		ReturnDataMessage,
		returnData.Property,
		returnData.Index,
		measurementID,
	)

	// Find the scheduler for this measurement
	h.mutex.Lock()
	defer h.mutex.Unlock()

	schedulerMap, exists := h.schedulers[measurementID.ProcessId]
	if !exists {
		h.log.Error("No scheduler map found for ProcessId %d",
			measurementID.ProcessId,
		)
		return
	}

	scheduler, exists := schedulerMap[measurementID.ChunkId]
	if !exists {
		h.log.Error("No scheduler found for %+v", measurementID)
		return
	}
	if !scheduler.containsGetter(jsonPort) {
		h.log.Error("Port %s not found in getters %v for %+v",
			port,
			scheduler.GetterDeployment.GetPorts(),
			measurementID,
		)
		return
	}
	scheduler.storeData(jsonPort, returnData.Data)
	h.log.Debug("Stored result for port %s, %+v (%d/%d received)",
		port,
		measurementID,
		scheduler.ReceivedReturns,
		scheduler.ExpectedReturns,
	)
	if !scheduler.allDataHere() {
		return
	}
	h.sendProcessDataForBuffered(measurementID, scheduler.Results)
}

// sendProcessDataForBuffered sends the collected buffered data as PROCESS_DATA
func (h *MeasurementReadyHandler) sendProcessDataForBuffered(
	measurementID instrument.MeasurementID,
	results map[instrument.JsonPort]any,
) {
	schedulerMap, exists := h.schedulers[measurementID.ProcessId]
	if !exists {
		h.log.Error("No scheduler map found for ProcessId %d",
			measurementID.ProcessId,
		)
		return
	}

	dataBytes, err := json.Marshal(results)
	if err != nil {
		h.log.Error("Failed to marshal buffered results for %+v: %v",
			measurementID,
			err,
		)
		return
	}
	processData := api.ProcessData{
		Data:      string(dataBytes),
		ProcessId: int64(measurementID.ProcessId),
		Timestamp: time.Now().UnixMicro(),
	}
	processDataBytes, err := json.Marshal(processData)
	if err != nil {
		h.log.Error("Failed to marshal %s for %+v: %v",
			ProcessDataMessage,
			measurementID,
			err,
		)
		return
	}

	if err := h.nc.Publish(ProcessDataMessage, processDataBytes); err != nil {
		h.log.Error("Failed to publish %s for %+v: %v",
			ProcessDataMessage,
			measurementID,
			err,
		)
		return
	}
	h.log.Info("Sent %s for buffered measurement %+v with %d results",
		ProcessDataMessage,
		measurementID,
		len(results),
	)

	// Clean up the scheduler
	delete(schedulerMap, measurementID.ChunkId)
	if len(schedulerMap) == 0 {
		delete(h.schedulers, measurementID.ProcessId)
	}
	h.markMeasurementComplete()
}
