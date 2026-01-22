package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// InstrumentServerClient provides methods to interact with the instrument-script-server RPC API
type InstrumentServerClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewInstrumentServerClient creates a new client for the instrument-script-server
func NewInstrumentServerClient(baseURL string) *InstrumentServerClient {
	return &InstrumentServerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// InstrumentStatus represents the status of an instrument
type InstrumentStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	PID    int    `json:"pid,omitempty"`
}

// MeasurementResult represents the result of a measurement
type MeasurementResult struct {
	Success bool                   `json:"success"`
	Results map[string]interface{} `json:"results,omitempty"`
	Error   string                 `json:"error,omitempty"`
}

// StartInstrumentRequest represents a request to start an instrument
type StartInstrumentRequest struct {
	ConfigFile string `json:"config_file"`
}

// MeasureRequest represents a request to run a measurement
type MeasureRequest struct {
	ScriptPath     string                 `json:"script_path"`
	Globals        map[string]interface{} `json:"globals,omitempty"`
	TypeManifest   string                 `json:"type_manifest,omitempty"`
	OutputJSON     bool                   `json:"output_json"`
}

// StartInstrument starts an instrument using the instrument-script-server
func (c *InstrumentServerClient) StartInstrument(ctx context.Context, configFile string) error {
	req := StartInstrumentRequest{
		ConfigFile: configFile,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, "POST", "/api/instruments/start", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to start instrument: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// StopInstrument stops an instrument
func (c *InstrumentServerClient) StopInstrument(ctx context.Context, instrumentName string) error {
	url := fmt.Sprintf("/api/instruments/%s/stop", instrumentName)
	resp, err := c.doRequest(ctx, "POST", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to stop instrument: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// ListInstruments lists all instruments managed by the instrument-script-server
func (c *InstrumentServerClient) ListInstruments(ctx context.Context) ([]InstrumentStatus, error) {
	resp, err := c.doRequest(ctx, "GET", "/api/instruments/list", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list instruments: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var instruments []InstrumentStatus
	if err := json.NewDecoder(resp.Body).Decode(&instruments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return instruments, nil
}

// Measure executes a measurement script on the instrument-script-server
func (c *InstrumentServerClient) Measure(ctx context.Context, req MeasureRequest) (*MeasurementResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, "POST", "/api/measure", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to execute measurement: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result MeasurementResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// SendCommand sends a command to a specific instrument
func (c *InstrumentServerClient) SendCommand(ctx context.Context, instrumentName, command string, params map[string]interface{}) (interface{}, error) {
	reqBody := map[string]interface{}{
		"command": command,
		"params":  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("/api/instruments/%s/command", instrumentName)
	resp, err := c.doRequest(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to send command: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result["result"], nil
}

// doRequest performs an HTTP request to the instrument-script-server
func (c *InstrumentServerClient) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}
