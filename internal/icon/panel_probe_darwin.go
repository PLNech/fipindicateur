//go:build darwin

package icon

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

int iconPanelDark(void);

// iconProbeWithin runs the panel probe inside the drawing appearance chosen
// by darkFlag (1 = DarkAqua, 0 = Aqua), whatever the host's system appearance
// is: the same thing the system does to the menu bar when the Auto
// appearance resolves dark in the evening.
static int iconProbeWithin(int darkFlag) {
    NSAppearance *a = [NSAppearance appearanceNamed:
        darkFlag ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua];
    if (a == nil) {
        return -1;
    }
    __block int r = -1;
    [a performAsCurrentDrawingAppearance:^{
        r = iconPanelDark();
    }];
    return r;
}
*/
import "C"

// probeWithinAppearance runs the panel probe inside a forced drawing
// appearance (dark=true resolves DarkAqua, false Aqua), so the dark half of
// the decision boundary is provable on a host that never leaves light mode.
// Test-only in intent (panel_darwin_test.go); the block captures no Go
// pointers, only the C result.
func probeWithinAppearance(dark bool) int {
	flag := 0
	if dark {
		flag = 1
	}
	return int(C.iconProbeWithin(C.int(flag)))
}
