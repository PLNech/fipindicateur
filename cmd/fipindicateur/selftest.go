package main

// `fipindicateur drawer --selftest`: the panel wiring self-test. It loads the
// REAL page in the REAL webkit (the exact constructor the tray uses), then
// for each scenario state injects the state, walks every view, asserts the
// view renders non-empty content, programmatically clicks EVERY enabled
// button (plus the two sliders) and asserts a command reaches the Go side.
// Any JS error (the page's window.onerror posts js_error) fails the run.
// Headless-friendly: run under `xvfb-run -a` (see the Makefile's selftest
// target); exit code is non-zero on any failure.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PLNech/fipindicateur/internal/drawer"
)

// stTimeout bounds each wait for a page-side reaction. Generous: webkit under
// xvfb on CI can be slow to schedule (an unaccelerated software renderer).
const stTimeout = 6 * time.Second

// stMsg is the payload the injected helpers post (JSON in Command.Key).
type stMsg struct {
	Ev    string   `json:"ev"`   // pong | info | list | mark | miss
	View  string   `json:"view"` // requested view, echoed back
	Sec   string   `json:"sec"`  // active section element id
	Text  int      `json:"text"` // rendered text length of the active section
	Items []string `json:"items"`
	ID    string   `json:"id"`
}

// stHelpers is injected once: DOM-only helpers the Go runner drives via Eval.
// They post through the same "fip" channel as real user actions.
const stHelpers = `(function () {
  function sec() {
    return ['view-main', 'view-history', 'view-settings']
      .map(function (id) { return document.getElementById(id); })
      .find(function (s) { return !s.hidden; });
  }
  function post(o) { window.webkit.messageHandlers.fip.postMessage(JSON.stringify({ action: 'selftest', key: JSON.stringify(o) })); }
  function clickables() {
    var s = sec();
    var els = Array.prototype.slice.call(document.querySelectorAll('header.bar button'))
      .concat(Array.prototype.slice.call(s.querySelectorAll('button')));
    return els.filter(function (b) { return !b.disabled && !b.hidden && b.id !== 'close'; });
  }
  window.__stInfo = function (view) {
    var s = sec();
    post({ ev: 'info', view: view, sec: s ? s.id : '', text: s ? s.innerText.trim().length : -1 });
  };
  window.__stList = function () {
    post({ ev: 'list', items: clickables().map(function (b, i) { return b.id || b.className.split(' ')[0] + '[' + i + ']'; }) });
  };
  window.__stClick = function (i) {
    var els = clickables();
    var b = els[i];
    if (!b) { post({ ev: 'miss', id: String(i) }); return; }
    post({ ev: 'mark', id: b.id || b.className.split(' ')[0] + '[' + i + ']' });
    b.click();
    if (b.id === 'statsClear' || b.id === 'prefsClear') b.click(); // two-click confirm rows
  };
  window.__stRange = function (id, v) {
    var el = document.getElementById(id);
    post({ ev: 'mark', id: id });
    el.value = v;
    el.dispatchEvent(new Event('input'));
    el.dispatchEvent(new Event('change'));
  };
  post({ ev: 'pong' });
})();`

// localOnly are page-local controls: their click must not raise a JS error
// but is not expected to post a command (view navigation, the Sortie fold).
var localOnly = map[string]bool{
	"back": true, "navHistory": true, "navSettings": true, "outToggle": true,
}

// stScenario is one state fixture the whole click-walk runs against.
type stScenario struct {
	name  string
	state drawer.State
}

func stScenarios() []stScenario {
	base := mockState()

	noHist := base
	noHist.History = nil
	noUp := base
	noUp.Upcoming = nil
	noDevs := base
	noDevs.Devices = nil
	noDevs.AudioDevices = nil
	casting := base
	casting.Cast = drawer.Cast{
		Active: true, DeviceName: base.Devices[0], Playing: true,
		Volume: 34, Muted: false, VolumeKnown: true, ControlType: "master",
	}

	return []stScenario{
		{"diffusion inactive", base}, // base IS the not-casting case, kept explicit
		{"état complet", base},
		{"historique vide", noHist},
		{"à venir vide", noUp},
		{"aucun appareil", noDevs},
		{"diffusion active", casting},
	}
}

// selftest drives one run; separated from runSelftest for a readable exit path.
type selftest struct {
	d        *drawer.Drawer
	cmds     chan drawer.Command
	failures []string
}

func (t *selftest) failf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	t.failures = append(t.failures, msg)
	fmt.Println("  ÉCHEC: " + msg)
}

// next waits for one command from the page (any action).
func (t *selftest) next() (drawer.Command, bool) {
	select {
	case c := <-t.cmds:
		return c, true
	case <-time.After(stTimeout):
		return drawer.Command{}, false
	}
}

// nextMsg waits for the next selftest helper message, failing JS errors and
// skipping (recording) any real command that slips in between.
func (t *selftest) nextMsg() (stMsg, bool) {
	for {
		c, ok := t.next()
		if !ok {
			return stMsg{}, false
		}
		switch c.Action {
		case "js_error":
			t.failf("erreur JS: %s", c.Key)
			return stMsg{}, false
		case "selftest":
			var m stMsg
			if err := json.Unmarshal([]byte(c.Key), &m); err != nil {
				t.failf("selftest: message illisible %q", c.Key)
				return stMsg{}, false
			}
			return m, true
		default:
			continue // a stray real command; the walker handles pairing
		}
	}
}

