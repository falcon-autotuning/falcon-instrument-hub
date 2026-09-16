package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAnnotatedMeasurementMetadata(t *testing.T) {
	dir := t.TempDir()
	script := `-- @falcon.metadata
-- schema_version: 1
-- measurement: get_voltage
-- targets:
--   getter:
--     capability: voltage
--     role: input
-- @falcon.end
return {}
`
	api := `instrument:
  vendor: Mock
  identifier: Meter1
  instrument_type: voltmeter
channel_groups:
  - name: analog
    io_types:
      - name: voltage
        role: input
        unit: V
`
	apiPath := filepath.Join(dir, "api.yml")
	if err := os.WriteFile(apiPath, []byte(api), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "get_voltage.lua"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	registry, err := loadAnnotatedMeasurementMetadata("", dir, []string{apiPath})
	if err != nil {
		t.Fatal(err)
	}
	requirement, ok := registry.requirement("get_voltage", "getter")
	if !ok || requirement.capability != "voltage" || requirement.role != "input" {
		t.Fatalf("annotation did not override legacy default: %+v", requirement)
	}
	if _, err := loadAnnotatedMeasurementMetadata("", dir, nil); err == nil {
		t.Fatal("annotated scripts without APIs accepted")
	}
	if err := os.WriteFile(apiPath, []byte(strings.Replace(api, "role: input", "role: output", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAnnotatedMeasurementMetadata("", dir, []string{apiPath}); err == nil || !strings.Contains(err.Error(), "not found in loaded instrument APIs") {
		t.Fatalf("expected capability/role mismatch, got %v", err)
	}
}

func TestInvalidAnnotationsPreventHandlerStartup(t *testing.T) {
	manager := &Manager{metadataError: os.ErrInvalid}
	if err := manager.Start(); err == nil {
		t.Fatal("Start accepted invalid metadata")
	}
	if err := manager.StartCoreHandlers(); err == nil {
		t.Fatal("StartCoreHandlers accepted invalid metadata")
	}
}
