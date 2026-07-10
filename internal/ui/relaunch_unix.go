//go:build !windows

package ui

import (
	"fmt"
	"os/exec"
	"syscall"
)

// relaunchCmd builds the detached helper that waits for this instance to exit
// then execs the (possibly freshly installed) binary. Setsid puts the helper in
// its own session so it survives the dying parent; /bin/sh gives us the sleep +
// exec sequencing.
func relaunchCmd(exe string) *exec.Cmd {
	cmd := exec.Command("/bin/sh", "-c", fmt.Sprintf("sleep 1; exec %q", exe))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // detach from the dying parent
	return cmd
}
