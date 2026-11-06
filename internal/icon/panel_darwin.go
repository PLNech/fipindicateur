//go:build darwin

package icon

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// iconPanelDark resolves the panel darkness from the current drawing
// appearance: bestMatch against the two base appearances answers what the
// interface (the menu bar included) is showing right now, Auto resolved.
// Returns 1 (dark), 0 (light) or -1 (no appearance resolvable, e.g. a bare
// test host), letting the Go side fall back to the legacy defaults probe. No
// NSApp involvement: reading the appearance must never spin up an
// NSApplication behind a test. currentDrawingAppearance is the macOS 12+
// accessor (the plain currentAppearance class property is deprecated there).
// Not static: the appearance-forcing test (panel_probe_darwin_test.go) links
// against it to prove the dark path triggers; the preamble here compiles
// once (no //export in this file), so the symbol is safe to share.
int iconPanelDark(void) {
    NSAppearance *a = [NSAppearance currentDrawingAppearance];
    if (a == nil) {
        return -1;
    }
    NSString *best = [a bestMatchFromAppearancesWithNames:
        @[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]];
    if (best == nil) {
        return -1;
    }
    return [best isEqualToString:NSAppearanceNameDarkAqua] ? 1 : 0;
}
*/
import "C"

import (
	"os/exec"
	"strings"
)

// darkPanel reports whether the macOS menu bar is dark. The answer comes from
// the appearance system, not from `defaults read -g AppleInterfaceStyle` any
// more: under the Auto (wallpaper-driven) appearance that key is absent, the
// old probe answered "light" forever, and the bars then vanished every
// evening on the bar that goes dark. Re-probed on every appearance flip (see
// RefreshPanelDark); the call itself is an in-process read, no process spawn.
func darkPanel() bool {
	switch C.iconPanelDark() {
	case 1:
		return true
	case 0:
		return false
	}
	// No appearance resolvable: fall back to the legacy defaults probe. It is
	// wrong under Auto (the key is absent there, read as light), but it is a
	// better last guess than a constant: the fixed Dark/Light settings, where
	// it is right, are the only case a bare host without an appearance can
	// still say anything about.
	out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Dark")
}
