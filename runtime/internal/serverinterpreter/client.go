// Package serverinterpreter provides the HTTP RPC client for instrument-script-server.
package serverinterpreter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ScriptServerClient is an HTTP RPC client for the instrument-script-server.
type ScriptServerClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewScriptServerClient creates a new client for the instrument-script-server RPC API.
func NewScriptServerClient(host string, port int) *ScriptServerClient {
	return &ScriptServerClient{
		baseURL: fmt.Sprintf("http://%s:%d/rpc", host, port),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// call performs an RPC call to the instrument-script-server.
func (c *ScriptServerClient) call(command string, params interface{}) (*RPCResponse, error) {
	request := RPCRequest{
		Command: command,
		Params:  params,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RPC request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL, "application/json", bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to send RPC request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read RPC response: %w", err)
	}

	var response RPCResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse RPC response: %w", err)
	}

	if !response.OK {
		return &response, fmt.Errorf("RPC error: %s", response.Error)
	}

	return &response, nil
}

// ListInstruments returns the list of available instruments.
func (c *ScriptServerClient) ListInstruments() ([]string, error) {
	response, err := c.call("list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	// ISS returns {"ok":true,"instruments":[...]} at the top level.
	if response.Instruments != nil {
		return response.Instruments, nil
	}

	// Fallback: try legacy format where instruments may be nested in Result.
	if result, ok := response.Result.(map[string]interface{}); ok {
		if instruments, ok := result["instruments"].([]interface{}); ok {
			names := make([]string, len(instruments))
			for i, inst := range instruments {
				if name, ok := inst.(string); ok {
					names[i] = name
				}
			}
			return names, nil
		}
	}

	// No instruments field at all — return empty list (not an error).
	return []string{}, nil
}

// StartInstrument sends the "start" command to create an instrument in the daemon.
// configPath is the path to the instrument YAML config (resolved by the ISS daemon).
// pluginPath optionally overrides the plugin .so to load.
func (c *ScriptServerClient) StartInstrument(configPath string, pluginPath string) (string, error) {
	params := map[string]interface{}{
		"config_path": configPath,
	}
	if pluginPath != "" {
		params["plugin"] = pluginPath
	}

	response, err := c.call("start", params)
	if err != nil {
		return "", err
	}

	// The response contains {"ok":true,"instrument":"<name>"}.
	if response.Instrument != "" {
		return response.Instrument, nil
	}
	return "", nil
}

// StopInstrument sends the "stop" command to remove an instrument from the daemon.
func (c *ScriptServerClient) StopInstrument(name string) error {
	_, err := c.call("stop", map[string]interface{}{
		"name": name,
	})
	return err
}

// Measure runs a Lua script synchronously and returns the parsed call results.
// globals are injected into the Lua environment as named variables.
// typeManifest, if non-nil, is passed as "type_manifest" so ISS calls main with
// positional arguments rather than relying on global injection alone.
func (c *ScriptServerClient) Measure(scriptPath string, globals map[string]interface{}, typeManifest map[string]interface{}) ([]ISSCallResult, error) {
	params := map[string]interface{}{
		"script_path": scriptPath,
	}
	if globals != nil {
		params["globals"] = globals
	}
	if typeManifest != nil {
		params["type_manifest"] = typeManifest
	}

	response, err := c.call("measure", params)
	if err != nil {
		return nil, err
	}

	if response.Results == nil {
		return []ISSCallResult{}, nil
	}

	var results []ISSCallResult
	if err := json.Unmarshal(response.Results, &results); err != nil {
		return nil, fmt.Errorf("failed to parse measure results: %w", err)
	}
	return results, nil
}

// ReadBuffer retrieves the float64 data for a buffer_id from ISS.
func (c *ScriptServerClient) ReadBuffer(bufferID string) ([]float64, error) {
	response, err := c.call("read_buffer", map[string]interface{}{
		"buffer_id": bufferID,
	})
	if err != nil {
		return nil, err
	}
	return response.Data, nil
}
