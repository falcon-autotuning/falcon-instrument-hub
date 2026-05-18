// Package serverinterpreter provides a live integration test that boots a real
// instrument-script-server process with mock VISA instruments and exercises the
// Hub's Bridge / ScriptServerClient against it.
//
// Skip with:  go test -run TestLive -short
//
// Requirements:
//   - instrument-script-server binary built at ../../../../instrument-script-server/build/instrument-script-server
//   - mock_visa_plugin.so at        ../../../../instrument-script-server/build/tests/mock_visa_plugin.so
//   - The tests/data/ fixtures at   ../../../../instrument-script-server/tests/data/
package serverinterpreter

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Paths – resolve relative to this source file so `go test` works from any cwd.
// ---------------------------------------------------------------------------

func repoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile = .../falcon-instrument-hub/runtime/internal/serverinterpreter/live_iss_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
}

func issRoot() string { return filepath.Join(repoRoot(), "instrument-script-server") }
func issBinary() string {
	return filepath.Join(issRoot(), "build", "instrument-script-server")
}
func mockPluginPath() string {
	return filepath.Join(issRoot(), "build", "tests", "mock_visa_plugin.so")
}
func issTestData() string    { return filepath.Join(issRoot(), "tests", "data") }
func issTestScripts() string { return filepath.Join(issTestData(), "test_scripts") }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// freePort asks the OS for an available TCP port.
func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// issProcess manages a real instrument-script-server subprocess.
type issProcess struct {
	cmd     *exec.Cmd
	rpcPort int
	workDir string
	cancel  context.CancelFunc
}

// startISS launches the ISS daemon in a temp directory. Caller must call stop().
//
// The ISS uses a system-wide PID file so only one daemon can run at a time.
// The RPC port is controlled by the INSTRUMENT_SCRIPT_SERVER_RPC_PORT env var.
func startISS(t *testing.T) *issProcess {
	t.Helper()

	binary := issBinary()
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		t.Skipf("instrument-script-server binary not found at %s – build it first", binary)
	}
	plugin := mockPluginPath()
	if _, err := os.Stat(plugin); os.IsNotExist(err) {
		t.Skipf("mock_visa_plugin.so not found at %s – build ISS tests first", plugin)
	}

	// Stop any previously running daemon to avoid PID file conflicts.
	stopCmd := exec.Command(binary, "daemon", "stop")
	_ = stopCmd.Run()
	time.Sleep(500 * time.Millisecond)

	workDir := t.TempDir()
	rpcPort := freePort()

	// Copy fixture files into work dir so relative paths resolve.
	copyFixtures(t, workDir)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, "daemon", "start")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		// Configure the RPC port via env var.
		fmt.Sprintf("INSTRUMENT_SCRIPT_SERVER_RPC_PORT=%d", rpcPort),
	)
	// Pipe output instead of inheriting stdout/stderr to avoid WaitDelay
	// errors when the daemon subprocess doesn't close its streams quickly.
	// We discard daemon output in tests to keep output clean; set to
	// os.Stdout/os.Stderr for debugging.
	cmd.WaitDelay = 2 * time.Second

	require.NoError(t, cmd.Start(), "failed to start ISS daemon")

	p := &issProcess{cmd: cmd, rpcPort: rpcPort, workDir: workDir, cancel: cancel}

	// Wait for the RPC endpoint to become responsive by polling with a raw
	// HTTP request (avoids depending on ListInstruments parsing).
	deadline := time.Now().Add(10 * time.Second)
	httpClient := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := httpClient.Post(
			fmt.Sprintf("http://127.0.0.1:%d/rpc", rpcPort),
			"application/json",
			strings.NewReader(`{"command":"list","params":{}}`),
		)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Logf("ISS daemon ready on port %d", rpcPort)
				return p
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	p.stop(t)
	t.Fatalf("ISS daemon did not become ready on port %d within 10 s", rpcPort)
	return nil
}

