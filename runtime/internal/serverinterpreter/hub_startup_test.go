// Package serverinterpreter provides hub binary startup integration tests.
//
// These tests verify that the instrument-hub binary starts correctly when
// given a --hub-config file whose format matches the YAML written by the
// instrument-controller data-retrieval integration test (WriteConfigFile).
//
// Skip all tests here with: go test ./internal/serverinterpreter/... -short
//
// Requirements for TestHubBinary_StartWithConfigFile:
//   - instrument-hub binary built at runtime/bin/instrument-hub  (make build-go)
//
// Requirements for TestHubBinary_StartWithConfigFile_WithISS (additional):
//   - instrument-script-server binary at ../../../../instrument-script-server/build/instrument-script-server
package serverinterpreter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Paths
// ---------------------------------------------------------------------------

// hubBinary returns the path to the compiled instrument-hub binary.
// Resolution order:
//  1. HUB_BINARY env var (explicit override)
//  2. runtime/bin/instrument-hub  (local build via 'make build-go')
//  3. /opt/falcon/bin/instrument-hub (installed system binary)
func hubBinary() string {
	if v := os.Getenv("HUB_BINARY"); v != "" {
		return v
	}
	_, thisFile, _, _ := runtime.Caller(0)
	local := filepath.Join(filepath.Dir(thisFile), "..", "..", "bin", "instrument-hub")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	return "/opt/falcon/bin/instrument-hub"
}

// vcpkgLibPath returns the directory containing vcpkg shared libraries needed
// by the instrument-hub and instrument-script-server binaries.
// Resolution order:
//  1. VCPKG_LIB env var (explicit override, matches what the user sets manually)
//  2. instrument-script-server/vcpkg_installed/x64-linux-dynamic/lib (sibling repo)
func vcpkgLibPath() string {
	if v := os.Getenv("VCPKG_LIB"); v != "" {
		return v
	}
	return filepath.Join(repoRoot(), "instrument-script-server",
		"vcpkg_installed", "x64-linux-dynamic", "lib")
}

// hubTestDataDir returns the path to falcon-instrument-hub/test_data.
func hubTestDataDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// ../../../test_data  =>  falcon-instrument-hub/test_data/
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "test_data")
}

// hubVcpkgLibPath returns the falcon-instrument-hub vcpkg lib dir, which
// contains libfalcon-core-c-api.so and other hub-specific shared libraries.
func hubVcpkgLibPath() string {
	if v := os.Getenv("HUB_VCPKG_LIB"); v != "" {
		return v
	}
	_, thisFile, _, _ := runtime.Caller(0)
	// hub root = serverinterpreter/../../../  =>  falcon-instrument-hub/
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..",
		"vcpkg_installed", "x64-linux-dynamic", "lib")
}

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

// validDeviceConfigYAML is a 2-dot 1-charge-sensor device config in the flat
// YAML format expected by the hub's Go config loader (internal/config/types.go).
// The topology mirrors the 2-dot-1-chargesensor fixture used by the
// instrument-controller data-retrieval integration test.
const validDeviceConfigYAML = `ScreeningGates: "S1;S2;S3"
PlungerGates: "P1;P2;P3"
Ohmics: "O1;O2;O3;O4"
BarrierGates: "B1;B2;B3;B4;B5"
ReservoirGates: "R1;R2;R3;R4"
num-unique-channels: 2
groups:
  group1:
    Name: "I_O1"
    NumDots: 2
    ScreeningGates: "S1;S2"
    ReservoirGates: "R1;R2"
    PlungerGates: "P1;P2"
    BarrierGates: "B1;B2;B3"
    Order: "O1;R1;B1;P1;B2;P2;B3;R2;O2"
  group2:
    Name: "I_O3"
    NumDots: 1
    ScreeningGates: "S2;S3"
    ReservoirGates: "R3;R4"
    PlungerGates: "P3"
    BarrierGates: "B4;B5"
    Order: "O3;R3;B4;P3;B5;R4;O4"
wiringDC:
  S1: {resistance: 1000.0, capacitance: 1.0e-12}
  S2: {resistance: 1000.0, capacitance: 1.0e-12}
  S3: {resistance: 1000.0, capacitance: 1.0e-12}
  P1: {resistance: 1000.0, capacitance: 1.0e-12}
  P2: {resistance: 1000.0, capacitance: 1.0e-12}
  P3: {resistance: 1000.0, capacitance: 1.0e-12}
  O1: {resistance: 1000.0, capacitance: 1.0e-12}
  O2: {resistance: 1000.0, capacitance: 1.0e-12}
  O3: {resistance: 1000.0, capacitance: 1.0e-12}
  O4: {resistance: 1000.0, capacitance: 1.0e-12}
  R1: {resistance: 1000.0, capacitance: 1.0e-12}
  R2: {resistance: 1000.0, capacitance: 1.0e-12}
  R3: {resistance: 1000.0, capacitance: 1.0e-12}
  R4: {resistance: 1000.0, capacitance: 1.0e-12}
  B1: {resistance: 1000.0, capacitance: 1.0e-12}
  B2: {resistance: 1000.0, capacitance: 1.0e-12}
  B3: {resistance: 1000.0, capacitance: 1.0e-12}
  B4: {resistance: 1000.0, capacitance: 1.0e-12}
  B5: {resistance: 1000.0, capacitance: 1.0e-12}
`

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// hubProcess manages a running instrument-hub binary subprocess.
type hubProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

