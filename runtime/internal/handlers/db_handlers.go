package handlers

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
)

func sendSimpleResponse(msg *nats.Msg, success bool, errMsg string) {
	resp := api.SimpleResponse{
		Success: success,
		Error:   errMsg,
	}
	respBytes, err := json.Marshal(resp)
	if err != nil {
		log.Printf("Error marshaling simple response: %v", err)
		// Fallback response if marshaling fails
		fallbackResp := `{"success":false,"error":"internal server error"}`
		if success {
			fallbackResp = `{"success":true}`
		}
		msg.Respond([]byte(fallbackResp))
		return
	}
	msg.Respond(respBytes)
}

// Helper to send a generic JSON response
func sendJsonResponse(msg *nats.Msg, data interface{}) {
	respBytes, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling JSON response for subject %s: %v", msg.Subject, err)
		sendSimpleResponse(msg, false, fmt.Sprintf("internal server error: failed to marshal response: %v", err))
		return
	}
	msg.Respond(respBytes)
}

// handleBulkAccess - Placeholder, needs db.GetAllCharacteristics() or similar
func handleBulkAccess(db *database.DB, msg *nats.Msg) {
	var cmd api.BulkDatabaseAccessCommand
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		log.Printf("Error unmarshaling BULK_DATABASE_ACCESS command: %v, Data: %s", err, string(msg.Data))
		sendSimpleResponse(msg, false, fmt.Sprintf("invalid BULK_DATABASE_ACCESS payload: %v", err))
		return
	}

	log.Printf("Received BULK_DATABASE_ACCESS command (Hash: %s). Fetching all characteristics (Not Implemented Yet).", cmd.Hash)
	// TODO: Implement db.GetAllCharacteristics() and map results to comms.BulkDatabaseReceiveResponse
	// For now, send an empty collection or an error
	resp := api.BulkDatabaseReceiveResponse{
		UnitCommandFields: cmd.UnitCommandFields, // Echo back request fields
		Collection:        []api.DatabaseCharacteristicPayload{},
	}
	log.Printf("BULK_DATABASE_ACCESS: Returning empty collection (implementation pending).")
	sendJsonResponse(msg, resp)
	// OR: sendSimpleResponse(msg, false, "BULK_DATABASE_ACCESS not fully implemented")
}
