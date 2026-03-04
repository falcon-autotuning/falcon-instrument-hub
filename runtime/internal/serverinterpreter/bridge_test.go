package serverinterpreter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockInstrumentScriptServer is a mock HTTP server for testing.
type MockInstrumentScriptServer struct {
	server     *httptest.Server
	jobs       map[string]string           // jobID -> status
	jobResults map[string]interface{}      // jobID -> result
	requests   []map[string]interface{}    // recorded requests
}

// NewMockInstrumentScriptServer creates a mock server for testing.
func NewMockInstrumentScriptServer() *MockInstrumentScriptServer {
	mock := &MockInstrumentScriptServer{
		jobs:       make(map[string]string),
		jobResults: make(map[string]interface{}),
		requests:   make([]map[string]interface{}, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleRequest))
	return mock
}

func (m *MockInstrumentScriptServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.URL.Path != "/rpc" {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "Only POST /rpc is supported",
		})
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "Invalid JSON",
		})
		return
	}

	m.requests = append(m.requests, req)

	command, _ := req["command"].(string)
	params, _ := req["params"].(map[string]interface{})

	switch command {
	case "submit_measure":
		jobID := "mock_job_" + time.Now().Format("20060102_150405")
		m.jobs[jobID] = "completed"
		m.jobResults[jobID] = map[string]interface{}{
			"status":  "success",
			"script":  params["script_path"],
			"results": []interface{}{},
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     true,
			"job_id": jobID,
		})

	case "job_status":
		jobID, _ := params["job_id"].(string)
		status, exists := m.jobs[jobID]
		if !exists {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":    false,
				"error": "Job not found",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     true,
			"job_id": jobID,
			"status": status,
		})

	case "job_result":
		jobID, _ := params["job_id"].(string)
		result, exists := m.jobResults[jobID]
		if !exists {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":    false,
				"error": "Job not found",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     true,
			"job_id": jobID,
			"result": result,
		})

	case "list":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"instruments": []string{"DAC1", "DMM1"},
			},
		})

	default:
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "Unknown command",
		})
	}
}

func (m *MockInstrumentScriptServer) Close() {
	m.server.Close()
}

func (m *MockInstrumentScriptServer) URL() string {
	return m.server.URL
}

func (m *MockInstrumentScriptServer) GetRequests() []map[string]interface{} {
	return m.requests
}

// Helper to extract host and port from httptest server URL
func parseHostPort(url string) (string, int) {
	// URL is like "http://127.0.0.1:12345"
	url = strings.TrimPrefix(url, "http://")
	parts := strings.Split(url, ":")
	if len(parts) != 2 {
		return "127.0.0.1", 8555
	}
	var port int
	_ = json.Unmarshal([]byte(parts[1]), &port)
	return parts[0], port
}

func TestScriptServerClient_SubmitMeasure(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	// Parse the mock server URL
	urlParts := strings.TrimPrefix(mock.URL(), "http://")
	hostPort := strings.Split(urlParts, ":")
	host := hostPort[0]
	port := 0
	if len(hostPort) > 1 {
		json.Unmarshal([]byte(hostPort[1]), &port)
	}

	client := NewScriptServerClient(host, port)

	jobID, err := client.SubmitMeasure("/path/to/script.lua")
	require.NoError(t, err)
	assert.Contains(t, jobID, "mock_job_")

	// Verify request was recorded
	requests := mock.GetRequests()
	require.Len(t, requests, 1)
	assert.Equal(t, "submit_measure", requests[0]["command"])
}

func TestScriptServerClient_JobStatus(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := strings.TrimPrefix(mock.URL(), "http://")
	hostPort := strings.Split(urlParts, ":")
	host := hostPort[0]
	port := 0
	if len(hostPort) > 1 {
		json.Unmarshal([]byte(hostPort[1]), &port)
	}

	client := NewScriptServerClient(host, port)

	// First submit a job
	jobID, err := client.SubmitMeasure("/test.lua")
	require.NoError(t, err)

	// Then get its status
	status, err := client.JobStatus(jobID)
	require.NoError(t, err)
	assert.Equal(t, "completed", status)
}

func TestScriptServerClient_JobResult(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := strings.TrimPrefix(mock.URL(), "http://")
	hostPort := strings.Split(urlParts, ":")
	host := hostPort[0]
	port := 0
	if len(hostPort) > 1 {
		json.Unmarshal([]byte(hostPort[1]), &port)
	}

	client := NewScriptServerClient(host, port)

	// Submit a job
	jobID, err := client.SubmitMeasure("/test.lua")
	require.NoError(t, err)

	// Get result
	result, err := client.JobResult(jobID)
	require.NoError(t, err)
	require.NotNil(t, result)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "success", resultMap["status"])
}

func TestScriptServerClient_WaitForJob(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := strings.TrimPrefix(mock.URL(), "http://")
	hostPort := strings.Split(urlParts, ":")
	host := hostPort[0]
	port := 0
	if len(hostPort) > 1 {
		json.Unmarshal([]byte(hostPort[1]), &port)
	}

	client := NewScriptServerClient(host, port)

	jobID, err := client.SubmitMeasure("/test.lua")
	require.NoError(t, err)

	status, err := client.WaitForJob(jobID, 10*time.Millisecond, 1*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "completed", status)
}

func TestBridge_ExecuteSetVoltage(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := strings.TrimPrefix(mock.URL(), "http://")
	hostPort := strings.Split(urlParts, ":")
	host := hostPort[0]
	port := 0
	if len(hostPort) > 1 {
		json.Unmarshal([]byte(hostPort[1]), &port)
	}

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
	}

	bridge, err := NewBridge(config)
	require.NoError(t, err)

	result, err := bridge.ExecuteSetVoltage("DAC1", 0, 1.5)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.JobID, "mock_job_")
	assert.Equal(t, "completed", result.Status)
}

func TestBridge_ExecuteGetVoltage(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := strings.TrimPrefix(mock.URL(), "http://")
	hostPort := strings.Split(urlParts, ":")
	host := hostPort[0]
	port := 0
	if len(hostPort) > 1 {
		json.Unmarshal([]byte(hostPort[1]), &port)
	}

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
	}

	bridge, err := NewBridge(config)
	require.NoError(t, err)

	result, err := bridge.ExecuteGetVoltage("DMM1", 0)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.JobID, "mock_job_")
	assert.Equal(t, "completed", result.Status)
}

func TestBridge_ExecuteMeasurementRequestJSON(t *testing.T) {
	mock := NewMockInstrumentScriptServer()
	defer mock.Close()

	urlParts := strings.TrimPrefix(mock.URL(), "http://")
	hostPort := strings.Split(urlParts, ":")
	host := hostPort[0]
	port := 0
	if len(hostPort) > 1 {
		json.Unmarshal([]byte(hostPort[1]), &port)
	}

	config := BridgeConfig{
		ScriptServerHost: host,
		ScriptServerPort: port,
	}

	bridge, err := NewBridge(config)
	require.NoError(t, err)

	jsonStr := `{
		"measurementName": "test_measurement",
		"setters": [{"id": "DAC1", "channel": 0}],
		"setVoltages": {"DAC1": 2.5}
	}`

	result, err := bridge.ExecuteMeasurementRequestJSON(jsonStr)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Contains(t, result.JobID, "mock_job_")
	assert.Equal(t, "completed", result.Status)
}

func TestDefaultBridgeConfig(t *testing.T) {
	config := DefaultBridgeConfig()

	assert.Equal(t, "127.0.0.1", config.ScriptServerHost)
	assert.Equal(t, 8555, config.ScriptServerPort)
}
