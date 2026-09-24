
import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ReadBuffer retrieves float64 data for a buffer id. ISS 2.0.0 does not expose
// buffer reads over gRPC, so this shells out to the installed CLI as a temporary
// compatibility adapter.
// FIX: Use the instrument-data package directly instead
func (c *ScriptServerClient) ReadBuffer(bufferID string) ([]float64, error) {
	cmd := exec.Command(c.issBinary, "buffer", "read", bufferID, "--json")
	cmd.Env = c.envWithRuntimePaths()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("instrument-script-server buffer read failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var payload struct {
		OK     bool              `json:"ok"`
		Error  []string          `json:"error"`
		Output []json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse buffer read JSON: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !payload.OK {
		return nil, fmt.Errorf("buffer read failed: %s", strings.Join(payload.Error, "; "))
	}

	for _, raw := range payload.Output {
		var candidate struct {
			Data []float64 `json:"data"`
		}
		if err := json.Unmarshal(raw, &candidate); err == nil && candidate.Data != nil {
			return candidate.Data, nil
		}
	}
	return nil, fmt.Errorf("buffer read JSON did not contain data for %s", bufferID)
}
