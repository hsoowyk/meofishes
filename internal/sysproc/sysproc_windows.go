//go:build windows

package sysproc

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func shell(ctx context.Context, line string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd", "/C", line)
}

func child(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func restart(exe string, args []string) error {
	cmd := exec.Command(exe, args[1:]...)
	return cmd.Start()
}

func mpvSocket(runtimeDir string, pid int) string {
	return fmt.Sprintf(`\\.\pipe\meofishes-mpv-%d`, pid)
}
