//go:build darwin

// The native half of the tray: one delegate owning the NSStatusItem and its
// NSMenu, modeled on the backend this replaces (fyne/systray's darwin .m)
// with the pieces that existed only to work around not owning the button
// removed. It lives in a real .m file, not in the cgo preamble of tray.go: a
// Go file using //export has its preamble compiled twice, which would
// duplicate the class symbols at link time (see
// internal/notify/delegate_darwin.m for the established pattern). ARC
// everywhere; the em-dash grep reaches this file too, so no U+2014 below.

#import <Cocoa/Cocoa.h>
#import <math.h>
#import <stdlib.h>

// Go callbacks, exported by tray.go.
extern void trayReady(void);
extern void trayExit(void);
extern void trayItemSelected(int id);
extern void trayScrollWheel(double deltaY, int precise);
extern void traySliderChanged(int sliderID, int value);
extern void trayMenuOpen(void);
extern void trayPrimaryClick(void *anchor);
extern void trayAppearanceChanged(void);

// The autosave name hands position persistence to AppKit: it stores the
// status item's spot under "NSStatusItem Preferred Position fipindicateur"
// and restores it on the next launch, so a place chosen by Command-drag
// survives restarts without our own bookkeeping (and without being clobbered
// by the next startup, which the old fixed-key approach could not promise).
static NSString * const StatusItemAutosave = @"fipindicateur";

// placeStatusItemDefault seeds the stored position before the item exists:
// the right-hand end of the extras area (0). On a notched MacBook a crowded
// bar overflows leftward and the leftmost item vanishes under the camera
// housing, present to Accessibility but invisible on screen. A stored value
// wins: either a user's Command-drag, or the value left by the replaced
// backend under its automatic "Item-0" name, migrated once here.
static void placeStatusItemDefault(void) {
    NSUserDefaults *d = [NSUserDefaults standardUserDefaults];
    NSString *key = [@"NSStatusItem Preferred Position "
        stringByAppendingString:StatusItemAutosave];
    if ([d objectForKey:key] != nil) {
        return;
    }
    NSNumber *old = [d objectForKey:@"NSStatusItem Preferred Position Item-0"];
    [d setDouble:old != nil ? [old doubleValue] : 0 forKey:key];
}

// TrayScrollView sits on top of the status item's button and catches scroll
// gestures (the volume wheel) while forwarding every click it does not
// consume back to the button, so the menu opens exactly as before the overlay
// existed. It is the direct heir of the replaced backend's RightClickDetector,
// minus the process-global event monitor that not owning the button required.
@interface TrayScrollView : NSView
@end

// primaryClick is the primary-click mode switch (main thread only).
// presentTrayMenu asks the delegate, defined further down, for the menu.
static BOOL primaryClick = NO;
static void presentTrayMenu(void);

@implementation TrayScrollView

// scrollWheel hands the raw gesture to Go (trayScrollWheel): precise trackpad
// deltas accumulate there until a whole step is complete, discrete wheel
// notches pass through one by one. Momentum is dropped right here: a flick's
// inertia must not keep turning the volume after the fingers leave the glass.
- (void)scrollWheel:(NSEvent *)event {
    if (event.momentumPhase != NSEventPhaseNone) {
        return;
    }
    trayScrollWheel(event.scrollingDeltaY, event.hasPreciseScrollingDeltas ? 1 : 0);
}

// Clicks are not consumed: the status item presents its menu on the button's
// mouse down, so the gesture is handed straight down the responder chain (the
// overlay's next responder is the button itself).
//
// Primary-click mode (trayEnablePrimaryClick, off by default) splits the
// gesture here instead, where every click on the item already lands: a plain
// left click goes to Go with the button as anchor (a popover opens under it),
// a right or control click presents the menu.
- (void)mouseDown:(NSEvent *)event {
    if (!primaryClick) {
        [[self nextResponder] mouseDown:event];
    } else if (event.modifierFlags & NSEventModifierFlagControl) {
        presentTrayMenu();
    } else {
        trayPrimaryClick((__bridge void *)[self superview]);
    }
}

