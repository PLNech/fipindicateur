//go:build darwin

// The notification delegate lives in a real .m file, not the cgo preamble:
// a Go file using //export has its preamble compiled twice, which would
// duplicate the class symbols at link time (static functions are immune,
// Objective-C classes are not).

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

// Go callbacks, exported by the package's cgo file.
extern void notifyOnLike(void);
extern void notifyOnDislike(void);
extern void notifyOnOpen(void);
extern void notifyOnDefault(void);

static NSString * const TrackCategory = @"track";

@interface NotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation NotifyDelegate
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
    didReceiveNotificationResponse:(UNNotificationResponse *)response
    completionHandler:(void (^)(void))completionHandler {
	NSString *aid = response.actionIdentifier;
	NSString *cat = response.notification.request.content.categoryIdentifier;
	if ([aid isEqualToString:@"like"]) {
		notifyOnLike();
	} else if ([aid isEqualToString:@"dislike"]) {
		notifyOnDislike();
	} else if ([aid isEqualToString:UNNotificationDefaultActionIdentifier]) {
		if ([cat isEqualToString:TrackCategory]) {
			notifyOnOpen();
		} else {
			// A plain notice posted with NotifyAction: the body click acts.
			// Notices posted without one leave the Go callback nil (no-op).
			notifyOnDefault();
		}
	}
	completionHandler();
}
@end

void notifyInstallDelegate(UNUserNotificationCenter *center) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		// The banner carries the like/dislike buttons only on the track
		// category: plain notices (cast, statistics) stay clickless. Clicking
		// the banner body itself opens the track link (the same one the
		// « Voir… » menu offers). The actions route back through the //export
		// functions into the app's taste path: pressing the button is a click
		// like the menu item, and that click is the consent.
		static NotifyDelegate *delegate = nil;
		delegate = [NotifyDelegate new];
		center.delegate = delegate;
		UNNotificationAction *like = [UNNotificationAction
			actionWithIdentifier:@"like" title:@"J'aime"
			options:UNNotificationActionOptionNone];
		UNNotificationAction *dislike = [UNNotificationAction
			actionWithIdentifier:@"dislike" title:@"Pas pour moi"
			options:UNNotificationActionOptionDestructive];
		UNNotificationCategory *cat = [UNNotificationCategory
			categoryWithIdentifier:TrackCategory
			actions:@[like, dislike]
			intentIdentifiers:@[]
			options:UNNotificationCategoryOptionNone];
		[center setNotificationCategories:[NSSet setWithObject:cat]];
	});
}
