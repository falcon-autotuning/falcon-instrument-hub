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

	// Performance monitoring
	stats   LoggerStats
	statsMu sync.RWMutex
}

// LoggerStats tracks logger performance metrics
type LoggerStats struct {
	TotalEnqueued   int64
	TotalDropped    int64
	TotalBatches    int64
	TotalEntries    int64
	QueueLength     int
	LastBatchTime   time.Time
	LastBatchSize   int
	QueueFullEvents int64
}

// LogEntry represents a buffered log entry with pre-captured timestamp
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Source    string
	Message   string
	Channel   string
}

// TODO: generalize this pageSize ot get it from the OS directly instead of
// asserting
const (
	TimeFormat = "2006-01-02 15:04:05.000000"
	pageSize   = 4096 // OS page size
)

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
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("PANIC in asyncWriter: %v\n", r)
			}
		}()
		logger.asyncWriter()
	}()

	// Write initial log entry
	logger.Info("SYSTEM", "Logger initialized")
	logger.Info("SYSTEM", fmt.Sprintf("Log file: %s", filePath))

	return logger, nil
}

// asyncWriter processes log entries from the queue
func (l *Logger) asyncWriter() {
	defer l.wg.Done()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var stringBuilder strings.Builder
	stringBuilder.Grow(pageSize * 4) // Pre-allocate buffer for efficiency

	fmt.Printf(
		"[ASYNC_DEBUG] AsyncWriter started at %s\n",
		time.Now().Format(TimeFormat),
	)

	tickerCount := 0
	for {
		select {
		case <-ticker.C:
			tickerCount++
			fmt.Printf("[ASYNC_DEBUG] Ticker fired #%d at %s, queue len: %d\n",
				tickerCount, time.Now().Format(TimeFormat), len(l.logQueue))

			// Every 50ms, drain the entire queue and write to file
			l.drainQueueAndWrite(&stringBuilder, pageSize)

			fmt.Printf(
				"[ASYNC_DEBUG] Completed draining queue, processed batch #%d\n",
				tickerCount,
			)

		case <-l.done:
			fmt.Printf("[ASYNC_DEBUG] Shutdown signal received\n")
			// Shutdown signal - drain everything and exit
			l.drainQueueAndWrite(&stringBuilder, pageSize)

			// Write any remaining content in buffer
			if stringBuilder.Len() > 0 {
				l.writeStringToFile(stringBuilder.String())
			}
			fmt.Printf("[ASYNC_DEBUG] AsyncWriter exiting\n")
			return
		}
	}
}

// drainQueueAndWrite empties the entire logQueue and writes to file in
// page-sized chunks
func (l *Logger) drainQueueAndWrite(builder *strings.Builder, pageSize int) {
	entriesProcessed := 0
	writeCount := 0

	fmt.Printf(
		"[DRAIN_DEBUG] Starting to drain queue, current len: %d\n",
		len(l.logQueue),
	)

	// Drain the entire queue
	for {
		select {
		case entry, ok := <-l.logQueue:
			if !ok {
				fmt.Printf(
					"[DRAIN_DEBUG] Channel closed, processed %d entries\n",
					entriesProcessed,
				)
				// Channel closed
				if builder.Len() > 0 {
					l.writeStringToFile(builder.String())
					writeCount++
					builder.Reset()
				}
				return
			}

			// Format the log entry and add to string builder
			logLine := l.formatLogEntry(entry)
			builder.WriteString(logLine)
			entriesProcessed++

			// If we've accumulated enough for a page, write it
			if builder.Len() >= pageSize {
				fmt.Printf(
					"[DRAIN_DEBUG] Writing chunk #%d (size: %d bytes)\n",
					writeCount+1,
					builder.Len(),
				)
				l.writeStringToFile(builder.String())
				writeCount++
				builder.Reset()
			}

		default:
			// No more entries in queue, break
			fmt.Printf(
				"[DRAIN_DEBUG] Queue empty, processed %d entries, made %d writes\n",
				entriesProcessed,
				writeCount,
			)
			goto escapeNoEntries
		}
	}
escapeNoEntries:

	// Write any remaining content that's less than a page
	if builder.Len() > 0 {
		fmt.Printf("[DRAIN_DEBUG] Writing final chunk (size: %d bytes)\n", builder.Len())
		l.writeStringToFile(builder.String())
		writeCount++
		builder.Reset()
	}

	// Update stats
	l.updateBatchStats(entriesProcessed)
	fmt.Printf(
		"[DRAIN_DEBUG] Updated stats: %d entries processed\n",
		entriesProcessed,
	)
}