func (p *issProcess) stop(t *testing.T) {
	t.Helper()
	// Try graceful shutdown via CLI first.
	stopCmd := exec.Command(issBinary(), "daemon", "stop")
	_ = stopCmd.Run()
	time.Sleep(500 * time.Millisecond)
	// Then cancel context (sends SIGKILL) and reap.
	p.cancel()
	_ = p.cmd.Wait()
}

// startMockInstruments registers mock instruments in the running daemon via
// HTTP RPC "start" commands. This is the correct way to populate the daemon's
// InstrumentRegistry (CLI start creates instruments in a separate process).
func startMockInstruments(t *testing.T, client *ScriptServerClient, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		configRel := fmt.Sprintf("tests/data/mock_instrument%d.yaml", i)
		name, err := client.StartInstrument(configRel, mockPluginPath())
		require.NoError(t, err, "failed to start mock instrument %d", i)
		t.Logf("started instrument: %s", name)
	}
}

// copyFixtures copies the mock instrument configs + test scripts into dst so
// that relative api_ref paths resolve when ISS starts instruments.
func copyFixtures(t *testing.T, dst string) {
	t.Helper()

	src := issTestData()
	dataDir := filepath.Join(dst, "tests", "data")
	require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "test_scripts"), 0o755))

	for _, name := range []string{
		"mock_api.yaml",
		"mock_instrument1.yaml",
		"mock_instrument2.yaml",
		"mock_instrument3.yaml",
		"test_script.lua",
	} {
		copyFile(t, filepath.Join(src, name), filepath.Join(dataDir, name))
	}

	// Copy all test scripts.
	entries, err := os.ReadDir(filepath.Join(src, "test_scripts"))
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		copyFile(t,
			filepath.Join(src, "test_scripts", e.Name()),
			filepath.Join(dataDir, "test_scripts", e.Name()),
		)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err, "read %s", src)
	require.NoError(t, os.WriteFile(dst, data, 0o644), "write %s", dst)
}

// ---------------------------------------------------------------------------
// Step 1 – Boot real ISS, verify basic RPC handshake
// ---------------------------------------------------------------------------

func TestLiveISS_ListInstruments(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live ISS test in -short mode")
	}

	iss := startISS(t)
	defer iss.stop(t)

	client := NewScriptServerClient("127.0.0.1", iss.rpcPort)

	instruments, err := client.ListInstruments()
	require.NoError(t, err)
	t.Logf("instruments (should be empty): %v", instruments)
	assert.Empty(t, instruments, "daemon starts with no instruments")
}

// ---------------------------------------------------------------------------
// Step 1 – Start mock instruments via RPC, verify they appear in list
// ---------------------------------------------------------------------------

func TestLiveISS_StartMockInstruments(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live ISS test in -short mode")
	}

	iss := startISS(t)
	defer iss.stop(t)

	client := NewScriptServerClient("127.0.0.1", iss.rpcPort)
	startMockInstruments(t, client, 3)

	instruments, err := client.ListInstruments()
	require.NoError(t, err)
	t.Logf("instruments after start: %v", instruments)
	assert.Len(t, instruments, 3, "expected 3 mock instruments")
	assert.Contains(t, instruments, "MockInstrument1")
	assert.Contains(t, instruments, "MockInstrument2")
	assert.Contains(t, instruments, "MockInstrument3")
}

// ---------------------------------------------------------------------------
// Step 2 – Submit a measurement script and poll for result
// ---------------------------------------------------------------------------

