package handlers

import (
	"fmt"
	"os"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/ports"
	"github.com/falcon-autotuning/instrument-server/runtime/internal/scriptmetadata"
	"gopkg.in/yaml.v3"
)

type measurementMetadataRegistry struct {
	Measurements map[string]measurementMetadata `yaml:"measurements"`
}

type measurementMetadata = scriptmetadata.Metadata
type measurementTargetMetadata = scriptmetadata.Target
type measurementResponseMetadata = scriptmetadata.Response

func defaultMeasurementMetadataRegistry() measurementMetadataRegistry {
	return measurementMetadataRegistry{
		Measurements: map[string]measurementMetadata{
			"set_voltage":           singleTargetMetadata("setter", "voltage", "output"),
			"set_many_voltages":     singleTargetMetadata("setter", "voltage", "output"),
			"ramp":                  singleTargetMetadata("setter", "voltage", "output"),
			"get_voltage":           singleTargetMetadata("getter", "measured_voltage", "input"),
			"get_many_voltages":     singleTargetMetadata("getter", "measured_voltage", "input"),
			"get_all_voltages":      singleTargetMetadata("getter", "measured_voltage", "input"),
			"measure_leakage":       singleTargetMetadata("getter", "measured_voltage", "input"),
			"measure_current":       singleTargetMetadata("getter", "voltage", "input"),
			"measure_illumination":  singleTargetMetadata("getter", "voltage", "input"),
			"set_sample_rate":       singleTargetMetadata("getter", "sample_rate", "setting"),
			"get_sample_rate":       singleTargetMetadata("getter", "sample_rate", "setting"),
			"set_number_of_samples": singleTargetMetadata("getter", "bins", "setting"),
			"get_number_of_samples": singleTargetMetadata("getter", "bins", "setting"),
			"set_slope":             singleTargetMetadata("setter", "slope", "setting"),
			"get_slope":             singleTargetMetadata("getter", "slope", "setting"),
			"set_trigger_level":     singleTargetMetadata("getter", "trigger_level", "setting"),
			"get_trigger_level":     singleTargetMetadata("getter", "trigger_level", "setting"),
			"set_trigger_leader":    singleTargetMetadata("getter", "trigger_level", "setting"),
			"get_trigger_leader":    singleTargetMetadata("getter", "trigger_level", "setting"),
			"measure_get_set":       sweepMetadata("stream"),
			"measure_1D_buffered":   sweepMetadata("stream"),
			"measure_2D_buffered":   sweepMetadata("stream"),
		},
	}
}

func singleTargetMetadata(targetKind, capability, role string) measurementMetadata {
	return measurementMetadata{
		Targets: map[string]measurementTargetMetadata{
			targetKind: {
				RequestSource: targetKind,
				Capability:    capability,
				Role:          role,
			},
		},
		Responses: []measurementResponseMetadata{
			{MetadataFrom: targetKind, ConnectionFrom: targetKind},
		},
	}
}

func sweepMetadata(getterCapability string) measurementMetadata {
	return measurementMetadata{
		Targets: map[string]measurementTargetMetadata{
			"setter": {
				RequestSource: "setter",
				Capability:    "voltage",
				Role:          "output",
			},
			"getter": {
				RequestSource: "getter",
				Capability:    getterCapability,
				Role:          "input",
			},
		},
		Responses: []measurementResponseMetadata{
			{MetadataFrom: "getter", ConnectionFrom: "setter"},
		},
	}
}

func loadMeasurementMetadataRegistry(path string) (measurementMetadataRegistry, error) {
	registry := defaultMeasurementMetadataRegistry()
	if path == "" {
		return registry, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return registry, fmt.Errorf("read measurement metadata %s: %w", path, err)
	}

	var loaded measurementMetadataRegistry
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		return registry, fmt.Errorf("parse measurement metadata %s: %w", path, err)
	}

	for name, metadata := range loaded.Measurements {
		registry.Measurements[name] = metadata
	}
	return registry, nil
}

func (r measurementMetadataRegistry) requirement(scriptName, targetKind string) (scriptPortRequirement, bool) {
	metadata, ok := r.Measurements[scriptName]
	if !ok {
		return scriptPortRequirement{}, false
	}
	target, ok := metadata.Targets[targetKind]
	if !ok || target.Capability == "" || target.Role == "" {
		return scriptPortRequirement{}, false
	}
	return scriptPortRequirement{capability: target.Capability, role: target.Role}, true
}

func loadAnnotatedMeasurementMetadata(metadataPath, scriptsPath string, apiPaths []string) (measurementMetadataRegistry, error) {
	registry, err := loadMeasurementMetadataRegistry(metadataPath)
	if err != nil {
		return registry, err
	}
	annotations, err := scriptmetadata.LoadDirectory(scriptsPath)
	if err != nil {
		return registry, err
	}
	if len(annotations) == 0 {
		return registry, nil
	}
	if len(apiPaths) == 0 {
		return registry, fmt.Errorf("annotated measurement scripts require instrument API files")
	}
	apis, err := ports.ParseInstrumentAPIs(apiPaths)
	if err != nil {
		return registry, err
	}
	for name, annotation := range annotations {
		for targetName, target := range annotation.Targets {
			found := false
			for _, api := range apis {
				for _, group := range api.ChannelGroups {
					for _, ioType := range group.IoTypes {
						if ioType.Name == target.Capability && ioType.Role == target.Role {
							found = true
						}
					}
				}
			}
			if !found {
				return registry, fmt.Errorf("measurement %q target %q: capability %q role %q not found in loaded instrument APIs", name, targetName, target.Capability, target.Role)
			}
		}
		metadata := registry.Measurements[name]
		metadata.Targets = annotation.Targets
		registry.Measurements[name] = metadata
	}
	return registry, nil
}
