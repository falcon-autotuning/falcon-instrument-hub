//go:build !windows

package handlers

import (
	"os"
	"os/exec"
	"syscall"
)

// setProcAttr configures process attributes for Unix systems.
// Setpgid creates a new process group so we can kill the entire group on shutdown.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// sendTermSignal sends SIGTERM to gracefully terminate a process on Unix.
func sendTermSignal(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