func (p *hubProcess) stop(t *testing.T) {
	t.Helper()
	p.cancel()
	_ = p.cmd.Wait()
}

// writeHubConfigFile writes an instrument_hub_config.yaml to configPath.
// The key/value format is identical to what WriteConfigFile generates in
// the instrument-controller data-retrieval integration test:
//
//	ofs << "wiremap: "          << wiremapPath          << "\n";
//	ofs << "quantum-dot-config: " << deviceConfigPath   << "\n";
//	ofs << "inst-config: "      << a << ";" << b        << "\n";
//	ofs << "instrument-server-port: 5555\n";
//	ofs << "local-database: "   << dataDir              << "\n";
//	ofs << "user-measurement-luas: " << luaDir          << "\n";
func writeHubConfigFile(
	t *testing.T,
	configPath, wiremapPath, deviceConfigPath, instConfig,
	natsURL, localDatabase, userMeasurementLuas string,
	instrumentServerPort int,
) {
	t.Helper()
	var b strings.Builder
	b.WriteString("wiremap: " + wiremapPath + "\n")
	b.WriteString("quantum-dot-config: " + deviceConfigPath + "\n")
	b.WriteString("inst-config: " + instConfig + "\n")
	b.WriteString(fmt.Sprintf("instrument-server-port: %d\n", instrumentServerPort))
	b.WriteString("local-database: " + localDatabase + "\n")
	b.WriteString("user-measurement-luas: " + userMeasurementLuas + "\n")
	if natsURL != "" {
		b.WriteString("nats-url: " + natsURL + "\n")
	}
	require.NoError(t, os.WriteFile(configPath, []byte(b.String()), 0o644))
	t.Logf("hub config written to %s:\n%s", configPath, b.String())
}

// startHubBinary launches the instrument-hub binary with --hub-config,
// --working-dir, and --iss-lib-path, and sets LD_LIBRARY_PATH so the binary
// can find its shared libraries.
//
// The setup mirrors the manual invocation:
//
//	VCPKG_LIB=.../vcpkg_installed/x64-linux-dynamic/lib
//	LD_LIBRARY_PATH=/opt/falcon/lib:$VCPKG_LIB \
//	  /opt/falcon/bin/instrument-hub start \
//	    --iss-lib-path "$VCPKG_LIB" --working-dir /tmp/hub-test
//
// Pass issBinPath="" to omit --iss-binary; noISS=true to add --no-iss.
// Returns a hubProcess; caller must call stop().
func startHubBinary(t *testing.T, configPath, workDir, issBinPath string, noISS bool) *hubProcess {
	t.Helper()
	bin := hubBinary()
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Skipf("instrument-hub binary not found at %s – run 'make build-go' first or install to /opt/falcon", bin)
	}

	libPath := vcpkgLibPath()

	args := []string{
		"start",
		"--hub-config", configPath,
		"--iss-lib-path", libPath,
		"--working-dir", workDir,
	}
	if issBinPath != "" {
		args = append(args, "--iss-binary", issBinPath)
	}
	if noISS {
		args = append(args, "--no-iss")
	}

	// Build LD_LIBRARY_PATH: prepend /opt/falcon/lib, the hub vcpkg lib dir
	// (libfalcon-core-c-api.so), and the ISS vcpkg lib dir to whatever the
	// current environment already has.
	existingLD := os.Getenv("LD_LIBRARY_PATH")
	newLD := "/opt/falcon/lib:" + hubVcpkgLibPath() + ":" + libPath
	if existingLD != "" {
		newLD += ":" + existingLD
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+newLD)
	require.NoError(t, cmd.Start(), "failed to start instrument-hub")
	t.Logf("instrument-hub started (pid=%d) args=%v LD_LIBRARY_PATH=%s", cmd.Process.Pid, args, newLD)
	return &hubProcess{cmd: cmd, cancel: cancel}
}

