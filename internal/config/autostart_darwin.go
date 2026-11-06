//go:build darwin

package config

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework ServiceManagement

#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>

// loginItemSet registers or unregisters this app as a login item. The item
// is the running app bundle, never a bare binary: a binary started at login
// is a background task, and macOS then parks the menu-bar icon off the bar.
// Returns 0 on success, 2 on failure.
static int loginItemSet(int enabled) {
	if (@available(macOS 13.0, *)) {
		SMAppService *svc = [SMAppService mainAppService];
		NSError *err = nil;
		BOOL ok = enabled ? [svc registerAndReturnError:&err]
		                  : [svc unregisterAndReturnError:&err];
		return ok ? 0 : 2;
	}
	return 2;
}
*/
import "C"
import "fmt"

// AutostartSupported reports whether "launch at login" is wired on this OS.
const AutostartSupported = true

// SetAutostart adds or removes the running app from the login items. It has
// to run from FipIndicateur.app: the service registers that bundle, so a
// later login opens the menu-bar agent and the icon stays on the bar.
func SetAutostart(enabled bool) error {
	if C.loginItemSet(boolToC(enabled)) == 0 {
		return nil
	}
	if enabled {
		return fmt.Errorf("login item: register failed (launch via FipIndicateur.app)")
	}
	return fmt.Errorf("login item: unregister failed")
}

func boolToC(v bool) C.int {
	if v {
		return 1
	}
	return 0
}
