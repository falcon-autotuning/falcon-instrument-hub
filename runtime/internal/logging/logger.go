package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Logger handles global logging for the runtime
type Logger struct {
	mu       sync.RWMutex
	file     *os.File
	filePath string
	logQueue chan LogEntry
	done     chan struct{}
	wg       sync.WaitGroup
}

// LogEntry represents a buffered log entry with pre-captured timestamp
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Source    string
	Message   string
	Channel   string
}

const TimeFormat = "2006-01-02 15:04:05.000000"

// NewLogger creates a new logger instance
func NewLogger(outputPath string) (*Logger, error) {
	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("falcon-runtime_%s.log", timestamp)
	filePath := filepath.Join(outputPath, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	logger := &Logger{
		file:     file,
		filePath: filePath,
		logQueue: make(chan LogEntry, 10000), // Large buffer for async logging
		done:     make(chan struct{}),
	}

	// Start async writer goroutine
	logger.wg.Add(1)
	go logger.asyncWriter()

	// Write initial log entry
	logger.Info("SYSTEM", "Logger initialized")
	logger.Info("SYSTEM", fmt.Sprintf("Log file: %s", filePath))

	return logger, nil
}

// asyncWriter processes log entries from the queue
func (l *Logger) asyncWriter() {
	defer l.wg.Done()

	// Batch writes for better performance
	batch := make([]LogEntry, 0, 100)
	ticker := time.NewTicker(50 * time.Millisecond) // Flush every 50ms
	defer ticker.Stop()

	for {
		select {
		case entry, ok := <-l.logQueue:
			if !ok {
				// Channel closed, flush remaining and exit
				if len(batch) > 0 {
					l.flushBatch(batch)
				}
				return
			}

			batch = append(batch, entry)

			// Flush batch when it gets large
			if len(batch) >= 100 {
				l.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// Periodic flush even if batch isn't full
			if len(batch) > 0 {
				l.flushBatch(batch)
				batch = batch[:0]
			}

		case <-l.done:
			// Shutdown signal received - drain queue and exit
			if len(batch) > 0 {
				l.flushBatch(batch)
			}

			// Drain remaining queue
			for {
				select {
				case entry := <-l.logQueue:
					l.writeEntry(entry)
				default:
					return
				}
			}
		}
	}
}

// flushBatch writes a batch of log entries
func (l *Logger) flushBatch(batch []LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	for _, entry := range batch {
		l.writeEntryUnsafe(entry)
	}

	// Sync to disk after batch
	if err := l.file.Sync(); err != nil {
		fmt.Printf("Error syncing log file: %v\n", err)
	}
}

// writeEntry writes a single entry (used during shutdown)
func (l *Logger) writeEntry(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	l.writeEntryUnsafe(entry)
	l.file.Sync()
}

// writeEntryUnsafe writes without acquiring mutex (caller must hold mutex)
func (l *Logger) writeEntryUnsafe(entry LogEntry) {
	var logLine string

	// Handle raw entries differently - just write the message as-is
	if entry.Level == "RAW" && entry.Source == "RAW" {
		logLine = entry.Message
		// Ensure raw messages end with newline if they don't already
		if !strings.HasSuffix(logLine, "\n") {
			logLine += "\n"
		}
	} else {
		// Format structured log entries
		if entry.Channel != "" {
			logLine = fmt.Sprintf("[%s] [%s] [%s] [%s] %s\n",
				entry.Timestamp.Format(TimeFormat),
				entry.Level,
				entry.Source,
				entry.Channel,
				entry.Message,
			)
		} else {
			logLine = fmt.Sprintf("[%s] [%s] [%s] %s\n",
				entry.Timestamp.Format(TimeFormat),
				entry.Level,
				entry.Source,
				entry.Message,
			)
		}
	}

	if _, err := l.file.WriteString(logLine); err != nil {
		fmt.Printf("Error writing to log file: %v\n", err)
	}

	// Also print to stdout for debugging
	fmt.Print(logLine)
}

// queueLogEntry adds a log entry to the async queue (non-blocking for callers)
func (l *Logger) queueLogEntry(level, source, message, channel string) {
	entry := LogEntry{
		Timestamp: time.Now(), // Capture timestamp immediately
		Level:     level,
		Source:    source,
		Message:   message,
		Channel:   channel,
	}

	select {
	case l.logQueue <- entry:
		// Successfully queued
	default:
		// Queue full, drop the log entry to avoid blocking
		// Could optionally write directly to stdout as fallback
		fmt.Printf("LOG QUEUE FULL: [%s] [%s] [%s] %s\n",
			entry.Timestamp.Format(TimeFormat), level, source, message)
	}
}

// Log writes a log entry to the queue (non-blocking)
func (l *Logger) Log(level, source, message string) {
	l.queueLogEntry(level, source, message, "")
}

// LogWithChannel writes a log entry with channel information (non-blocking)
func (l *Logger) LogWithChannel(level, source, message, channel string) {
	l.queueLogEntry(level, source, message, channel)
}

// WriteRaw writes a raw log line to the queue
func (l *Logger) WriteRaw(logLine string) {
	// For raw logs, we want to preserve the original format exactly
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     "RAW",
		Source:    "RAW",
		Message:   logLine,
		Channel:   "",
	}

	select {
	case l.logQueue <- entry:
	default:
		// Fallback: print directly (preserve raw format)
		fmt.Print(logLine)
		if !strings.HasSuffix(logLine, "\n") {
			fmt.Print("\n")
		}
	}
}

// Info logs an info message (non-blocking)
func (l *Logger) Info(source, message string) {
	l.Log("INFO", source, message)
}

// Debug logs a debug message (non-blocking)
func (l *Logger) Debug(source, message string) {
	l.Log("DEBUG", source, message)
}

// Warn logs a warning message (non-blocking)
func (l *Logger) Warn(source, message string) {
	l.Log("WARN", source, message)
}

// Error logs an error message (non-blocking)
func (l *Logger) Error(source, message string) {
	l.Log("ERROR", source, message)
}

// LogNATSMessage logs a message received from NATS (non-blocking)
func (l *Logger) LogNATSMessage(channel, message string) {
	l.LogWithChannel("NATS", "MESSAGE", message, channel)
}

// Close closes the log file and shuts down async writer
func (l *Logger) Close() error {
	// Signal shutdown to async writer
	select {
	case <-l.done:
		// Already closed
	default:
		close(l.done)
	}

	// Wait for writer to finish
	l.wg.Wait()

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		// Write shutdown message directly
		shutdownMsg := fmt.Sprintf("[%s] [%s] [%s] %s\n",
			time.Now().Format(TimeFormat),
			"INFO",
			"SYSTEM",
			"Logger shutting down")
		l.file.WriteString(shutdownMsg)
		l.file.Sync()

		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

// GetLogPath returns the current log file path
func (l *Logger) GetLogPath() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.filePath
}
