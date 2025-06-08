package venv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/falcon-autotuning/instrument-server/runtime/internal/logging"
)

const (
	defaultPackage = "instrument-templates @ git+ssh://git@github.com/falcon-autotuning/instrument-templates.git@dev"
	logSource      = "VENV"
)

// Manager handles Python virtual environment creation and management
type Manager struct {
	logger   *logging.Logger
	venvPath string
}

// NewManager creates a new virtual environment manager
func NewManager(logger *logging.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// SetupEnvironment creates a Python virtual environment with the specified
// packages
func (m *Manager) SetupEnvironment(outputPath string, packages []string) error {
	// Create venv directory in the output path
	venvDir := filepath.Join(outputPath, "venv")
	m.venvPath = venvDir

	m.logger.Info(
		"VENV",
		fmt.Sprintf("Creating Python virtual environment at: %s", venvDir),
	)

	// Create the virtual environment using python3.13
	if err := m.createVirtualEnvironment(venvDir); err != nil {
		return fmt.Errorf("failed to create virtual environment: %v", err)
	}

	activateVenvScript := filepath.Join(venvDir, "bin/activate")
	// Create complete package list including falcon_core
	allPackages := append(
		packages, defaultPackage,
	)

	// Install each package using uv
	for _, pkg := range allPackages {
		m.logger.Info(
			"VENV",
			fmt.Sprintf("Installing package with uv: %s", pkg),
		)
		installURL := m.resolvePackageURL(pkg)

		cmdStr := fmt.Sprintf(
			". %s && uv pip install '%s'",
			activateVenvScript,
			installURL,
		)

		m.logger.Info("VENV", fmt.Sprintf("Executing: sh -c \"%s\"", cmdStr))
		cmd := exec.Command("sh", "-c", cmdStr)
		// cmd.Dir = workingDirectory // Optional: Set working directory if
		// necessary

		output, err := cmd.CombinedOutput()
		if err != nil {
			m.logger.Error(
				"VENV",
				fmt.Sprintf("Failed to install package %s: %v", pkg, err),
			)
			m.logger.Error("VENV", fmt.Sprintf("Output: %s", string(output)))
			return fmt.Errorf(
				"failed to install package %s: %v. Output: %s",
				pkg,
				err,
				string(output),
			)
		}
		m.logger.Info(
			"VENV",
			fmt.Sprintf(
				"Successfully installed package: %s\nOutput: %s",
				pkg,
				string(output),
			),
		)
	}

	m.logger.Info(
		"VENV",
		fmt.Sprintf(
			"Python virtual environment packages installed in: %s",
			venvDir,
		),
	)
	return nil
}

// GetPath returns the virtual environment path
func (m *Manager) GetPath() string {
	return m.venvPath
}

// GetPythonInterpreter returns the path to the Python interpreter in the
// virtual environment
func (m *Manager) GetPythonInterpreter() string {
	if m.venvPath == "" {
		return "python3" // fallback to system python
	}

	// Try to find python in the virtual environment
	if _, err := os.Stat(filepath.Join(m.venvPath, "bin", "python")); err == nil {
		return filepath.Join(m.venvPath, "bin", "python")
	} else if _, err := os.Stat(filepath.Join(m.venvPath, "Scripts", "python.exe")); err == nil {
		return filepath.Join(m.venvPath, "Scripts", "python.exe")
	}

	m.logger.Warn(
		logSource,
		fmt.Sprintf(
			"Could not find python in virtual environment %s, falling back to system python",
			m.venvPath,
		),
	)
	return "python3"
}

// createVirtualEnvironment creates the virtual environment
func (m *Manager) createVirtualEnvironment(venvDir string) error {
	cmd := exec.Command("uv", "venv", venvDir)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// getPipPath finds the pip executable in the virtual environment
func (m *Manager) getPipPath(venvDir string) (string, error) {
	if _, err := os.Stat(filepath.Join(venvDir, "bin", "pip")); err == nil {
		return filepath.Join(venvDir, "bin", "pip"), nil
	} else if _, err := os.Stat(filepath.Join(venvDir, "Scripts", "pip.exe")); err == nil {
		return filepath.Join(venvDir, "Scripts", "pip.exe"), nil
	}
	return "", fmt.Errorf("could not find pip in virtual environment")
}

// upgradePip upgrades pip in the virtual environment
func (m *Manager) upgradePip(pipPath string) error {
	m.logger.Info(logSource, "Upgrading pip in virtual environment")
	cmd := exec.Command(pipPath, "install", "--upgrade", "pip")
	return cmd.Run()
}

// installPackages installs the specified packages plus falcon_core
func (m *Manager) installPackages(pipPath string, packages []string) error {
	// Create complete package list including falcon_core
	allPackages := append(packages, defaultPackage)

	for _, pkg := range allPackages {
		m.logger.Info(logSource, fmt.Sprintf("Installing package: %s", pkg))

		installURL := m.resolvePackageURL(pkg)

		cmd := exec.Command(pipPath, "install", installURL)
		output, err := cmd.CombinedOutput()
		if err != nil {
			m.logger.Error(
				logSource,
				fmt.Sprintf("Failed to install package %s: %v", pkg, err),
			)
			m.logger.Error(logSource, fmt.Sprintf("Output: %s", string(output)))
			return fmt.Errorf("failed to install package %s: %v", pkg, err)
		}
		m.logger.Info(
			logSource,
			fmt.Sprintf("Successfully installed package: %s", pkg),
		)
	}

	return nil
}

// resolvePackageURL converts a package specification to an installation URL
func (m *Manager) resolvePackageURL(pkg string) string {
	if len(pkg) >= 4 && (pkg[:4] == "http" || strings.Contains(pkg, "@")) {
		// Package already has a URL or git specification
		return pkg
	}

	// Check if it looks like a GitHub repo (contains a slash)
	if strings.Contains(pkg, "/") {
		// Assume it's a GitHub repo in format "owner/repo"
		return fmt.Sprintf("git+https://github.com/%s.git", pkg)
	}

	// Otherwise, treat it as a regular PyPI package
	return pkg
}
