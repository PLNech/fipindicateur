//go:build darwin

package open

import "os/exec"

func openCmd(url string) *exec.Cmd {
	return exec.Command("open", url)
}
