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

// SubmitMeasure submits a measurement script to be executed.
func (c *ScriptServerClient) SubmitMeasure(scriptPath string) (string, error) {
	params := SubmitMeasureParams{
		ScriptPath: scriptPath,
	}

	response, err := c.call("submit_measure", params)
	if err != nil {
		return "", err
	}

	return response.JobID, nil
}

// JobStatus queries the status of a measurement job.
func (c *ScriptServerClient) JobStatus(jobID string) (string, error) {
	params := JobStatusParams{
		JobID: jobID,
	}

	response, err := c.call("job_status", params)
	if err != nil {
		return "", err
	}

	return response.Status, nil
}

// JobResult retrieves the result of a completed measurement job.
func (c *ScriptServerClient) JobResult(jobID string) (interface{}, error) {
	params := JobResultParams{
		JobID: jobID,
	}

	response, err := c.call("job_result", params)
	if err != nil {
		return nil, err
	}

	return response.Result, nil
}

// WaitForJob polls the job status until completion or timeout.
func (c *ScriptServerClient) WaitForJob(jobID string, pollInterval time.Duration, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := c.JobStatus(jobID)
		if err != nil {
			return "", err
		}

		switch status {
		case "completed", "failed", "canceled":
			return status, nil
		}

		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timeout waiting for job %s", jobID)
}

// ListInstruments returns the list of available instruments.
func (c *ScriptServerClient) ListInstruments() ([]string, error) {
	response, err := c.call("list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	// Parse instruments from response
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

	return nil, fmt.Errorf("unexpected response format for list")
}