- (void)rightMouseDown:(NSEvent *)event {
    if (primaryClick) {
        presentTrayMenu();
        return;
    }
    [[self nextResponder] rightMouseDown:event];
}

- (void)otherMouseDown:(NSEvent *)event {
    [[self nextResponder] otherMouseDown:event];
}

@end

// TraySliderView is the custom view behind the top-level slider item: an
// NSSlider with a little padding. A menu item's custom view receives the
// mouse events during tracking (that is what NSMenuItem.view exists for),
// which is how menu-bar audio apps host a live slider. Continuous: every drag
// tick reports to Go, which applies the level and lets the events coalescer
// decide what reaches the log.
@interface TraySliderView : NSView
- (instancetype)initWithID:(int)sliderID value:(int)value;
- (void)setLevel:(int)value enabled:(BOOL)enabled;
@end

@implementation TraySliderView
{
    NSSlider *slider;
    int sliderID;
}

- (instancetype)initWithID:(int)anID value:(int)value {
    self = [super initWithFrame:NSMakeRect(0, 0, 168, 26)];
    if (self != nil) {
        sliderID = anID;
        slider = [[NSSlider alloc] initWithFrame:NSMakeRect(8, 3, 152, 20)];
        slider.minValue = 0;
        slider.maxValue = 100;
        slider.intValue = value;
        slider.continuous = YES;
        slider.target = self;
        slider.action = @selector(sliderChanged:);
        slider.autoresizingMask = NSViewWidthSizable;
        [self addSubview:slider];
    }
    return self;
}

- (void)sliderChanged:(id)sender {
    (void)sender;
    traySliderChanged(sliderID, (int)round(slider.doubleValue));
}

- (void)setLevel:(int)value enabled:(BOOL)on {
    // A programmatic set does not fire the action, so this refresh never
    // comes back into Go as if the user had moved the handle.
    slider.intValue = value;
    slider.enabled = on;
}

@end

@interface TrayAppDelegate : NSObject <NSApplicationDelegate, NSMenuDelegate>
@end

@implementation TrayAppDelegate
{
    NSStatusItem *statusItem;
    NSMenu *menu;
}

- (void)applicationDidFinishLaunching:(NSNotification *)note {
    (void)note;
    statusItem = [[NSStatusBar systemStatusBar]
        statusItemWithLength:NSVariableStatusItemLength];
    statusItem.autosaveName = StatusItemAutosave;
    statusItem.visible = YES;

    menu = [[NSMenu alloc] init];
    menu.autoenablesItems = FALSE;
    menu.delegate = self;
    // The menu lives on the item for good: any click on the button opens it,
    // which is the whole click behavior this app has. The replaced backend
    // attached and detached the menu around a synthesized click only because
    // it did not own the button; owning it makes the dance unnecessary.
    statusItem.menu = menu;

    // The scroll-catching overlay (volume on wheel and trackpad): a subview
    // of the button, sized to it and keeping pace with resizes, that forwards
    // the clicks it does not consume (see TrayScrollView).
    TrayScrollView *scroll = [[TrayScrollView alloc]
        initWithFrame:statusItem.button.bounds];
    scroll.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    [statusItem.button addSubview:scroll];

    // Live appearance tracking: under the Auto (wallpaper-driven) appearance
    // the menu bar flips dark every evening and back in the morning. The
    // notification names the flip so Go re-probes the panel and re-inks the
    // glyph, instead of keeping the boot-time answer forever (the old
    // defaults-key probe froze the launch-time state by construction).
    [[NSNotificationCenter defaultCenter] addObserver:self
        selector:@selector(effectiveAppearanceChanged:)
        name:@"NSApplicationDidChangeEffectiveAppearanceNotification"
        object:nil];

    trayReady();
}

- (void)applicationWillTerminate:(NSNotification *)note {
    (void)note;
    trayExit();
}

// applicationSupportsSecureRestorableState opts the app into the modern
// state-restoration contract macOS 14+ asks every NSApplication delegate to
// declare; without it [NSApp run] logs a warning on every launch.
- (BOOL)applicationSupportsSecureRestorableState:(NSApplication *)app {
    (void)app;
    return YES;
}

