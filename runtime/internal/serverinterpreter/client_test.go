package serverinterpreter

import (
	"testing"

	daemonv1 "github.com/falcon-autotuning/instrument-server/runtime/internal/issproto/instserver/daemon/v1"
)

func TestBuildMeasureJobRequestConvertsGlobalsAndSkipsContext(t *testing.T) {
	req, err := buildMeasureJobRequest(
		"/tmp/measure.lua",
		map[string]interface{}{
			"sampleRate": 1000,
			"getter": map[string]interface{}{
				"id":      "Meter1",
				"channel": 2,
			},
		},
		map[string]interface{}{
			"parameters": []map[string]interface{}{
				{"name": "ctx", "type": "RuntimeContext"},
				{"name": "sampleRate", "type": "number"},
				{"name": "getter", "type": "InstrumentTarget"},
			},
		},
	)
	if err != nil {
		t.Fatalf("buildMeasureJobRequest returned error: %v", err)
	}

	if req.GetScriptPath() != "/tmp/measure.lua" {
		t.Fatalf("unexpected script path: %s", req.GetScriptPath())
	}
	if got := req.GetGlobals().GetMap()["sampleRate"].GetI(); got != 1000 {
		t.Fatalf("sampleRate global = %d, want 1000", got)
	}
	getter := req.GetGlobals().GetMap()["getter"].GetMMap().GetValues()
	if got := getter["id"].GetS(); got != "Meter1" {
		t.Fatalf("getter id = %q, want Meter1", got)
	}
	if got := getter["channel"].GetI(); got != 2 {
		t.Fatalf("getter channel = %d, want 2", got)
	}

	params := req.GetTypeManifest().GetParameters()
	if len(params) != 2 {
		t.Fatalf("manifest parameter count = %d, want 2", len(params))
	}
	if params[0].GetName() == "ctx" || params[1].GetName() == "ctx" {
		t.Fatalf("ctx should not be sent in ISS type manifest: %#v", params)
	}
	if params[0].GetType() != daemonv1.LuaTypes_LUA_TYPES_DOUBLE {
		t.Fatalf("sampleRate type = %s, want double", params[0].GetType())
	}
}

func TestBuildMeasureJobRequestConvertsTypedScalarMaps(t *testing.T) {
	req, err := buildMeasureJobRequest(
		"/tmp/measure.lua",
		map[string]interface{}{
			"setVoltages": map[string]float64{
				"Source1:1": 0.25,
				"Source1:2": -0.5,
			},
		},
		map[string]interface{}{
			"parameters": []map[string]interface{}{
				{"name": "setVoltages", "type": "{string: number}"},
			},
		},
	)
	if err != nil {
		t.Fatalf("buildMeasureJobRequest returned error: %v", err)
	}

	values := req.GetGlobals().GetMap()["setVoltages"].GetMMap().GetValues()
	if got := values["Source1:1"].GetD(); got != 0.25 {
		t.Fatalf("Source1:1 voltage = %v, want 0.25", got)
	}
	if got := values["Source1:2"].GetD(); got != -0.5 {
		t.Fatalf("Source1:2 voltage = %v, want -0.5", got)
	}
}

func TestMeasureJobResultToCallResultsMapsBufferReturn(t *testing.T) {
	resp := &daemonv1.MeasureJobResultResponse{
		Results: []*daemonv1.CommandResult{
			{
				InstrumentName: "Meter1",
				Verb:           "GET_DATAPOINT",
				Param: []*daemonv1.TypedParameter{
					{
						Name: "return",
						Type: daemonv1.LuaTypes_LUA_TYPES_DATA_BUFFER,
						Value: &daemonv1.VariableValue{
							Value: &daemonv1.VariableValue_S{S: "buffer-123"},
						},
						Dbmeta: &daemonv1.DataBufferMetadata{
							ElementCount: 42,
							DataType:     3,
						},
					},
				},
			},
		},
	}

	results := measureJobResultToCallResults(resp)
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	result := results[0]
	if result.Instrument != "Meter1" || result.Verb != "GET_DATAPOINT" {
		t.Fatalf("unexpected result identity: %#v", result)
	}
	if result.Return.Type != "buffer" {
		t.Fatalf("return type = %q, want buffer", result.Return.Type)
	}
	if result.Return.BufferID != "buffer-123" {
		t.Fatalf("buffer id = %q, want buffer-123", result.Return.BufferID)
	}
	if result.Return.ElementCount != 42 {
		t.Fatalf("element count = %d, want 42", result.Return.ElementCount)
	}
}
