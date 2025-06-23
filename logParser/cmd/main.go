package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// LogEntry represents a single log entry with timestamp and content
type LogEntry struct {
	Timestamp time.Time
	Content   string
}

// LogParser handles parsing of falcon runtime logs
type LogParser struct {
	timestampRegex *regexp.Regexp
}

// NewLogParser creates a new log parser instance
func NewLogParser() *LogParser {
	// Regex to match timestamp pattern: [2025-06-23 11:57:15.123]
	timestampRegex := regexp.MustCompile(
		`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3})\]`,
	)

	return &LogParser{
		timestampRegex: timestampRegex,
	}
}

// ParseFile parses a log file and returns sorted log entries
func (p *LogParser) ParseFile(filename string) ([]LogEntry, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var entries []LogEntry
	var currentEntry *LogEntry

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()

		// Check if line starts with a timestamp
		matches := p.timestampRegex.FindStringSubmatch(line)

		if len(matches) == 2 {
			// New log entry with timestamp
			if currentEntry != nil {
				// Save previous entry
				entries = append(entries, *currentEntry)
			}

			// Parse timestamp
			timestamp, err := time.Parse("2006-01-02 15:04:05.000", matches[1])
			if err != nil {
				return nil, fmt.Errorf(
					"failed to parse timestamp %s: %w",
					matches[1],
					err,
				)
			}

			// Create new entry
			currentEntry = &LogEntry{
				Timestamp: timestamp,
				Content:   line,
			}
		} else {
			// Continuation line - append to current entry
			if currentEntry != nil {
				currentEntry.Content += "\n" + line
			} else {
				// Handle case where file doesn't start with timestamp
				log.Printf("Warning: found line without timestamp: %s", line)
			}
		}
	}

	// Don't forget the last entry
	if currentEntry != nil {
		entries = append(entries, *currentEntry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// Sort entries by timestamp
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return entries, nil
}

// WriteToFile writes sorted log entries to a file
func (p *LogParser) WriteToFile(entries []LogEntry, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, entry := range entries {
		_, err := writer.WriteString(entry.Content + "\n")
		if err != nil {
			return fmt.Errorf("failed to write entry: %w", err)
		}
	}

	return nil
}

// PrintStats prints statistics about the parsed log
func (p *LogParser) PrintStats(entries []LogEntry) {
	if len(entries) == 0 {
		fmt.Println("No log entries found")
		return
	}

	fmt.Printf("Parsed %d log entries\n", len(entries))
	fmt.Printf("Time range: %s to %s\n",
		entries[0].Timestamp.Format("2006-01-02 15:04:05.000000"),
		entries[len(entries)-1].Timestamp.Format("2006-01-02 15:04:05.000000"))

	duration := entries[len(entries)-1].Timestamp.Sub(entries[0].Timestamp)
	fmt.Printf("Duration: %s\n", duration)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(
			os.Stderr,
			"Usage: %s <input_log_file> [output_log_file]\n",
			os.Args[0],
		)
		os.Exit(1)
	}

	inputFile := os.Args[1]
	outputFile := ""

	if len(os.Args) >= 3 {
		outputFile = os.Args[2]
	} else {
		// Default output filename
		outputFile = strings.TrimSuffix(inputFile, ".log") + "_sorted.log"
	}

	parser := NewLogParser()

	// Parse the log file
	fmt.Printf("Parsing log file: %s\n", inputFile)
	entries, err := parser.ParseFile(inputFile)
	if err != nil {
		log.Fatalf("Failed to parse log file: %v", err)
	}

	// Print statistics
	parser.PrintStats(entries)

	// Write sorted output
	fmt.Printf("Writing sorted log to: %s\n", outputFile)
	err = parser.WriteToFile(entries, outputFile)
	if err != nil {
		log.Fatalf("Failed to write output file: %v", err)
	}

	fmt.Println("Log parsing and sorting completed successfully!")
}