// waitForHubStatus subscribes to STATUS.instrument-server on the given NATS
// URL and blocks until the hub publishes at least one message or timeout elapses.
// The STATUS handler inside the hub fires every 4 s only after all handlers
// are fully set up, so a successful return means the hub is ready.
func waitForHubStatus(t *testing.T, natsURL string, timeout time.Duration) {
	t.Helper()
	nc, err := nats.Connect(natsURL, nats.Timeout(5*time.Second))
	require.NoError(t, err, "test NATS client failed to connect")
	defer nc.Close()

	ready := make(chan struct{}, 1)
	sub, err := nc.Subscribe("STATUS.instrument-server", func(msg *nats.Msg) {
		t.Logf("hub STATUS received: %s", string(msg.Data))
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()
	require.NoError(t, nc.Flush())

	select {
	case <-ready:
		t.Log("hub is ready (STATUS.instrument-server received)")
	case <-time.After(timeout):
		t.Fatalf("hub did not publish STATUS.instrument-server within %s", timeout)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestHubBinary_StartWithConfigFile verifies that the instrument-hub binary
// starts, connects to NATS, loads the device config, and responds to
// DEVICE_CONFIG_REQUEST when given a --hub-config file.
//
// The config file format matches exactly what WriteConfigFile in the
// instrument-controller data-retrieval integration test produces:
//
//	wiremap: <abs-path>/2-dot-1-chargesensor-wiremap.yml
//	quantum-dot-config: <abs-path>/2-dot-1-chargesensor.yml
//	inst-config: <abs-path>/multimeter-config.yml;<abs-path>/source-config.yml
//	instrument-server-port: 5555
//	local-database: <abs-path>/data
//	user-measurement-luas: <abs-path>/lua
//
// --no-iss is passed so the instrument-script-server binary is not required.
func TestHubBinary_StartWithConfigFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping hub binary startup test in -short mode")
	}

	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	luaDir := filepath.Join(tmpDir, "lua")
	workDir := filepath.Join(tmpDir, "hub")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.MkdirAll(luaDir, 0o755))
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	// Write the device config in the flat YAML format the Go loader expects.
	deviceConfigPath := filepath.Join(tmpDir, "2-dot-1-chargesensor.yml")
	require.NoError(t, os.WriteFile(deviceConfigPath, []byte(validDeviceConfigYAML), 0o644))

	// Wiremap lives in test_data alongside the instrument configs.
	wiremapPath := filepath.Join(hubTestDataDir(), "2-dot-1-chargesensor-wiremap.yml")
	if _, err := os.Stat(wiremapPath); os.IsNotExist(err) {
		t.Skipf("test_data wiremap not found at %s", wiremapPath)
	}

	// inst-config: semicolon-separated paths just as in data-retrieval.cpp.
	// The hub passes these to ISS; since we run --no-iss the paths are stored
	// but no instrument loading is attempted.
	instConfig := strings.Join([]string{
		filepath.Join(hubTestDataDir(), "multimeter-config.yml"),
		filepath.Join(hubTestDataDir(), "source-config.yml"),
	}, ";")

	// Start an embedded NATS server so we know the URL before writing the config.
	ns, testNC := startEmbeddedNATS(t)
	defer ns.Shutdown()
	defer testNC.Close()
	natsURL := ns.ClientURL()

	// Write the hub config file.
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	writeHubConfigFile(t, configPath,
		wiremapPath, deviceConfigPath, instConfig,
		natsURL, dataDir, luaDir, 5555)

	// Start the hub binary without ISS.
	hub := startHubBinary(t, configPath, workDir, "", true)
	defer hub.stop(t)

	// The hub's STATUS handler publishes every 4 s once all handlers are up.
	// 15 s is generous headroom for slow CI machines.
	waitForHubStatus(t, natsURL, 15*time.Second)

	// --- Verify DEVICE_CONFIG_REQUEST → DEVICE_CONFIG_RESPONSE round-trip ---
	respCh := make(chan *nats.Msg, 1)
	sub, err := testNC.Subscribe("FALCON.DEVICE_CONFIG_RESPONSE", func(msg *nats.Msg) {
		respCh <- msg
	})
	require.NoError(t, err)
	defer sub.Unsubscribe()
	require.NoError(t, testNC.Flush())

	reqPayload, _ := json.Marshal(map[string]interface{}{"timestamp": time.Now().UnixMicro()})
	require.NoError(t, testNC.Publish("INSTRUMENTHUB.DEVICE_CONFIG_REQUEST", reqPayload))
	require.NoError(t, testNC.Flush())

	select {
	case msg := <-respCh:
		t.Logf("DEVICE_CONFIG_RESPONSE: %s", string(msg.Data))
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(msg.Data, &resp))
		assert.NotEmpty(t, resp, "expected a non-empty device config response")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for FALCON.DEVICE_CONFIG_RESPONSE")
	}
}

