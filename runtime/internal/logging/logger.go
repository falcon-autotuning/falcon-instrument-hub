package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger handles global logging for the runtime
type Logger struct {
	mu       sync.RWMutex
	file     *os.File
	filePath string
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
	Channel   string    `json:"channel,omitempty"`
}

// NewLogger creates a new logger instance
func NewLogger(outputPath string) (*Logger, error) {
	// Create logs directory if it doesn't exist
	logsDir := filepath.Join(outputPath, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("falcon-runtime_%s.log", timestamp)
	filePath := filepath.Join(logsDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	logger := &Logger{
		file:     file,
		filePath: filePath,
	}

	// Write initial log entry
	logger.Info("SYSTEM", "Logger initialized")
	logger.Info("SYSTEM", fmt.Sprintf("Log file: %s", filePath))

	return logger, nil
}

// Log writes a log entry to the file
func (l *Logger) Log(level, source, message string) {
	l.LogWithChannel(level, source, message, "")
}

// LogWithChannel writes a log entry with channel information
func (l *Logger) LogWithChannel(level, source, message, channel string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now()

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

	// Write to file
	if l.file != nil {
		if _, err := l.file.WriteString(logLine); err != nil {
			fmt.Printf("Error writing to log file: %v\n", err)
		}
		if err := l.file.Sync(); err != nil {
			fmt.Printf("Error syncing log file: %v\n", err)
		}
	}

	// Also print to stdout for debugging
	fmt.Print(logLine)
}

// WriteRaw writes a raw log line to the file (used for custom timestamps)
func (l *Logger) WriteRaw(logLine string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Write to file
	if l.file != nil {
		l.file.WriteString(logLine)
		l.file.Sync() // Force write to disk
	}

	// Also print to stdout for debugging
	fmt.Print(logLine)
}

// Info logs an info message
func (l *Logger) Info(source, message string) {
	l.Log("INFO", source, message)
}

// Debug logs a debug message
func (l *Logger) Debug(source, message string) {
	l.Log("DEBUG", source, message)
}

// Warn logs a warning message
func (l *Logger) Warn(source, message string) {
	l.Log("WARN", source, message)
}

// Error logs an error message
func (l *Logger) Error(source, message string) {
	l.Log("ERROR", source, message)
}

// LogNATSMessage logs a message received from NATS
func (l *Logger) LogNATSMessage(channel, message string) {
	l.LogWithChannel("NATS", "MESSAGE", message, channel)
}

// Close closes the log file
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		// Write shutdown message directly without using the logger methods to avoid recursion
		shutdownMsg := fmt.Sprintf("[%s] [%s] [%s] %s\n",
			time.Now().Format("2006-01-02 15:04:05.000"),
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
