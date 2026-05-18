package serverinterpreter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockInstrumentScriptServer is a mock HTTP server for testing.
type MockInstrumentScriptServer struct {
	server   *httptest.Server
	requests []map[string]interface{}
}

// NewMockInstrumentScriptServer creates a mock server for testing.
func NewMockInstrumentScriptServer() *MockInstrumentScriptServer {
	mock := &MockInstrumentScriptServer{
		requests: make([]map[string]interface{}, 0),
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

	switch command {
	case "measure":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      true,
			"results": []interface{}{},
		})

	case "read_buffer":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":            true,
			"data":          []float64{},
			"buffer_id":     "x",
			"element_count": 0,
		})

	case "list":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"instruments": []string{"DAC1", "DMM1"},
			},
		})

	case "start":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":         true,
			"instrument": "MockInstrument",
		})

	case "stop":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
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

func TestDefaultBridgeConfig(t *testing.T) {
	config := DefaultBridgeConfig()

	assert.Equal(t, "127.0.0.1", config.ScriptServerHost)
	assert.Equal(t, 8555, config.ScriptServerPort)
}