// TestHubBinary_StartWithConfigFile_WithISS extends the above test to verify
// that the hub also boots the instrument-script-server daemon at the RPC port
// specified by instrument-server-port in the config file.
//
// Requires the instrument-script-server binary to be present (see issBinary()).
// Skips instrument-loading assertions because the test_data instrument configs
// reference generated API files that may not be present; the key assertion is
// that the ISS daemon is reachable on the configured port.
func TestHubBinary_StartWithConfigFile_WithISS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping hub+ISS startup test in -short mode")
	}

	issBin := issBinary()
	if _, err := os.Stat(issBin); os.IsNotExist(err) {
		t.Skipf("instrument-script-server binary not found at %s – build ISS first", issBin)
	}

	// Stop any leftover daemon to avoid PID-file conflicts.
	_ = exec.Command(issBin, "daemon", "stop").Run()
	time.Sleep(500 * time.Millisecond)

	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	luaDir := filepath.Join(tmpDir, "lua")
	workDir := filepath.Join(tmpDir, "hub")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, os.MkdirAll(luaDir, 0o755))
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	deviceConfigPath := filepath.Join(tmpDir, "2-dot-1-chargesensor.yml")
	require.NoError(t, os.WriteFile(deviceConfigPath, []byte(validDeviceConfigYAML), 0o644))

	wiremapPath := filepath.Join(hubTestDataDir(), "2-dot-1-chargesensor-wiremap.yml")
	if _, err := os.Stat(wiremapPath); os.IsNotExist(err) {
		t.Skipf("test_data wiremap not found at %s", wiremapPath)
	}

	// Allocate a free port for the ISS RPC server. The hub passes this to the
	// ISS daemon via INSTRUMENT_SCRIPT_SERVER_RPC_PORT (set from instrument-server-port).
	issRPCPort := freePort()

	instConfig := strings.Join([]string{
		filepath.Join(hubTestDataDir(), "multimeter-config.yml"),
		filepath.Join(hubTestDataDir(), "source-config.yml"),
	}, ";")

	ns, testNC := startEmbeddedNATS(t)
	defer ns.Shutdown()
	defer testNC.Close()
	natsURL := ns.ClientURL()

	configPath := filepath.Join(tmpDir, "test-config.yaml")
	writeHubConfigFile(t, configPath,
		wiremapPath, deviceConfigPath, instConfig,
		natsURL, dataDir, luaDir, issRPCPort)

	// Hub auto-starts the ISS daemon using the provided binary.
	hub := startHubBinary(t, configPath, workDir, issBin, false)
	defer func() {
		hub.stop(t)
		// Best-effort daemon cleanup.
		_ = exec.Command(issBin, "daemon", "stop").Run()
		time.Sleep(300 * time.Millisecond)
	}()

	// Wait for hub to finish its startup sequence.
	waitForHubStatus(t, natsURL, 20*time.Second)

	// Verify the ISS daemon is reachable on the port specified in the config.
	issClient := NewScriptServerClient("127.0.0.1", issRPCPort)
	deadline := time.Now().Add(15 * time.Second)
	var issReady bool
	for time.Now().Before(deadline) {
		_, err := issClient.ListInstruments()
		if err == nil {
			issReady = true
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	require.True(t, issReady,
		"ISS daemon should be reachable on port %d (configured via instrument-server-port in hub config)",
		issRPCPort)
	t.Logf("ISS daemon is reachable on port %d", issRPCPort)
}
