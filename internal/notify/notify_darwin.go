//go:build darwin

// Package notify shows desktop notifications on track changes. On macOS that
// is a banner through the UserNotifications framework. Best-effort: any
// failure is logged and ignored, never fatal.
package notify

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework UserNotifications

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#import <dispatch/dispatch.h>

// Go callbacks. The //export functions live in this package; the header is
// emitted by cgo next to the generated _cgo_export.h. The delegate class that
// calls back into them lives in delegate_darwin.m: a file using //export has
// its preamble compiled twice, which would duplicate an @implementation.
extern void notifyOnLike(void);
extern void notifyOnDislike(void);
extern void notifyOnOpen(void);
extern void notifyOnDenied(void);
extern void notifyInstallDelegate(UNUserNotificationCenter *center);

static NSString * const TrackCategory = @"track";

// notifyOnMain runs one unit of UserNotifications work on the main thread.
// Notify is called from tray and metadata goroutines; the center asserts on
// main-thread-only access.
static void notifyOnMain(void (^work)(void)) {
	if ([NSThread isMainThread]) {
		work();
	} else {
		dispatch_async(dispatch_get_main_queue(), work);
	}
}

// notifyDeliver posts one banner. A fixed identifier replaces the previous
// one, the same way the Linux path passes replaces_id.
static void notifyDeliver(UNUserNotificationCenter *center, UNNotificationContent *content) {
	UNNotificationRequest *req = [UNNotificationRequest
		requestWithIdentifier:@"pl.nech.fipindicateur.now" content:content trigger:nil];
	[center addNotificationRequest:req withCompletionHandler:^(NSError *error) {
		if (error != nil) {
			NSLog(@"fipindicateur notify: addNotificationRequest failed: %@", error);
		}
	}];
}

// notifyPost copies the strings, then posts once permission is known.
// Requesting permission is async; posting before it returns drops the
// banner. macOS decides how long a banner stays.
static void notifyPost(const char *title, const char *subtitle, const char *body,
                       const char *iconPath, BOOL track) {
	@autoreleasepool {
		// An unbundled process (make run, the bare fip binary) has no
		// notification center: asking for it raises an Objective-C exception
		// the Go side cannot catch, and the whole app dies on the first
		// track change. Stay quiet instead; playback is unaffected.
		if ([NSBundle mainBundle].bundleIdentifier == nil) {
			return;
		}
		UNMutableNotificationContent *content = [UNMutableNotificationContent new];
		if (title != NULL && title[0] != '\0') {
			content.title = [NSString stringWithUTF8String:title];
		}
		if (subtitle != NULL && subtitle[0] != '\0') {
			content.subtitle = [NSString stringWithUTF8String:subtitle];
		}
		if (body != NULL && body[0] != '\0') {
			content.body = [NSString stringWithUTF8String:body];
		}
		if (iconPath != NULL && iconPath[0] != '\0') {
			NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:iconPath]];
			NSError *err = nil;
			UNNotificationAttachment *att = [UNNotificationAttachment
				attachmentWithIdentifier:@"cover" URL:url options:nil error:&err];
			if (att != nil) {
				content.attachments = @[att];
			}
		}
		if (track) {
			content.categoryIdentifier = TrackCategory;
		}
		notifyOnMain(^{
			UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
			notifyInstallDelegate(center);
			[center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
				NSLog(@"fipindicateur notify: authorization status %ld", (long)settings.authorizationStatus);
				if (settings.authorizationStatus == UNAuthorizationStatusAuthorized) {
					notifyOnMain(^{ notifyDeliver(center, content); });
					return;
				}
				if (settings.authorizationStatus != UNAuthorizationStatusNotDetermined) {
					// denied: stay quiet, playback is unaffected. Say it once
					// to the app, which can point the user at System Settings;
					// the log line above names the status for the rest.
					if (settings.authorizationStatus == UNAuthorizationStatusDenied) {
						notifyOnDenied();
					}
					return;
				}
				[center requestAuthorizationWithOptions:UNAuthorizationOptionAlert
					completionHandler:^(BOOL granted, NSError *error) {
						NSLog(@"fipindicateur notify: requestAuthorization granted %d error %@", granted, error);
						if (granted) {
							notifyOnMain(^{ notifyDeliver(center, content); });
						}
					}];
			}];
		});
	}
}
*/
import "C"
import (
	"context"
	"sync"
	"unsafe"
)