// formatLogEntry formats a single log entry into a string with newline
func (l *Logger) formatLogEntry(entry LogEntry) string {
	// Handle raw entries differently - just write the message as-is
	if entry.Level == "RAW" && entry.Source == "RAW" {
		logLine := entry.Message
		// Ensure raw messages end with newline if they don't already
		if !strings.HasSuffix(logLine, "\n") {
			logLine += "\n"
		}
		return logLine
	}

	// Format structured log entries
	if entry.Channel != "" {
		return fmt.Sprintf("[%s] [%s] [%s] [%s] %s\n",
			entry.Timestamp.Format(TimeFormat),
			entry.Level,
			entry.Source,
			entry.Channel,
			entry.Message,
		)
	} else {
		return fmt.Sprintf("[%s] [%s] [%s] %s\n",
			entry.Timestamp.Format(TimeFormat),
			entry.Level,
			entry.Source,
			entry.Message,
		)
	}
}

// writeStringToFile writes a string to the log file and syncs
func (l *Logger) writeStringToFile(content string) {
	fmt.Printf("[WRITE_DEBUG] Starting write of %d bytes\n", len(content))

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		fmt.Printf("[WRITE_DEBUG] File is nil, aborting write\n")
		return
	}

	// Write the entire string at once
	if _, err := l.file.WriteString(content); err != nil {
		fmt.Printf("Error writing to log file: %v\n", err)
		return
	}

	// Also print to stdout for debugging
	fmt.Print(content)

	// Sync to disk
	if err := l.file.Sync(); err != nil {
		fmt.Printf("Error syncing log file: %v\n", err)
	}

	fmt.Printf("[WRITE_DEBUG] Completed write and sync\n")
}

// updateBatchStats updates statistics for batch processing
func (l *Logger) updateBatchStats(entriesProcessed int) {
	if entriesProcessed == 0 {
		return
	}

	l.statsMu.Lock()
	defer l.statsMu.Unlock()

	l.stats.TotalBatches++
	l.stats.TotalEntries += int64(entriesProcessed)
	l.stats.LastBatchTime = time.Now()
	l.stats.LastBatchSize = entriesProcessed
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
		l.statsMu.Lock()
		l.stats.TotalEnqueued++
		l.statsMu.Unlock()
	default:
		// Queue full, drop the log entry to avoid blocking
		l.statsMu.Lock()
		l.stats.TotalDropped++
		l.stats.QueueFullEvents++
		queueLen := len(l.logQueue)
		l.statsMu.Unlock()

		// Enhanced logging with queue diagnostics
		fmt.Printf(
			"LOG QUEUE FULL (len=%d): [%s] [%s] [%s] %s\n",
			queueLen,
			entry.Timestamp.Format(TimeFormat),
			level,
			source,
			message,
		)
		// Also check if asyncWriter is still alive
		fmt.Printf(
			"QUEUE STATS: Cap=%d, Len=%d, Enqueued=%d, Batches=%d\n",
			cap(
				l.logQueue,
			),
			len(l.logQueue),
			l.stats.TotalEnqueued,
			l.stats.TotalBatches,
		)
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

// GetStats returns current logger performance statistics
func (l *Logger) GetStats() LoggerStats {
	l.statsMu.RLock()
	defer l.statsMu.RUnlock()

	stats := l.stats
	stats.QueueLength = len(l.logQueue)
	return stats
}

// LogStats writes current performance statistics to the log
func (l *Logger) LogStats() {
	stats := l.GetStats()
	l.Info("LOGGER_STATS", fmt.Sprintf(
		"Enqueued: %d, Dropped: %d, Batches: %d, Entries: %d, QueueLen: %d, QueueFull: %d",
		stats.TotalEnqueued,
		stats.TotalDropped,
		stats.TotalBatches,
		stats.TotalEntries,
		stats.QueueLength,
		stats.QueueFullEvents,
	))
}

// GetLogPath returns the current log file path
func (l *Logger) GetLogPath() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.filePath
}
