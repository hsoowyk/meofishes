//go:build !linux && !windows

package sysproc

import "os/exec"

// child: macOS/BSD have no parent-death signal; children are stopped by
// meofishes when it quits normally.
func child(cmd *exec.Cmd) {}
