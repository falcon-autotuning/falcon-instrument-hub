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
	scheduler, err := h.selectScheduler(measurementID)
	if err != nil {
		h.mutex.Unlock()
		h.log.Error("Error selecting scheduler: %v", err)
		return
	}
	// Update ready checklist for the specific scheduler
	if !scheduler.containsGetter(jsonPort) {
		h.mutex.Unlock()
		h.log.Error("Port %s not found in getters %v for %+v",
			port,
			scheduler.GetterDeployment.GetPorts(),
			measurementID,
		)
		return
	}
	scheduler.storeData(jsonPort, returnData.Data)
	if !scheduler.allDataHere() {
		h.mutex.Unlock()
		h.log.Debug("Stored result for port %s, %+v (%d/%d received)",
			port,
			measurementID,
			scheduler.ReceivedReturns,
			scheduler.ExpectedReturns,
		)
		return
	}
	results := scheduler.Results
	// Clean up the scheduler
	schedulerMap := h.schedulers[measurementID.ProcessId]
	delete(schedulerMap, measurementID.ChunkId)
	if len(schedulerMap) == 0 {
		delete(h.schedulers, measurementID.ProcessId)
	}
	h.mutex.Unlock()

	h.log.Debug("Stored result for port %s, %+v (%d/%d received)",
		port,
		measurementID,
		scheduler.ReceivedReturns,
		scheduler.ExpectedReturns,
	)
	h.sendProcessData(measurementID, results)

	h.markMeasurementComplete()
	h.logger.LogStats()
}

// sendProcessDataForBuffered sends the collected data as PROCESS_DATA
func (h *MeasurementReadyHandler) sendProcessData(
	measurementID instrument.MeasurementID,
	results map[instrument.JsonPort]any,
) {
	dataBytes, err := json.Marshal(results)
	if err != nil {
		h.log.Error("Failed to marshal results for %+v: %v",
			measurementID,
			err,
		)
		return
	}
	processData := api.ProcessData{
		Data:      string(dataBytes),
		ProcessId: int64(measurementID.ProcessId),
		ChunkId:   int64(measurementID.ChunkId),
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
	h.log.Info("Sent %s for measurement %+v with %d results",
		ProcessDataMessage,
		measurementID,
		len(results),
	)
}
