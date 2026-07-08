package stats

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PLNech/fipindicateur/internal/events"
	"github.com/PLNech/fipindicateur/internal/open"
)

//go:embed report.html.tmpl
var reportTemplate string

// dataPlaceholder is replaced by the marshaled Report JSON. json.Marshal
// escapes <, > and & by default, so the blob is safe to embed inside <script>.
const dataPlaceholder = "__FIP_DATA__"

// Render produces the self-contained HTML report from the derived model.
func (r Report) Render() ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	html := strings.Replace(reportTemplate, dataPlaceholder, string(data), 1)
	return []byte(html), nil
}

// Generate loads the default events log, derives the report, and renders it.
// A missing log is not an error: it renders an empty (all-zero) report so the
// user always sees a page, never a crash.
func Generate(now time.Time) ([]byte, Report, error) {
	path, err := events.DefaultPath()
	if err != nil {
		return nil, Report{}, err
	}
	evs, err := events.Load(path)
	if err != nil {
		return nil, Report{}, err
	}
	r := Build(evs, now)
	html, err := r.Render()
	return html, r, err
}

// ServeAndOpen serves the HTML on an ephemeral 127.0.0.1 port and opens it in
// the browser. It serves over HTTP (not file://) because Firefox on Ubuntu is
// a Snap and cannot read file:// under hidden dirs like ~/.local/share. The
// server shuts down a few seconds after the page is fetched, or after a hard
// timeout if the browser never connects. Blocking: callers wanting the tray to
// stay responsive should run it in a goroutine.
func ServeAndOpen(html []byte) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	var once sync.Once
	fetched := make(chan struct{})
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path != "/" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(html)
			once.Do(func() { close(fetched) })
		}),
	}
	go func() { _ = srv.Serve(ln) }()

	url := fmt.Sprintf("http://%s/", ln.Addr().String())
	open.URL(url)

	// Give the browser a moment to load the (self-contained) page, then stop.
	select {
	case <-fetched:
		time.Sleep(3 * time.Second)
	case <-time.After(30 * time.Second):
	}
	return srv.Close()
}

// RunCLI implements the `fipindicateur stats` subcommand. Flags: --out <path>
// writes the HTML to a file (for sharing/productising); --no-open suppresses
// the browser. With no flags it serves and opens. Returns a process exit code.
func RunCLI(args []string) int {
	var out string
	open := true
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--out", "-o":
			if i+1 < len(args) {
				i++
				out = args[i]
			}
		case "--no-open":
			open = false
		case "-h", "--help":
			fmt.Println("Usage: fipindicateur stats [--out <file.html>] [--no-open]")
			return 0
		}
	}

	html, r, err := Generate(time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "fipindicateur: stats: %v\n", err)
		return 1
	}

	if out != "" {
		if err := os.WriteFile(out, html, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "fipindicateur: write %s: %v\n", out, err)
			return 1
		}
		fmt.Printf("Rapport ecrit dans %s (%d sessions, %d evenements).\n", out, r.Totals.Sessions, r.Calibration.Events)
		if !open {
			return 0
		}
	}

	if open {
		if err := ServeAndOpen(html); err != nil {
			fmt.Fprintf(os.Stderr, "fipindicateur: serve: %v\n", err)
			return 1
		}
	}
	return 0
}