// setPrimaryClick switches the item between its two click behaviors. Off
// (the default): the menu lives on the item and any click opens it. On: the
// item carries no menu, so AppKit stops presenting one by itself and the
// scroll overlay routes the clicks (see TrayScrollView).
- (void)setPrimaryClick:(NSNumber *)on {
    primaryClick = on.boolValue;
    statusItem.menu = primaryClick ? nil : menu;
}

// presentMenu shows the menu in primary-click mode: attached for the length
// of one synthesized click (performClick tracks the menu and returns when it
// closes), then detached again so the next left click stays ours.
- (void)presentMenu {
    statusItem.menu = menu;
    [statusItem.button performClick:nil];
    statusItem.menu = primaryClick ? nil : menu;
}

- (void)menuHandler:(id)sender {
    trayItemSelected((int)[sender tag]);
}

// effectiveAppearanceChanged forwards the appearance flip to Go: the glyph
// must re-ink (light ink on a dark bar) the moment the bar changes under it.
- (void)effectiveAppearanceChanged:(NSNotification *)note {
    (void)note;
    trayAppearanceChanged();
}

// menuWillOpen re-syncs the live state the menu displays but does not own:
// the volume slider must show the active sink's level NOW, not the level of
// build time. trayMenuOpen lands in Go and comes straight back through the
// volume UI sync; we are on the main thread, so the marshaled setters run
// inline (onMain's fast path).
- (void)menuWillOpen:(NSMenu *)m {
    if (m == menu) {
        trayMenuOpen();
    }
}

// updateButtonStyle keeps the icon/title layout coherent: icon only, icon
// with text to the left, or text only. The title stays empty in this app,
// but the setter exists and must not leave a stale imagePosition behind.
- (void)updateButtonStyle {
    NSStatusBarButton *b = statusItem.button;
    if (b.image == nil) {
        b.imagePosition = NSNoImage;
    } else if (b.title.length == 0) {
        b.imagePosition = NSImageOnly;
    } else {
        b.imagePosition = NSImageLeft;
    }
}

- (void)setStatusIcon:(NSImage *)image {
    statusItem.button.image = image;
    [self updateButtonStyle];
}

- (void)setStatusTitle:(NSString *)title {
    statusItem.button.title = title;
    [self updateButtonStyle];
}

- (void)setStatusTooltip:(NSString *)tip {
    statusItem.button.toolTip = tip;
}

// findItem walks the tree by tag, submenus included (separators carry the
// default tag 0, which no real item uses: ids start at 1).
- (NSMenuItem *)findItem:(int)tag inMenu:(NSMenu *)m {
    NSMenuItem *found = [m itemWithTag:tag];
    if (found != nil) {
        return found;
    }
    for (NSMenuItem *it in m.itemArray) {
        if (it.hasSubmenu) {
            found = [self findItem:tag inMenu:it.submenu];
            if (found != nil) {
                return found;
            }
        }
    }
    return nil;
}

// addItem installs one entry, creating its parent submenu on the fly, or
// updates it when the tag is already there (state re-syncs land as updates;
// the menu never removes items, so slots are stable).
- (void)addItem:(NSArray *)spec {
    int tag = [spec[0] intValue];
    int parent = [spec[1] intValue];
    NSString *title = spec[2];
    NSString *tooltip = spec[3];
    BOOL disabled = [spec[4] boolValue];
    BOOL checked = [spec[5] boolValue];

    NSMenu *target = menu;
    if (parent > 0) {
        NSMenuItem *parentItem = [self findItem:parent inMenu:menu];
        if (parentItem == nil) {
            return;
        }
        if (!parentItem.hasSubmenu) {
            NSMenu *sub = [[NSMenu alloc] init];
            sub.autoenablesItems = FALSE;
            parentItem.submenu = sub;
        }
        target = parentItem.submenu;
    }
    NSMenuItem *item = [self findItem:tag inMenu:target];
    if (item == nil) {
        item = [target addItemWithTitle:title
                                 action:@selector(menuHandler:)
                          keyEquivalent:@""];
        item.tag = tag;
        item.target = self;
    }
    item.title = title;
    item.toolTip = tooltip;
    item.enabled = !disabled;
    item.state = checked ? NSControlStateValueOn : NSControlStateValueOff;
}

