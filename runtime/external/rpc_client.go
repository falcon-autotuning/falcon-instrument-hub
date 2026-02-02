package rpcclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL    string       // e.g. "http://127.0.0.1:8555"
	HTTPClient *http.Client // injected for tests; defaults if nil
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type rpcRequest struct {
	Command string      `json:"command"`
	Params  interface{} `json:"params"`
}

type rpcResponse struct {
	Ok    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Raw   json.RawMessage `json:"-"`
}

// Call sends POST /rpc and returns the full JSON response body.
// If ok=false, returns an error containing the server-provided message.
func (c *Client) Call(ctx context.Context, command string, params any) (json.RawMessage, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}

	reqBody, err := json.Marshal(rpcRequest{Command: command, Params: params})
	if err != nil {
		return nil, fmt.Errorf("rpc marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/rpc", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("rpc build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc post: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rpc read response: %w", err)
	}

	// HTTP layer errors still often include JSON; prefer the payload if possible.
	var rr struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	_ = json.Unmarshal(body, &rr)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if rr.Error != "" {
			return nil, fmt.Errorf("rpc http %d: %s", resp.StatusCode, rr.Error)
		}
		return nil, fmt.Errorf("rpc http %d: %s", resp.StatusCode, string(body))
	}

	if rr.Ok == false {
		if rr.Error == "" {
			rr.Error = "unknown rpc error (ok=false)"
		}
		return nil, fmt.Errorf("rpc error: %s", rr.Error)
	}

	return json.RawMessage(body), nil
}

/***************
 * Typed helpers
 ***************/

func (c *Client) DaemonStatus(ctx context.Context) error {
	_, err := c.Call(ctx, "daemon", map[string]any{"action": "status"})
	return err
}

func (c *Client) SubmitMeasure(ctx context.Context, scriptPath string) (jobID string, err error) {
	raw, err := c.Call(ctx, "submit_measure", map[string]any{"script_path": scriptPath})
	if err != nil {
		return "", err
	}
	var out struct {
		Ok    bool   `json:"ok"`
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("submit_measure unmarshal: %w", err)
	}
	if out.JobID == "" {
		return "", fmt.Errorf("submit_measure: missing job_id")
	}
	return out.JobID, nil
}

func (c *Client) JobStatus(ctx context.Context, jobID string) (status string, err error) {
	raw, err := c.Call(ctx, "job_status", map[string]any{"job_id": jobID})
	if err != nil {
		return "", err
	}
	var out struct {
		Ok     bool   `json:"ok"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("job_status unmarshal: %w", err)
	}
	if out.Status == "" {
		return "", fmt.Errorf("job_status: missing status")
	}
	return out.Status, nil
}

func (c *Client) JobResult(ctx context.Context, jobID string) (json.RawMessage, error) {
	raw, err := c.Call(ctx, "job_result", map[string]any{"job_id": jobID})
	if err != nil {
		return nil, err
	}
	return raw, nil
}

