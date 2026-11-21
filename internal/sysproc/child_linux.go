package sysproc

import (
	"os/exec"
	"syscall"
)

// child: the kernel kills the child when meofishes dies (even on a crash).
func child(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