- (void)addSeparator {
    [menu addItem:[NSMenuItem separatorItem]];
}

// The item mutators take @[ id, value ]: one shape, one find, one write.

- (void)setItemTitle:(NSArray *)args {
    NSMenuItem *item = [self findItem:[args[0] intValue] inMenu:menu];
    if (item != nil) {
        item.title = args[1];
    }
}

- (void)setItemTooltip:(NSArray *)args {
    NSMenuItem *item = [self findItem:[args[0] intValue] inMenu:menu];
    if (item != nil) {
        item.toolTip = args[1];
    }
}

- (void)setItemEnabled:(NSArray *)args {
    NSMenuItem *item = [self findItem:[args[0] intValue] inMenu:menu];
    if (item != nil) {
        item.enabled = [args[1] boolValue];
    }
}

- (void)setItemChecked:(NSArray *)args {
    NSMenuItem *item = [self findItem:[args[0] intValue] inMenu:menu];
    if (item != nil) {
        item.state = [args[1] boolValue] ? NSControlStateValueOn
                                         : NSControlStateValueOff;
    }
}

- (void)setItemHidden:(NSArray *)args {
    NSMenuItem *item = [self findItem:[args[0] intValue] inMenu:menu];
    if (item != nil) {
        item.hidden = [args[1] boolValue];
    }
}

// insertSlider hosts a slider item in the ROOT menu, immediately before the
// anchor item (spec[0], its tag): the volume slider is a top-level entry, so
// adjusting it is one click to open the menu and one drag, with no submenu
// hop. The Volume submenu keeps the mute toggle and the preset checkboxes.
// The anchor is found by tag, never by index, so a cast-list refresh cannot
// shift the insertion point out from under us; a missing anchor appends at
// the end rather than dropping the slider.
- (void)insertSlider:(NSArray *)spec {
    int before = [spec[0] intValue];
    int sliderID = [spec[1] intValue];
    int value = [spec[2] intValue];
    NSMenuItem *item = [[NSMenuItem alloc] init];
    item.title = @"";
    item.view = [[TraySliderView alloc] initWithID:sliderID value:value];
    item.tag = sliderID;
    NSUInteger idx = [menu indexOfItemWithTag:before];
    if (idx == NSNotFound) {
        [menu addItem:item];
    } else {
        [menu insertItem:item atIndex:idx];
    }
}

- (void)setSliderState:(NSArray *)args {
    NSMenuItem *item = [self findItem:[args[0] intValue] inMenu:menu];
    if (item.view != nil && [item.view isKindOfClass:[TraySliderView class]]) {
        [(TraySliderView *)item.view setLevel:[args[1] intValue]
                                     enabled:[args[2] boolValue]];
    }
}

- (void)quit {
    [NSApp stop:self];
    // The run loop only exits after servicing one more event, so post one.
    NSEvent *ev = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                     location:NSMakePoint(0, 0)
                                modifierFlags:0
                                    timestamp:0
                                 windowNumber:0
                                      context:nil
                                      subtype:0
                                        data1:0
                                        data2:0];
    [NSApp postEvent:ev atStart:NO];
}

@end


static TrayAppDelegate *owner = nil;

// onMain marshals one unit of AppKit work to the main thread and waits: the
// Go callers run on other goroutines, and the status item and menu are
// main-thread objects. The wait keeps concurrent callers ordered (the frame
// pump relies on it), exactly like the backend this replaces.
// Already on the main thread (the menu-will-open hook re-entering the
// setters), the call runs inline: waiting on ourselves would stall menu
// tracking for nothing.
static void onMain(SEL sel, id obj) {
    if ([NSThread isMainThread]) {
        // The selector is one of this class's own marshaling methods, never
        // caller-supplied, so the leak heuristic does not apply.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Warc-performSelector-leaks"
        [owner performSelector:sel withObject:obj];
#pragma clang diagnostic pop
        return;
    }
    [owner performSelectorOnMainThread:sel withObject:obj waitUntilDone:YES];
}

