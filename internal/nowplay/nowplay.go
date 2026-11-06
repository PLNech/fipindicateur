//go:build darwin

package nowplay

// Package nowplay (darwin) registers the app with MPNowPlayingInfoCenter and
// MPRemoteCommandCenter. macOS routes the keyboard media keys, the Control
// Center widget, the lock screen and headphone clicks to whichever process
// published itself there. MPRIS does not exist on this platform, so without
// this registration the keys go to whatever other player last claimed them.

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework MediaPlayer -framework AppKit

#import <AppKit/AppKit.h>
#import <MediaPlayer/MediaPlayer.h>
#import <dispatch/dispatch.h>

// Go callbacks. The //export functions live in this package; the header is
// emitted by cgo next to the generated _cgo_export.h.
extern void nowplayOnPlay(void);
extern void nowplayOnPause(void);
extern void nowplayOnToggle(void);
extern void nowplayOnNext(void);
extern void nowplayOnPrev(void);
extern void nowplayOnRegistered(void);

// nowplayOnMain runs one unit of MediaPlayer/AppKit work on the main thread.
// None of the Go callers is on it: fyne/systray runs onReady on its own
// goroutine (systray.go Register), and SetTrack/SetPlaying are called from
// metadata and playback-observer goroutines. MPRemoteCommandCenter and
// MPNowPlayingInfoCenter are main-thread API; registering off it does not
// fail loudly, macOS just never routes the media keys to us and they keep
// going to the previous claimant.
static void nowplayOnMain(void (^work)(void)) {
	if ([NSThread isMainThread]) {
		work();
	} else {
		dispatch_async(dispatch_get_main_queue(), work);
	}
}

static void nowplayStart(void) {
	nowplayOnMain(^{
	// MPRemoteCommandCenter delivers on the main thread and needs a running
	// NSApp. fyne/systray created one before starting its run loop; this is
	// a no-op if so.

	MPRemoteCommandCenter *cc = [MPRemoteCommandCenter sharedCommandCenter];

	[cc.playCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
		(void)event;
		nowplayOnPlay();
		return MPRemoteCommandHandlerStatusSuccess;
	}];
	[cc.pauseCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
		(void)event;
		nowplayOnPause();
		return MPRemoteCommandHandlerStatusSuccess;
	}];
	[cc.togglePlayPauseCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
		(void)event;
		nowplayOnToggle();
		return MPRemoteCommandHandlerStatusSuccess;
	}];
	// A live radio has no previous/next track, so next/previous zap the
	// webradio list instead (the Go handlers decide; see Handler.Next/Prev).
	// Enabling them puts the pair to work in Control Center and on the lock
	// screen instead of hiding two dead buttons.
	[cc.nextTrackCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
		(void)event;
		nowplayOnNext();
		return MPRemoteCommandHandlerStatusSuccess;
	}];
	[cc.previousTrackCommand addTargetWithHandler:^MPRemoteCommandHandlerStatus(MPRemoteCommandEvent *event) {
		(void)event;
		nowplayOnPrev();
		return MPRemoteCommandHandlerStatusSuccess;
	}];

	// Seek is meaningless on a live stream. Disabled, so the system does not
	// offer a scrub bar that would do nothing.
	cc.changePlaybackPositionCommand.enabled = NO;
	cc.skipForwardCommand.enabled = NO;
	cc.skipBackwardCommand.enabled = NO;
	nowplayOnRegistered();
	});
}

static void nowplayStop(void) {
	nowplayOnMain(^{
	MPRemoteCommandCenter *cc = [MPRemoteCommandCenter sharedCommandCenter];
	[cc.playCommand removeTarget:nil];
	[cc.pauseCommand removeTarget:nil];
	[cc.togglePlayPauseCommand removeTarget:nil];
	[cc.nextTrackCommand removeTarget:nil];
	[cc.previousTrackCommand removeTarget:nil];
	[MPNowPlayingInfoCenter defaultCenter].playbackState = MPNowPlayingPlaybackStateStopped;
	[MPNowPlayingInfoCenter defaultCenter].nowPlayingInfo = nil;
	});
}

