// Opening Armarium again while it runs (Finder, Launchpad, Spotlight) sends
// the running app a reopen Apple Event instead of starting a second copy; it
// becomes "open Armarium in the browser".
#import <Cocoa/Cocoa.h>

extern void goReopen(void);

@interface ArmariumReopen : NSObject
- (void)reopen:(NSAppleEventDescriptor *)event withReply:(NSAppleEventDescriptor *)reply;
@end

@implementation ArmariumReopen
- (void)reopen:(NSAppleEventDescriptor *)event withReply:(NSAppleEventDescriptor *)reply {
	goReopen();
}
@end

static ArmariumReopen *handler;

void listenForReopen(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		handler = [ArmariumReopen new];
		[[NSAppleEventManager sharedAppleEventManager] setEventHandler:handler
		                                                   andSelector:@selector(reopen:withReply:)
		                                                 forEventClass:kCoreEventClass
		                                                    andEventID:kAEReopenApplication];
	});
}
