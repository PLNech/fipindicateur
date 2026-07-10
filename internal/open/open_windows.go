//go:build windows

package open

import "os/exec"

// openCmd launches a URL in the default browser via the Windows shell URL
// handler. rundll32 url.dll,FileProtocolHandler is the classic, quoting-safe
// way to hand a full URL (query string and all) to whatever is registered for
// http/https, without the argument-parsing quirks of `cmd /c start`.
func openCmd(url string) *exec.Cmd {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
}
