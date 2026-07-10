//go:build windows

package ui

import (
	"fmt"
	"os/exec"
)

// relaunchCmd builds the detached helper that waits for this instance to exit
// then relaunches the binary. Windows has no fork/exec or setsid: cmd.exe gives
// us the timeout (sleep) plus start, and "start" launches the new process
// independently of this one. The timeout mirrors the 1s grace the Unix build
// gives the old instance to free its resources.
func relaunchCmd(exe string) *exec.Cmd {
	return exec.Command("cmd", "/c", fmt.Sprintf("timeout /t 1 /nobreak >nul & start \"\" %q", exe))
}
