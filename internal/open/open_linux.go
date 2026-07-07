//go:build linux

package open

import "os/exec"

func openCmd(url string) *exec.Cmd {
	return exec.Command("xdg-open", url)
}