// nowplaySet publishes the track and the playback state in one write. macOS
// does not infer playbackState from an audio session the way iOS does; both
// the dictionary (rate) and the center's playbackState must be set, or the
// keys keep going to the previous claimant.
static void nowplaySet(const char *title, const char *artist, const char *album, int playing) {
	@autoreleasepool {
		// The dictionary is built on the caller's thread: plain Foundation,
		// no main-thread requirement. It must be: the Go side frees the C
		// strings as soon as this returns, so they cannot cross the hop;
		// the block retains info instead.
		NSMutableDictionary *info = [NSMutableDictionary dictionary];
		if (title != NULL && title[0] != '\0') {
			info[MPMediaItemPropertyTitle] = [NSString stringWithUTF8String:title];
		}
		if (artist != NULL && artist[0] != '\0') {
			info[MPMediaItemPropertyArtist] = [NSString stringWithUTF8String:artist];
		}
		if (album != NULL && album[0] != '\0') {
			info[MPMediaItemPropertyAlbumTitle] = [NSString stringWithUTF8String:album];
		}
		info[MPNowPlayingInfoPropertyPlaybackRate] = playing ? @1.0 : @0.0;
		info[MPNowPlayingInfoPropertyDefaultPlaybackRate] = @1.0;
		info[MPNowPlayingInfoPropertyIsLiveStream] = @YES;

		nowplayOnMain(^{
			MPNowPlayingInfoCenter *center = [MPNowPlayingInfoCenter defaultCenter];
			center.nowPlayingInfo = info;
			center.playbackState = playing ? MPNowPlayingPlaybackStatePlaying : MPNowPlayingPlaybackStatePaused;
		});
	}
}
*/
import "C"
import (
	"log"
	"sync"
	"unsafe"
)

// Handler and Info live in types.go: they are part of the platform-neutral
// API consumed by internal/ui on every OS, so the no-op build must see them.

var (
	mu      sync.Mutex
	handler Handler
	track   Info
	playing bool
	started bool
)

// Start registers the command center. Safe to call once; a second call
// replaces the handler without re-adding targets.
func Start(h Handler) {
	mu.Lock()
	handler = h
	first := !started
	started = true
	mu.Unlock()
	if first {
		C.nowplayStart()
	}
	publishLocked()
}

// Stop drops the registration and clears the published track.
func Stop() {
	mu.Lock()
	if !started {
		mu.Unlock()
		return
	}
	started = false
	handler = Handler{}
	mu.Unlock()
	C.nowplayStop()
}

// SetTrack publishes a new track, keeping the current playback state.
func SetTrack(info Info) {
	mu.Lock()
	track = info
	on := started
	mu.Unlock()
	if on {
		publish()
	}
}

// SetPlaying publishes whether audio is coming out.
func SetPlaying(on bool) {
	mu.Lock()
	playing = on
	live := started
	mu.Unlock()
	if live {
		publish()
	}
}

func publish() {
	mu.Lock()
	title, artist, album := track.Title, track.Artist, track.Album
	on := playing
	mu.Unlock()
	ctitle := C.CString(title)
	cartist := C.CString(artist)
	calbum := C.CString(album)
	defer C.free(unsafe.Pointer(ctitle))
	defer C.free(unsafe.Pointer(cartist))
	defer C.free(unsafe.Pointer(calbum))
	C.nowplaySet(ctitle, cartist, calbum, boolToC(on))
}

// publishLocked is Start's first paint. It takes no lock of its own beyond
// the snapshot, same as publish.
func publishLocked() { publish() }

func boolToC(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// The command center invokes these on the AppKit main thread, which on macOS
// is also the systray loop. The handlers (App.Play/Pause/Toggle) are the same
// ones MPRIS calls on Linux and are safe to run there. They must live in this
// file: cgo only links //export functions from the file that imports "C".

//export nowplayOnPlay
func nowplayOnPlay() { call(func(h Handler) func() { return h.Play }) }

//export nowplayOnPause
func nowplayOnPause() { call(func(h Handler) func() { return h.Pause }) }

//export nowplayOnToggle
func nowplayOnToggle() { call(func(h Handler) func() { return h.Toggle }) }

//export nowplayOnNext
func nowplayOnNext() { call(func(h Handler) func() { return h.Next }) }

//export nowplayOnPrev
func nowplayOnPrev() { call(func(h Handler) func() { return h.Prev }) }

// nowplayOnRegistered marks that the command-center registration block really
// ran on the AppKit main thread. One log line at startup: a registration that
// silently never happens is exactly the failure mode that cost us a debugging
// session, so the happy path says so.
//
//export nowplayOnRegistered
func nowplayOnRegistered() {
	log.Printf("nowplay: media keys registered on the AppKit main thread")
}

func call(pick func(Handler) func()) {
	mu.Lock()
	fn := pick(handler)
	mu.Unlock()
	if fn != nil {
		fn()
	}
}