// Actions wires banner interactions back into the app. Like and Dislike are
// the banner buttons on the track banner: pressing one is a click like the
// menu item, so the app records it the same way (the click IS the consent).
// Open receives the track link when the user clicks the banner body. Default
// fires when the user clicks the body of a plain (non-track) banner posted
// with NotifyAction, so a notice can open the thing it talks about. Denied
// fires when the system reports notification permission denied and a banner
// was dropped: the app can then point the user at System Settings, the only
// place the switch lives.
type Actions struct {
	Like    func()
	Dislike func()
	Open    func(link string)
	Denied  func()
	Default func()
}

// Notifier sends replace-in-place desktop notifications.
type Notifier struct{}

// The banner interactions come back through the //export callbacks, which are
// package-level (cgo exports have no receiver), so the actions live here,
// installed by New and guarded by mu together with lastLink.
var (
	mu       sync.Mutex
	actions  Actions
	lastLink string
)

// New returns a notifier. Authorization is requested on the first post, so
// construction never fails and callers need not nil-check.
func New(a Actions) *Notifier {
	mu.Lock()
	actions = a
	mu.Unlock()
	return &Notifier{}
}

// Notify posts a plain banner (system notices: cast, statistics, updates),
// replacing the previous one. timeoutMs is ignored: macOS decides how long a
// banner stays. No action buttons: a notice is not a verdict to make. It also
// retires any NotifyAction click: only the newest actionable banner acts.
func (n *Notifier) Notify(summary, body, iconPath string, timeoutMs int) {
	mu.Lock()
	actions.Default = nil
	mu.Unlock()
	n.push(summary, "", body, iconPath, "", timeoutMs)
}

// NotifyAction posts a plain banner whose body click routes to fn (the
// delegate's default action for non-track banners). timeoutMs is ignored,
// like Notify.
func (n *Notifier) NotifyAction(summary, body, iconPath string, timeoutMs int, actionLabel string, fn func()) {
	_ = actionLabel // macOS banners are click-to-act on the body; no inline button needed
	mu.Lock()
	actions.Default = fn
	mu.Unlock()
	n.push(summary, "", body, iconPath, "", timeoutMs)
}

// NotifyTrack posts the track banner: structured title/subtitle/body, the
// cover art, the like/dislike buttons and click-to-open on the listen link.
func (n *Notifier) NotifyTrack(title, subtitle, body, iconPath, link string, timeoutMs int) {
	n.push(title, subtitle, body, iconPath, link, timeoutMs)
}

func (n *Notifier) push(title, subtitle, body, iconPath, link string, timeoutMs int) {
	if n == nil {
		return
	}
	_ = timeoutMs
	mu.Lock()
	lastLink = link
	mu.Unlock()
	ct, cs, cb, ci := C.CString(title), C.CString(subtitle), C.CString(body), C.CString(iconPath)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(cb))
	defer C.free(unsafe.Pointer(ci))
	C.notifyPost(ct, cs, cb, ci, link != "")
}

// FetchCover downloads the cover and returns its path, or "" on any error.
func (n *Notifier) FetchCover(ctx context.Context, url string) string {
	return fetchCover(ctx, url)
}

// Close releases nothing. The center is process-wide.
func (*Notifier) Close() {}

func withActions(f func(a Actions)) {
	mu.Lock()
	a := actions
	mu.Unlock()
	if f != nil {
		f(a)
	}
}

//export notifyOnLike
func notifyOnLike() {
	withActions(func(a Actions) {
		if a.Like != nil {
			a.Like()
		}
	})
}

//export notifyOnDislike
func notifyOnDislike() {
	withActions(func(a Actions) {
		if a.Dislike != nil {
			a.Dislike()
		}
	})
}

//export notifyOnOpen
func notifyOnOpen() {
	mu.Lock()
	link := lastLink
	a := actions
	mu.Unlock()
	if link != "" && a.Open != nil {
		a.Open(link)
	}
}

//export notifyOnDefault
func notifyOnDefault() {
	withActions(func(a Actions) {
		if a.Default != nil {
			a.Default()
		}
	})
}

//export notifyOnDenied
func notifyOnDenied() {
	withActions(func(a Actions) {
		if a.Denied != nil {
			a.Denied()
		}
	})
}
