package scriptmetadata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validHeader = `-- @falcon.metadata
-- schema_version: 1
-- measurement: get_voltage
-- targets:
--   getter:
--     capability: measured_voltage
--     role: input
-- @falcon.end
return { main = Get_Voltage }
`

func TestParse(t *testing.T) {
	annotation, err := Parse("get_voltage.tl", strings.NewReader(validHeader))
	if err != nil {
		t.Fatal(err)
	}
	if annotation.Targets["getter"].Capability != "measured_voltage" || annotation.Targets["getter"].Role != "input" {
		t.Fatalf("unexpected annotation: %+v", annotation)
	}
}

func TestInvalidAnnotations(t *testing.T) {
	tests := []struct {
		name, header, want string
	}{
		{"version", strings.Replace(validHeader, "schema_version: 1", "schema_version: 2", 1), "schema_version must be 1"},
		{"unknown field", strings.Replace(validHeader, "role: input", "rol: input", 1), "field rol not found"},
		{"missing capability", strings.Replace(validHeader, "--     capability: measured_voltage\n", "", 1), "capability must be"},
		{"role", strings.Replace(validHeader, "role: input", "role: meter", 1), "role must be"},
		{"target", strings.Replace(validHeader, "getter:", "meter:", 1), "unsupported target"},
		{"request source", strings.Replace(validHeader, "role: input", "role: input\n--     request_source: setter", 1), "request_source must match"},
		{"basename", strings.Replace(validHeader, "measurement: get_voltage", "measurement: other", 1), "must match script basename"},
		{"duplicate YAML key", strings.Replace(validHeader, "role: input", "role: input\n--     role: output", 1), "already defined"},
		{"extra document", strings.Replace(validHeader, "-- @falcon.end", "-- ---\n-- schema_version: 1\n-- @falcon.end", 1), "exactly one YAML document"},
		{"unterminated", strings.Split(validHeader, "-- @falcon.end")[0], "missing @falcon.end"},
		{"code inside block", strings.Replace(validHeader, "-- @falcon.end", "return {}\n-- @falcon.end", 1), "must contain line comments"},
		{"duplicate block", strings.Replace(validHeader, "return { main = Get_Voltage }", validHeader, 1), "duplicate metadata block"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse("get_voltage.lua", strings.NewReader(test.header))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestIgnoreAnnotationsInsideCode(t *testing.T) {
	for _, prefix := range []string{"local text = [[\n", "--[[\n", "--[=[\n"} {
		annotation, err := Parse("get_voltage.lua", strings.NewReader(prefix+validHeader+"\n]]"))
		if err != nil || annotation != nil {
			t.Fatalf("parsed text after %q: %+v, %v", prefix, annotation, err)
		}
	}
}

func TestCompanionAnnotations(t *testing.T) {
	dir := t.TempDir()
	for _, ext := range []string{".tl", ".lua"} {
		if err := os.WriteFile(filepath.Join(dir, "get_voltage"+ext), []byte(validHeader), 0600); err != nil {
			t.Fatal(err)
		}
	}
	annotations, err := LoadDirectory(dir)
	if err != nil || len(annotations) != 1 {
		t.Fatalf("matching companions rejected: %+v, %v", annotations, err)
	}
	conflict := strings.Replace(validHeader, "measured_voltage", "voltage", 1)
	if err := os.WriteFile(filepath.Join(dir, "get_voltage.lua"), []byte(conflict), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDirectory(dir); err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("expected companion conflict, got %v", err)
	}
}
