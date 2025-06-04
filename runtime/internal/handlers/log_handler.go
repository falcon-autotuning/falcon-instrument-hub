package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/api"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	LogHandlerName = "LOG_HANDLER"
)

// ToTime converts the timestamp to a Go time.Time
func ToTime(l api.Log) time.Time {
	return api.ToTime(l)
}

// LogHandler handles LOG.* NATS messages
type LogHandler struct {
	logger       *logging.Logger
	subscription *nats.Subscription
}

// NewLogHandler creates a new log handler
func NewLogHandler(logger *logging.Logger) *LogHandler {
	return &LogHandler{
		logger: logger,
	}
}

// Subscribe subscribes to LOG.* channels and starts handling messages
func (h *LogHandler) Subscribe(nc *nats.Conn) error {
	// Subscribe to all LOG.* channels
	sub, err := nc.Subscribe("LOG.>", h.handleLogMessage)
	if err != nil {
		return fmt.Errorf("failed to subscribe to LOG.*: %w", err)
	}

	h.subscription = sub
	h.logger.Info(LogHandlerName, "Subscribed to LOG.> channels")
	log.Printf("LOG handler subscribed to LOG.* channels")

	return nil
}

// Unsubscribe unsubscribes from LOG.* channels
func (h *LogHandler) Unsubscribe() error {
	if h.subscription != nil {
		err := h.subscription.Unsubscribe()
		if err != nil {
			h.logger.Error(
				LogHandlerName,
				fmt.Sprintf("Failed to unsubscribe: %v", err),
			)
			return err
		}
		h.logger.Info(LogHandlerName, "Unsubscribed from LOG.* channels")
		h.subscription = nil
	}
	return nil
}

// handleLogMessage processes incoming LOG messages
func (h *LogHandler) handleLogMessage(msg *nats.Msg) {
	channel := msg.Subject
	rawData := msg.Data

	// Debug: Log message reception
	h.logger.Debug(
		LogHandlerName,
		fmt.Sprintf("Received message on %s: %s", channel, string(rawData)),
	)

	// Try to decode as JSON first (structured log message)
	var apiLog api.Log
	if err := json.Unmarshal(rawData, &apiLog); err != nil {
		// If JSON decoding fails, treat as plain text message
		h.logger.Debug(
			LogHandlerName,
			fmt.Sprintf("Treating as plain text (JSON decode failed: %v)", err),
		)
		h.handlePlainTextLog(channel, string(rawData))
		return
	}

	// Handle structured log message
	h.logger.Debug(LogHandlerName, "Handling structured log")
	h.handleStructuredLog(channel, &apiLog)

	// Optional: Reply if the message expects a response
	if msg.Reply != "" {
		response := fmt.Sprintf("LOG received: %s", channel)
		if err := msg.Respond([]byte(response)); err != nil {
			h.logger.Error(
				LogHandlerName,
				fmt.Sprintf("Failed to respond to %s: %v", channel, err),
			)
		}
	}
}

// handleStructuredLog processes a structured log message with timestamp and
// optional hash
func (h *LogHandler) handleStructuredLog(channel string, apiLog *api.Log) {
	// Get timestamp (use provided timestamp or current time)
	timestamp := api.ToTime(apiLog)

	// Get hash if present
	hash := apiLog.Hash

	// Extract log level from channel if possible
	level := h.extractLogLevel(channel)

	// Create formatted message with hash if present
	formattedMessage := fmt.Sprintf(
		"[HASH:%s] %s",
		strconv.FormatInt(hash, 10),
		apiLog.Message,
	)

	// Log with custom timestamp
	h.logWithTimestamp(level, "EXTERNAL", formattedMessage, channel, timestamp)

	// Also log to NATS message log
	h.logger.LogNATSMessage(channel, fmt.Sprintf("JSON: %s", apiLog.Message))
}

// handlePlainTextLog processes a plain text log message (backward
// compatibility)
func (h *LogHandler) handlePlainTextLog(channel, message string) {
	// Extract log level from channel
	level := h.extractLogLevel(channel)

	// Log with current timestamp
	h.logger.LogWithChannel(level, "EXTERNAL", message, channel)

	// Also log to NATS message log
	h.logger.LogNATSMessage(channel, fmt.Sprintf("TEXT: %s", message))
}

// extractLogLevel extracts log level from channel name
func (h *LogHandler) extractLogLevel(channel string) string {
	// LOG.INFO, LOG.DEBUG, LOG.WARN, LOG.ERROR
	if len(channel) > 4 { // "LOG." is 4 characters
		channelLevel := channel[4:] // everything after "LOG."
		switch channelLevel {
		case "DEBUG":
			return "DEBUG"
		case "WARN":
			return "WARN"
		case "ERROR":
			return "ERROR"
		case "INFO":
			return "INFO"
		default:
			// For custom channels like LOG.DEVICE.SENSOR1, use INFO level
			return "INFO"
		}
	}
	return "INFO" // default
}

// logWithTimestamp logs a message with a specific timestamp
func (h *LogHandler) logWithTimestamp(
	level, source, message, channel string,
	timestamp time.Time,
) {
	// Format: [TIMESTAMP] [LEVEL] [SOURCE] [CHANNEL] MESSAGE
	var logLine string
	if channel != "" {
		logLine = fmt.Sprintf("[%s] [%s] [%s] [%s] %s\n",
			timestamp.Format("2006-01-02 15:04:05.000"),
			level,
			source,
			channel,
			message,
		)
	} else {
		logLine = fmt.Sprintf("[%s] [%s] [%s] %s\n",
			timestamp.Format("2006-01-02 15:04:05.000"),
			level,
			source,
			message,
		)
	}

	// Write to logger's file directly
	h.logger.WriteRaw(logLine)
}

// GetSubscription returns the current subscription (for testing)
func (h *LogHandler) GetSubscription() *nats.Subscription {
	return h.subscription
}
