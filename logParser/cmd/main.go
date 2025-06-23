package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

const timeFormat = "2006-01-02 15:04:05.000000"

// LogParser handles parsing of falcon runtime logs
type LogParser struct {
	timestampRegex *regexp.Regexp
}

// NewLogParser creates a new log parser instance
func NewLogParser() *LogParser {
	// Regex to match timestamp pattern: [2025-06-23 11:57:15.123456]
	timestampRegex := regexp.MustCompile(
		`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{6})\]`,
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
			timestamp, err := time.Parse(timeFormat, matches[1])
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
		entries[0].Timestamp.Format(timeFormat),
		entries[len(entries)-1].Timestamp.Format(timeFormat))

	duration := entries[len(entries)-1].Timestamp.Sub(entries[0].Timestamp)
	fmt.Printf("Duration: %s\n", duration)
}

// LogFileInfo represents a log file with its parsed timestamp
type LogFileInfo struct {
	Path      string
	Timestamp time.Time
}

// FindNewestLogFile finds the newest log file in a directory
func FindNewestLogFile(dir string) (string, error) {
	// Pattern to match log files: <prefix>_YYYY-MM-DD_HH-MM-SS.log
	logPattern := regexp.MustCompile(
		`^(.+)_(\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2})\.log$`,
	)

	files, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var logFiles []LogFileInfo

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		matches := logPattern.FindStringSubmatch(file.Name())
		if len(matches) != 3 {
			continue
		}

		// Parse timestamp from filename
		timestampStr := matches[2]
		timestamp, err := time.Parse("2006-01-02_15-04-05", timestampStr)
		if err != nil {
			log.Printf(
				"Warning: failed to parse timestamp from %s: %v",
				file.Name(),
				err,
			)
			continue
		}

		logFiles = append(logFiles, LogFileInfo{
			Path:      filepath.Join(dir, file.Name()),
			Timestamp: timestamp,
		})
	}

	if len(logFiles) == 0 {
		return "", fmt.Errorf("no log files found in directory %s", dir)
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].Timestamp.After(logFiles[j].Timestamp)
	})

	return logFiles[0].Path, nil
}

func main() {
	var newest bool
	var outputFile string

	flag.BoolVar(
		&newest,
		"newest",
		false,
		"Find and parse the newest log file in the specified directory",
	)
	flag.StringVar(&outputFile, "output", "", "Output file path (optional)")
	flag.Parse()

	args := flag.Args()

	if len(args) < 1 {
		fmt.Fprintf(
			os.Stderr,
			"Usage: %s [flags] <input_log_file_or_directory>\n",
			os.Args[0],
		)
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		fmt.Fprintf(
			os.Stderr,
			"  -newest    Find and parse the newest log file in the specified directory\n",
		)
		fmt.Fprintf(
			os.Stderr,
			"  -output    Specify output file path (optional)\n",
		)
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s mylog.log\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -newest /path/to/logs/\n", os.Args[0])
		fmt.Fprintf(
			os.Stderr,
			"  %s -newest -output sorted.log /path/to/logs/\n",
			os.Args[0],
		)
		os.Exit(1)
	}

	var inputFile string
	inputPath := args[0]

	if newest {
		// Find newest log file in directory
		newestFile, err := FindNewestLogFile(inputPath)
		if err != nil {
			log.Fatalf("Failed to find newest log file: %v", err)
		}
		inputFile = newestFile
		fmt.Printf("Found newest log file: %s\n", inputFile)
	} else {
		// Use provided file path directly
		inputFile = inputPath
	}

	// Determine output file path
	if outputFile == "" {
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
