//go:build !windows

package sysproc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func shell(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", line)
}

func restart(exe string, args []string) error { return syscall.Exec(exe, args, os.Environ()) }

func mpvSocket(runtimeDir string, pid int) string {
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, fmt.Sprintf("meofishes-mpv-%d.sock", pid))
}
