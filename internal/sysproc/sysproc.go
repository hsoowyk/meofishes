// Package sysproc hides the operating-system differences of child
// processes, restarts and IPC endpoints (Linux, macOS, Windows).
package sysproc

import (
	"context"
	"os/exec"
)

// Shell returns a command that runs a shell line (sh -c / cmd /C).
func Shell(ctx context.Context, line string) *exec.Cmd { return shell(ctx, line) }

// Child makes a child process die with meofishes where the system allows
// it (Linux), and hides its console window (Windows).
func Child(cmd *exec.Cmd) { child(cmd) }

// Restart replaces (Unix) or relaunches (Windows) the running program.
// On Windows the caller must quit afterwards.
func Restart(exe string, args []string) error { return restart(exe, args) }

// MpvSocket is the IPC endpoint for mpv's --input-ipc-server.
func MpvSocket(runtimeDir string, pid int) string { return mpvSocket(runtimeDir, pid) }
