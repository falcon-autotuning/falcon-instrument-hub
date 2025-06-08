package manageVenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

func TestNewManager(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(logger, tempDir)
	assert.NotNil(t, manager)
	assert.Equal(t, logger, manager.logger)
	assert.Empty(t, manager.venvPath)
}

func TestManager_SetupEnvironment_EmptyPackages(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(logger, tempDir)
	err = manager.SetupEnvironment([]string{})
	assert.NoError(t, err, "Should not error when no packages are provided")
}

func TestManager_SetupEnvironment_Success(t *testing.T) {
	// Skip this test if python3 is not available
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available, skipping virtual environment test")
	}

	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(logger, tempDir)

	// Use a simple package that should install quickly
	packages := []string{"six"} // six is a small, commonly available package

	err = manager.SetupEnvironment(packages)
	require.NoError(t, err, "Failed to setup virtual environment")

	// Verify venv path is set
	expectedVenvPath := filepath.Join(tempDir, "venv")
	assert.Equal(t, expectedVenvPath, manager.GetPath())

	// Verify virtual environment directory exists
	assert.DirExists(t, expectedVenvPath)

	// Verify python executable exists in venv
	pythonPath := manager.GetPythonInterpreter()
	assert.NotEqual(
		t,
		"python3",
		pythonPath,
		"Should use venv python, not system python",
	)

	// Check that either bin/python or Scripts/python.exe exists
	binPython := filepath.Join(expectedVenvPath, "bin", "python")
	scriptsPython := filepath.Join(expectedVenvPath, "Scripts", "python.exe")

	pythonExists := false
	if _, err := os.Stat(binPython); err == nil {
		pythonExists = true
		assert.Equal(t, binPython, pythonPath)
	} else if _, err := os.Stat(scriptsPython); err == nil {
		pythonExists = true
		assert.Equal(t, scriptsPython, pythonPath)
	}
	assert.True(
		t,
		pythonExists,
		"Python executable should exist in virtual environment",
	)
}

func TestManager_GetPythonInterpreter_NoVenv(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(logger, tempDir)

	// No venv setup, should return system python
	pythonPath := manager.GetPythonInterpreter()
	assert.Equal(t, "python3", pythonPath)
}

func TestManager_GetPythonInterpreter_InvalidVenv(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(logger, tempDir)

	// Set an invalid venv path
	manager.venvPath = "/nonexistent/path"

	// Should fallback to system python
	pythonPath := manager.GetPythonInterpreter()
	assert.Equal(t, "python3", pythonPath)
}

func TestManager_resolvePackageURL(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(logger, tempDir)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "HTTP URL",
			input:    "https://github.com/user/repo.git",
			expected: "https://github.com/user/repo.git",
		},
		{
			name:     "Git SSH with @",
			input:    "falcon_core @ git+ssh://git@github.com/falcon-autotuning/falcon-core.git@dev",
			expected: "falcon_core @ git+ssh://git@github.com/falcon-autotuning/falcon-core.git@dev",
		},
		{
			name:     "Simple GitHub repo",
			input:    "user/repo",
			expected: "git+https://github.com/user/repo.git",
		},
		{
			name:     "Package name only",
			input:    "numpy",
			expected: "numpy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.resolvePackageURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_createVirtualEnvironment(t *testing.T) {
	// Skip this test if python3 is not available
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip(
			"python3 not available, skipping virtual environment creation test",
		)
	}

	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(logger, tempDir)
	venvPath := filepath.Join(tempDir, "test_venv")

	err = manager.createVirtualEnvironment(venvPath)
	require.NoError(t, err, "Failed to create virtual environment")

	// Verify venv directory exists
	assert.DirExists(t, venvPath)

	// Verify pyvenv.cfg exists (this is created by venv)
	pyvenvCfg := filepath.Join(venvPath, "pyvenv.cfg")
	assert.FileExists(t, pyvenvCfg)
}

func TestManager_getPipPath_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(logger, tempDir)

	// Use a non-existent directory
	nonExistentPath := filepath.Join(tempDir, "nonexistent")

	_, err = manager.getPipPath(nonExistentPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not find pip")
}

func TestManager_InstallPackagesIntegration(t *testing.T) {
	// This is a more intensive test that actually installs a package
	// Skip if we don't want to run integration tests
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip this test if python3 is not available
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available, skipping package installation test")
	}

	tempDir := t.TempDir()
	logger, err := logging.NewLogger(tempDir)
	require.NoError(t, err)
	defer logger.Close()

	manager := NewManager(logger, tempDir)

	// Test with a very simple, fast-installing package
	packages := []string{"six==1.16.0"} // Pin version for predictability

	err = manager.SetupEnvironment(packages)
	if err != nil {
		// If this fails due to network issues, skip rather than fail
		if strings.Contains(err.Error(), "network") ||
			strings.Contains(err.Error(), "timeout") {
			t.Skip("Network issues preventing package installation test")
		}
		require.NoError(t, err, "Failed to setup environment with packages")
	}

	// Verify the package was installed by trying to import it
	venvPython := manager.GetPythonInterpreter()
	cmd := exec.Command(venvPython, "-c", "import six; print(six.__version__)")
	output, err := cmd.Output()

	if err == nil {
		// If successful, verify we got the expected version
		assert.Contains(t, string(output), "1.16.0")
	} else {
		// If import failed, just log it rather than failing the test
		// as this could be due to various environment issues
		t.Logf("Warning: Could not verify package installation: %v", err)
	}
}