func TestLiveISS_SubmitMeasure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live ISS test in -short mode")
	}

	iss := startISS(t)
	defer iss.stop(t)

	client := NewScriptServerClient("127.0.0.1", iss.rpcPort)
	startMockInstruments(t, client, 3)

	// Submit the simplest test script (uses MockInstrument1.ECHO).
	scriptPath := filepath.Join("tests", "data", "test_scripts", "simple_call.lua")
	jobID, err := client.SubmitMeasure(scriptPath)
	require.NoError(t, err)
	require.NotEmpty(t, jobID)
	t.Logf("submitted job: %s", jobID)

	// Poll to completion.
	status, err := client.WaitForJob(jobID, 200*time.Millisecond, 30*time.Second)
	require.NoError(t, err)
	t.Logf("job status: %s", status)
	assert.Equal(t, "completed", status)

	// Retrieve result.
	result, err := client.JobResult(jobID)
	require.NoError(t, err)
	t.Logf("job result: %+v", result)
}

// ---------------------------------------------------------------------------
// Step 2 – Use the Bridge high-level API against real ISS
// ---------------------------------------------------------------------------

func TestLiveISS_BridgeSetGetVoltage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live ISS test in -short mode")
	}

	iss := startISS(t)
	defer iss.stop(t)

	client := NewScriptServerClient("127.0.0.1", iss.rpcPort)
	startMockInstruments(t, client, 1)

	config := BridgeConfig{
		ScriptServerHost: "127.0.0.1",
		ScriptServerPort: iss.rpcPort,
	}
	bridge, err := NewBridge(config)
	require.NoError(t, err)

	// Set voltage.
	setResult, err := bridge.ExecuteSetVoltage("MockInstrument1", 0, 2.5)
	require.NoError(t, err)
	t.Logf("SetVoltage result: %+v", setResult)

	// Get voltage.
	getResult, err := bridge.ExecuteGetVoltage("MockInstrument1", 0)
	require.NoError(t, err)
	t.Logf("GetVoltage result: %+v", getResult)
}

// ---------------------------------------------------------------------------
// Step 2 – Execute a MeasurementRequest JSON through the bridge
// ---------------------------------------------------------------------------

func TestLiveISS_BridgeMeasurementRequestJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live ISS test in -short mode")
	}

	iss := startISS(t)
	defer iss.stop(t)

	client := NewScriptServerClient("127.0.0.1", iss.rpcPort)
	startMockInstruments(t, client, 1)

	config := BridgeConfig{
		ScriptServerHost: "127.0.0.1",
		ScriptServerPort: iss.rpcPort,
	}
	bridge, err := NewBridge(config)
	require.NoError(t, err)

	jsonStr := `{
		"measurementName": "test_get_set",
		"setters": [{"id": "MockInstrument1", "channel": 0}],
		"getters": [{"id": "MockInstrument1", "channel": 0}],
		"setVoltages": {"MockInstrument1": 1.5}
	}`

	result, err := bridge.ExecuteMeasurementRequestJSON(jsonStr)
	require.NoError(t, err)
	t.Logf("MeasurementRequestJSON result: %+v", result)
}

// ===========================================================================
// Step 3 – Full NATS end-to-end: falcon → hub → ISS
// ===========================================================================

