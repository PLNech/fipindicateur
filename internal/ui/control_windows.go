//go:build windows

package ui

import (
	"fmt"
	"os"
)

// The control socket is a Unix-domain-socket feature (Linux/macOS/BSD). On
// Windows the server is a no-op and the client subcommands report that they are
// not supported, so the cross-platform entrypoint still compiles and runs.

func (a *App) startControlServer() {}

func (a *App) stopControlServer() {}

// RunControlClient reports that the control CLI is unavailable on Windows.
func RunControlClient(args []string) int {
	fmt.Fprintln(os.Stderr, "les commandes fip ne sont pas prises en charge sur Windows")
	return 1
}