// interact collects the outcome of one triggered interaction: the helper's
// click mark plus, when wantCmd, one real command (wantAction constrains the
// action when non-empty). Mark and command arrive on independent goroutines
// (drawer dispatch), so both orders are accepted. Returns true when the
// interaction fully verified; failures are recorded.
func (t *selftest) interact(scenario, view, desc string, wantCmd bool, wantAction string) bool {
	deadline := time.After(stTimeout)
	gotMark := false
	gotCmd := false
	for !(gotMark && (gotCmd || !wantCmd)) {
		select {
		case c := <-t.cmds:
			switch c.Action {
			case "js_error":
				t.failf("%s/%s: %s: erreur JS: %s", scenario, view, desc, c.Key)
				return false
			case "selftest":
				var m stMsg
				if json.Unmarshal([]byte(c.Key), &m) == nil && m.Ev == "mark" {
					gotMark = true
				} else if m.Ev == "miss" {
					t.failf("%s/%s: %s: élément introuvable", scenario, view, desc)
					return false
				}
			default:
				if wantAction != "" && c.Action != wantAction {
					t.failf("%s/%s: %s: commande %q au lieu de %q", scenario, view, desc, c.Action, wantAction)
					return false
				}
				gotCmd = true
			}
		case <-deadline:
			switch {
			case !gotMark:
				t.failf("%s/%s: %s: pas de marque de clic", scenario, view, desc)
			default:
				t.failf("%s/%s: bouton muet: %s", scenario, view, desc)
			}
			return false
		}
	}
	return true
}

// drain empties pending commands (between steps), flagging JS errors.
func (t *selftest) drain() {
	for {
		select {
		case c := <-t.cmds:
			if c.Action == "js_error" {
				t.failf("erreur JS: %s", c.Key)
			}
		default:
			return
		}
	}
}

func runSelftest() int {
	if !drawer.Available {
		fmt.Fprintln(os.Stderr, "le panneau n'est disponible que sous Linux")
		return 1
	}

	t := &selftest{cmds: make(chan drawer.Command, 64)}
	t.d = drawer.New(func(c drawer.Command) { t.cmds <- c }, nil)
	if err := t.d.Show(mockState()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	t.d.SetWindowTitle("le panneau · FIP (autotest)")

	// Wait for the page, then inject the helpers (Eval drops scripts until
	// the page is ready, so ping until the pong comes back).
	ready := false
	for i := 0; i < 20 && !ready; i++ {
		t.d.Eval(stHelpers)
		select {
		case c := <-t.cmds:
			if c.Action == "selftest" && strings.Contains(c.Key, "pong") {
				ready = true
			}
		case <-time.After(500 * time.Millisecond):
		}
	}
	if !ready {
		fmt.Println("ÉCHEC: la page n'a jamais répondu (webkit indisponible ?)")
		return 1
	}

	views := []string{"main", "history", "settings"}
	clicks := 0
	for _, sc := range stScenarios() {
		scStart := time.Now()
		fmt.Printf("scénario « %s »\n", sc.name)
		for _, view := range views {
			t.d.Push(sc.state)
			t.d.SetView(view)
			time.Sleep(150 * time.Millisecond) // let the render settle
			t.drain()

			// The view must exist and carry rendered content.
			t.d.Eval(fmt.Sprintf("window.__stInfo(%q);", view))
			info, ok := t.nextMsg()
			if !ok || info.Ev != "info" {
				t.failf("%s/%s: pas de réponse __stInfo", sc.name, view)
				continue
			}
			if info.Sec != "view-"+view {
				t.failf("%s/%s: vue active %q", sc.name, view, info.Sec)
				continue
			}
			if info.Text <= 0 {
				t.failf("%s/%s: vue rendue vide", sc.name, view)
				continue
			}

			// Enumerate and click every enabled control.
			t.d.Eval("window.__stList();")
			list, ok := t.nextMsg()
			if !ok || list.Ev != "list" {
				t.failf("%s/%s: pas de liste de contrôles", sc.name, view)
				continue
			}
			for i, desc := range list.Items {
				t.d.Push(sc.state) // reset state so the walk is deterministic
				time.Sleep(30 * time.Millisecond)
				t.drain()
				t.d.Eval(fmt.Sprintf("window.__stClick(%d);", i))
				// The drawer hands each page message to a fresh goroutine, so
				// the click mark and the resulting command can arrive in
				// either order: collect both without assuming a sequence.
				if t.interact(sc.name, view, desc, !localOnly[desc], "") {
					clicks++
				}
				if localOnly[desc] {
					// Navigation flips the view: come back before the next index.
					t.d.SetView(view)
					time.Sleep(80 * time.Millisecond)
					t.drain()
				}
			}

			// The two sliders post through events, not clicks.
			if view == "main" && !(sc.state.Cast.Active && !sc.state.Cast.VolumeKnown) {
				t.drain()
				t.d.Eval("window.__stRange('vol', 33);")
				if t.interact(sc.name, view, "vol", true, "volume") {
					clicks++
				}
			}
			if view == "settings" {
				t.drain()
				t.d.Eval("window.__stRange('fade', 7);")
				if t.interact(sc.name, view, "fade", true, "crossfade") {
					clicks++
				}
			}
		}
		fmt.Printf("  (%.1fs)\n", time.Since(scStart).Seconds())
	}

	fmt.Printf("\nautotest du panneau: %d interactions vérifiées, %d échec(s)\n", clicks, len(t.failures))
	if len(t.failures) > 0 {
		return 1
	}
	fmt.Println("OK")
	return 0
}