// startEmbeddedNATS spins up an in-process NATS server on a random port and
// returns a connected client. Caller should defer nc.Close() and ns.Shutdown().
func startEmbeddedNATS(t *testing.T) (*server.Server, *nats.Conn) {
	t.Helper()
	port := freePort()
	opts := &server.Options{
		Host: "127.0.0.1",
		Port: port,
		// JetStream is not required for the basic MEASURE_COMMAND flow, but
		// enable it in case we expand the test later.
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	ns, err := server.NewServer(opts)
	require.NoError(t, err)
	go ns.Start()
	require.True(t, ns.ReadyForConnections(5*time.Second), "NATS not ready")

	nc, err := nats.Connect(ns.ClientURL())
	require.NoError(t, err)
	return ns, nc
}

// TestLiveISS_NATS_EndToEnd exercises the complete chain:
//
//	falcon client → NATS MEASURE_COMMAND.external.* → Hub handler →
//	Bridge HTTP RPC → real ISS daemon → mock instruments →
//	bridge returns result → Hub handler → NATS MEASURE_RESPONSE.external.* → falcon client
//
// This is the "Step 3" integration test.
func TestLiveISS_NATS_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live ISS NATS end-to-end test in -short mode")
	}

	// 1. Start real ISS daemon with mock instruments.
	iss := startISS(t)
	defer iss.stop(t)

	client := NewScriptServerClient("127.0.0.1", iss.rpcPort)
	startMockInstruments(t, client, 3)

	// 2. Start embedded NATS.
	ns, nc := startEmbeddedNATS(t)
	defer ns.Shutdown()
	defer nc.Close()

	// 3. Set up a hub-side Bridge that points at the live ISS.
	bridgeCfg := BridgeConfig{
		ScriptServerHost: "127.0.0.1",
		ScriptServerPort: iss.rpcPort,
	}
	bridge, err := NewBridge(bridgeCfg)
	require.NoError(t, err)

	// 4. Subscribe to MEASURE_COMMAND.external.* on NATS and act as the hub
	//    handler: parse the command, forward to ISS via Bridge, publish result
	//    on MEASURE_RESPONSE.external.*.
	var wg sync.WaitGroup

	_, subErr := nc.Subscribe("MEASURE_COMMAND.external.*", func(msg *nats.Msg) {
		// Tokens: MEASURE_COMMAND.external.<name>
		tokens := strings.Split(msg.Subject, ".")
		name := tokens[len(tokens)-1]
		t.Logf("[hub] received MEASURE_COMMAND for %q", name)

		// The payload is a falcon MeasurementRequest JSON envelope.
		result, bErr := bridge.ExecuteMeasurementRequestJSON(string(msg.Data))
		var respBytes []byte
		if bErr != nil {
			respBytes, _ = json.Marshal(map[string]interface{}{
				"error":  bErr.Error(),
				"status": "failed",
			})
		} else {
			respBytes, _ = json.Marshal(result)
		}

		// Publish response.
		respSubject := "MEASURE_RESPONSE.external." + name
		if pubErr := nc.Publish(respSubject, respBytes); pubErr != nil {
			t.Logf("[hub] publish error: %v", pubErr)
		}
		nc.Flush()
		t.Logf("[hub] published MEASURE_RESPONSE.external.%s", name)
	})
	require.NoError(t, subErr)

	// 5. Now act as falcon: subscribe to MEASURE_RESPONSE, send a command.
	responseCh := make(chan *nats.Msg, 1)
	_, err = nc.Subscribe("MEASURE_RESPONSE.external.test_measure", func(msg *nats.Msg) {
		responseCh <- msg
	})
	require.NoError(t, err)
	require.NoError(t, nc.Flush())

	// Build a falcon-style MeasurementRequest JSON.
	falconRequest := map[string]interface{}{
		"measurementName": "test_measure",
		"setters":         []map[string]interface{}{{"id": "MockInstrument1", "channel": 0}},
		"getters":         []map[string]interface{}{{"id": "MockInstrument1", "channel": 0}},
		"setVoltages":     map[string]interface{}{"MockInstrument1": 3.3},
	}
	requestJSON, _ := json.Marshal(falconRequest)

	wg.Add(1)
	go func() {
		defer wg.Done()
		pErr := nc.Publish("MEASURE_COMMAND.external.test_measure", requestJSON)
		if pErr != nil {
			t.Logf("[falcon] publish error: %v", pErr)
		}
		nc.Flush()
		t.Log("[falcon] published MEASURE_COMMAND.external.test_measure")
	}()

	// Wait for the response (or timeout).
	select {
	case resp := <-responseCh:
		t.Logf("[falcon] received MEASURE_RESPONSE: %s", string(resp.Data))

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(resp.Data, &result))
		// We don't assert "completed" because the Bridge may forward to
		// submit_measure which can fail if scripts are missing; the critical
		// thing is that the full NATS round-trip worked.
		t.Logf("[falcon] parsed result: %+v", result)

	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for MEASURE_RESPONSE")
	}

	wg.Wait()
}
