//go:build windows

package handlers

import (
	"os"
	"os/exec"
)

// setProcAttr is a no-op on Windows (Setpgid is not supported).
func setProcAttr(cmd *exec.Cmd) {
	// Windows does not support Setpgid.
	// Process cleanup is handled differently on Windows.
}

// sendTermSignal on Windows falls back to Kill since SIGTERM doesn't exist.
func sendTermSignal(p *os.Process) error {
	return p.Kill()
}
