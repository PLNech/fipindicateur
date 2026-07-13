//go:build !windows

package ui

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// controlListener holds the running server's socket. Kept in a package variable
// (like instanceLock) so there is exactly one per process; nil when no server
// is up. controlSocket is the path to unlink on shutdown.
var (
	controlListener net.Listener
	controlSocket   string
)

// controlSocketPath is the control socket location: XDG_RUNTIME_DIR when set
// (tmpfs, per-user), else the OS temp dir as a fallback. Same directory policy
// as the single-instance lock.
func controlSocketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "fipindicateur.sock")
}

// startControlServer opens the control socket and serves it in the background.
// Best-effort: a failure to bind is logged and the app runs on without it (the
// socket is a convenience, never load-bearing). We hold the single-instance
// lock by the time this runs, so any pre-existing socket file is stale (a
// crashed predecessor) and is removed before binding.
func (a *App) startControlServer() {
	path := controlSocketPath()
	_ = os.Remove(path) // clear a stale socket from a crashed instance
	ln, err := net.Listen("unix", path)
	if err != nil {
		log.Printf("ui: control socket: %v", err)
		return
	}
	if err := os.Chmod(path, 0o600); err != nil {
		log.Printf("ui: control socket chmod: %v", err)
	}
	controlListener = ln
	controlSocket = path
	go a.serveControl(ln)
}

// serveControl accepts connections until the listener is closed on shutdown.
func (a *App) serveControl(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed (stopControlServer) or fatal accept error
		}
		go a.handleControlConn(conn)
	}
}

// handleControlConn reads command lines from one connection and writes each
// response back, until the client closes its write half (EOF) or an error
// occurs. One command per line; the client may send several before closing.
func (a *App) handleControlConn(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		resp := handleControl(a, strings.TrimSpace(sc.Text()))
		if _, err := io.WriteString(conn, resp); err != nil {
			return
		}
	}
}

// stopControlServer closes the listener and unlinks the socket file. Safe to
// call when no server is running.
func (a *App) stopControlServer() {
	if controlListener != nil {
		_ = controlListener.Close()
		controlListener = nil
	}
	if controlSocket != "" {
		_ = os.Remove(controlSocket)
		controlSocket = ""
	}
}

// RunControlClient is the `fip`/`fipindicateur` control subcommand: it connects
// to the running instance's socket, sends the args as one command line, streams
// the response to stdout and returns 0. When no instance is running (dial
// fails) it prints a French notice to stderr and returns 1.
func RunControlClient(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: fip <status|play|pause|toggle|stations|station id>")
		return 2
	}
	conn, err := net.Dial("unix", controlSocketPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "fipindicateur n'est pas lancé")
		return 1
	}
	defer conn.Close()
	fmt.Fprintln(conn, strings.Join(args, " "))
	// Signal end-of-command so the server's line loop returns and we read the
	// full (possibly multi-line) response to EOF.
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}
	_, _ = io.Copy(os.Stdout, conn)
	return 0
}