// strAndFree converts a C string handed over by Go and releases it right
// away: the NSString is retained by ARC, so nothing needs the buffer after
// this returns.
static NSString *strAndFree(char *c) {
    NSString *s = [NSString stringWithUTF8String:c];
    free(c);
    return s == nil ? @"" : s;
}

void trayRegister(void) {
    [NSApplication sharedApplication];
    placeStatusItemDefault();
    owner = [TrayAppDelegate new];
    [NSApp setDelegate:owner];
}

void trayRunLoop(void) {
    [NSApp run];
}

static void presentTrayMenu(void) {
    [owner presentMenu];
}

void trayEnablePrimaryClick(short on) {
    onMain(@selector(setPrimaryClick:), @(on != 0));
}

void trayQuit(void) {
    onMain(@selector(quit), nil);
}

void traySetIconBytes(char *png, int len) {
    if (png == NULL || len <= 0) {
        return;
    }
    @autoreleasepool {
        NSImage *image = [[NSImage alloc]
            initWithData:[NSData dataWithBytes:png length:(NSUInteger)len]];
        if (image == nil) {
            return;
        }
        // Size from the PNG itself: the tray hands 44 px @2x bytes, drawn at
        // half the pixel size (22 pt), sharp on retina. The replaced backend
        // flattened every image to 16x16.
        NSSize px = image.size;
        image.size = NSMakeSize(px.width / 2, px.height / 2);
        onMain(@selector(setStatusIcon:), image);
    }
}

void traySetStatusTitle(char *title) {
    NSString *s = strAndFree(title);
    if (owner == nil) {
        return;
    }
    onMain(@selector(setStatusTitle:), s);
}

void traySetStatusTooltip(char *tip) {
    NSString *s = strAndFree(tip);
    if (owner == nil) {
        return;
    }
    onMain(@selector(setStatusTooltip:), s);
}

void trayAddItem(int itemID, int parentID, char *title, char *tooltip,
                 short disabled, short checked) {
    NSString *t = strAndFree(title);
    NSString *tt = strAndFree(tooltip);
    if (owner == nil) {
        return;
    }
    onMain(@selector(addItem:), @[@(itemID), @(parentID), t, tt,
                                  @(disabled != 0), @(checked != 0)]);
}

void trayAddSeparator(void) {
    if (owner == nil) {
        return;
    }
    onMain(@selector(addSeparator), nil);
}

void traySetItemTitle(int itemID, char *title) {
    NSString *s = strAndFree(title);
    if (owner == nil) {
        return;
    }
    onMain(@selector(setItemTitle:), @[@(itemID), s]);
}

void traySetItemTooltip(int itemID, char *tip) {
    NSString *s = strAndFree(tip);
    if (owner == nil) {
        return;
    }
    onMain(@selector(setItemTooltip:), @[@(itemID), s]);
}

void traySetItemEnabled(int itemID, short enabled) {
    if (owner == nil) {
        return;
    }
    onMain(@selector(setItemEnabled:), @[@(itemID), @(enabled != 0)]);
}

void traySetItemChecked(int itemID, short checked) {
    if (owner == nil) {
        return;
    }
    onMain(@selector(setItemChecked:), @[@(itemID), @(checked != 0)]);
}

void traySetItemHidden(int itemID, short hidden) {
    if (owner == nil) {
        return;
    }
    onMain(@selector(setItemHidden:), @[@(itemID), @(hidden != 0)]);
}

void trayInsertSlider(int parentID, int sliderID, int value) {
    if (owner == nil) {
        return;
    }
    onMain(@selector(insertSlider:), @[@(parentID), @(sliderID), @(value)]);
}

void traySetSliderState(int sliderID, int value, short enabled) {
    if (owner == nil) {
        return;
    }
    onMain(@selector(setSliderState:),
           @[@(sliderID), @(value), @(enabled != 0)]);
}
