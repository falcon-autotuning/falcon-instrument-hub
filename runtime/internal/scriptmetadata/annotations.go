// Package scriptmetadata validates declarative metadata in Teal/Lua headers.
package scriptmetadata

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Target struct {
	RequestSource string `yaml:"request_source"`
	Capability    string `yaml:"capability"`
	Role          string `yaml:"role"`
}

type Response struct {
	MetadataFrom   string `yaml:"metadata_from"`
	ConnectionFrom string `yaml:"connection_from"`
}

type Metadata struct {
	Targets   map[string]Target `yaml:"targets"`
	Responses []Response        `yaml:"responses"`
}

type Annotation struct {
	SchemaVersion int               `yaml:"schema_version"`
	Measurement   string            `yaml:"measurement"`
	Targets       map[string]Target `yaml:"targets"`
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var longComment = regexp.MustCompile(`^\[=*\[`)

// Parse reads a single YAML block in the leading line-comment header. It never
// evaluates user code and ignores annotation-like text after executable code.
func Parse(path string, reader io.Reader) (*Annotation, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var payload strings.Builder
	active, found := false, false
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" && !active {
			continue
		}
		if !strings.HasPrefix(line, "--") {
			if active {
				return nil, fmt.Errorf("%s:%d: annotation must contain line comments and end with @falcon.end", path, lineNumber)
			}
			break
		}
		comment := strings.TrimPrefix(line, "--")
		if longComment.MatchString(comment) {
			if active {
				return nil, fmt.Errorf("%s:%d: annotation must use line comments", path, lineNumber)
			}
			break
		}
		switch strings.TrimSpace(comment) {
		case "@falcon.metadata":
			if active || found {
				return nil, fmt.Errorf("%s:%d: duplicate metadata block", path, lineNumber)
			}
			active, found = true, true
		case "@falcon.end":
			if !active {
				return nil, fmt.Errorf("%s:%d: unexpected @falcon.end", path, lineNumber)
			}
			active = false
		default:
			if active {
				// Remove only the conventional space after '--'; preserve YAML indentation.
				payload.WriteString(strings.TrimPrefix(comment, " "))
				payload.WriteByte('\n')
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: read annotation: %w", path, err)
	}
	if active {
		return nil, fmt.Errorf("%s: missing @falcon.end", path)
	}
	if !found {
		return nil, nil
	}
	decoder := yaml.NewDecoder(strings.NewReader(payload.String()))
	decoder.KnownFields(true)
	var annotation Annotation
	if err := decoder.Decode(&annotation); err != nil {
		return nil, fmt.Errorf("%s: invalid annotation YAML: %w", path, err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("%s: annotation must contain exactly one YAML document", path)
	}
	if err := annotation.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if annotation.Measurement != base {
		return nil, fmt.Errorf("%s: measurement %q must match script basename %q", path, annotation.Measurement, base)
	}
	return &annotation, nil
}

// Validate enforces the version 1 field constraints described by the schema.
func (a Annotation) Validate() error {
	if a.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1")
	}
	if !identifier.MatchString(a.Measurement) {
		return fmt.Errorf("measurement must be an identifier")
	}
	if len(a.Targets) == 0 {
		return fmt.Errorf("targets must not be empty")
	}
	for name, target := range a.Targets {
		if name != "getter" && name != "setter" {
			return fmt.Errorf("unsupported target %q; expected getter or setter", name)
		}
		if target.RequestSource != "" && target.RequestSource != name {
			return fmt.Errorf("target %q request_source must match its name", name)
		}
		if !identifier.MatchString(target.Capability) {
			return fmt.Errorf("target %q capability must be an API io_type identifier", name)
		}
		if target.Role != "input" && target.Role != "output" && target.Role != "setting" {
			return fmt.Errorf("target %q role must be input, output, or setting", name)
		}
	}
	return nil
}

// LoadDirectory accepts matching .tl/.lua companions but rejects conflicting
// declarations. Scripts without annotations keep their legacy behavior.
func LoadDirectory(path string) (map[string]Annotation, error) {
	annotations := make(map[string]Annotation)
	if path == "" {
		return annotations, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read measurement scripts %s: %w", path, err)
	}
	for _, entry := range entries {
		ext := filepath.Ext(entry.Name())
		if entry.IsDir() || (ext != ".tl" && ext != ".lua") {
			continue
		}
		filename := filepath.Join(path, entry.Name())
		file, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		annotation, parseErr := Parse(filename, file)
		closeErr := file.Close()
		if parseErr != nil {
			return nil, parseErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if annotation == nil {
			continue
		}
		if previous, ok := annotations[annotation.Measurement]; ok && !reflect.DeepEqual(previous, *annotation) {
			return nil, fmt.Errorf("%s: conflicting Teal/Lua annotations for %q", filename, annotation.Measurement)
		}
		annotations[annotation.Measurement] = *annotation
	}
	return annotations, nil
}
