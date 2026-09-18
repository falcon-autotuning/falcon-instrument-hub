package serverinterpreter

import (
	"fmt"
	"path/filepath"
)

// ScriptDispatcher executes user-provided Lua measurement scripts in ISS.
type ScriptDispatcher struct {
	client      *ScriptServerClient
	scriptsPath string
}

// ScriptDispatcherConfig configures gRPC dispatch and local CLI buffer reads.
type ScriptDispatcherConfig struct {
	ServerHost  string
	ServerPort  int
	ScriptsPath string
	ISSBinary   string
	ISSLibPath  string
}

func NewScriptDispatcher(config ScriptDispatcherConfig) *ScriptDispatcher {
	host := config.ServerHost
	port := config.ServerPort
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 8555
	}

	client := NewScriptServerClientWithOptions(host, port, ScriptServerClientOptions{
		ISSBinary:  config.ISSBinary,
		ISSLibPath: config.ISSLibPath,
	})
	return &ScriptDispatcher{client: client, scriptsPath: config.ScriptsPath}
}

// ResolvedCallResult extends ISSCallResult with buffer data resolved inline.
type ResolvedCallResult struct {
	ISSCallResult
	BufferData []float64 // populated when Return.Type == "buffer"
}

// RunMeasurement calls ISS measure (sync), resolves all buffer results, returns
// the full call list with buffer data populated.
// typeManifest, if non-nil, tells ISS to call main with positional arguments
// (required for Teal-compiled scripts with named parameters).
func (d *ScriptDispatcher) RunMeasurement(scriptName string, globals map[string]interface{}, typeManifest map[string]interface{}) ([]ResolvedCallResult, error) {
	scriptPath := filepath.Join(d.scriptsPath, scriptName+".lua")
	results, err := d.client.Measure(scriptPath, globals, typeManifest)
	if err != nil {
		return nil, fmt.Errorf("measure script %s: %w", scriptName, err)
	}

	resolved := make([]ResolvedCallResult, len(results))
	for i, r := range results {
		resolved[i] = ResolvedCallResult{ISSCallResult: r}
		if r.Return.Type == "buffer" && r.Return.BufferID != "" {
			data, err := d.client.ReadBuffer(r.Return.BufferID)
			if err != nil {
				return nil, fmt.Errorf("read_buffer %s: %w", r.Return.BufferID, err)
			}
			resolved[i].BufferData = data
		}
	}
	return resolved, nil
}
