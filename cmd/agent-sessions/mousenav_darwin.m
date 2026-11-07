//go:build darwin && desktop

#import <Cocoa/Cocoa.h>
#include <math.h>

#include "_cgo_export.h"

// NSEvent numbers the mouse's side buttons 3 (back) and 4 (forward) when
// no mouse utility remaps them.
#define BUTTON_BACK 3
#define BUTTON_FORWARD 4

// A two-finger swipe goes back or forward once it has gone this far
// sideways (in points), and more than twice as far as up or down.
#define SWIPE_DISTANCE 80.0

static id monitor;

// The scroll gesture in progress: how far the fingers went, and whether it
// already navigated.
static CGFloat swipeX, swipeY;
static BOOL swipeSent;

// With "Swipe between pages" set to two fingers, macOS sends no swipe
// event: the page swipe is a scroll that apps recognise themselves, as
// Safari does. Only gestures count (trackpad, Magic Mouse), not a wheel or
// the momentum after the fingers lift. The scroll still reaches the page;
// the page drops the swipe if it scrolled something (swipeNavigation.ts).
static void track_swipe(NSEvent *event) {
	NSEventPhase phase = [event phase];
	if (phase == NSEventPhaseNone) {
		return;
	}
	if (phase & (NSEventPhaseMayBegin | NSEventPhaseBegan)) {
		swipeX = 0;
		swipeY = 0;
		swipeSent = NO;
	}
	if (swipeSent || ![NSEvent isSwipeTrackingFromScrollEventsEnabled]) {
		return;
	}
	// Follow the fingers whatever the scroll direction setting: right is back.
	CGFloat dx = [event scrollingDeltaX];
	swipeX += [event isDirectionInvertedFromDevice] ? dx : -dx;
	swipeY += [event scrollingDeltaY];
	if (fabs(swipeX) >= SWIPE_DISTANCE && fabs(swipeX) > 2 * fabs(swipeY)) {
		swipeSent = YES;
		goSwipeNavigate((void *)[event window], swipeX < 0);
	}
}

// WKWebView does not report the side buttons to the page as buttons 3 and
// 4. A local monitor sees the app's events, in every window, before any view
// does: it takes the side buttons, moves the event's window back or forward
// on the release, and passes every other event on.
//
// Logi Options+ delivers its side buttons as a horizontal swipe instead of
// a button, as does a three-finger swipe when "Swipe between pages" is set
// to three fingers, so those are taken too. As in Safari, a positive deltaX
// means back. Scroll events are watched for two-finger swipes and always
// passed on.
void hook_side_buttons(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (monitor != nil) {
			return;
		}
		NSEventMask mask = NSEventMaskOtherMouseDown | NSEventMaskOtherMouseUp | NSEventMaskSwipe | NSEventMaskScrollWheel;
		monitor = [[NSEvent addLocalMonitorForEventsMatchingMask:mask handler:^NSEvent *(NSEvent *event) {
			if ([event type] == NSEventTypeScrollWheel) {
				track_swipe(event);
				return event;
			}
			if ([event type] == NSEventTypeSwipe) {
				CGFloat dx = [event deltaX];
				if (dx == 0) {
					return event;
				}
				goMouseNavigate((void *)[event window], dx < 0);
				return nil;
			}
			NSInteger button = [event buttonNumber];
			if (button != BUTTON_BACK && button != BUTTON_FORWARD) {
				return event;
			}
			if ([event type] == NSEventTypeOtherMouseUp) {
				goMouseNavigate((void *)[event window], button == BUTTON_FORWARD);
			}
			return nil;
		}] retain];
	});
}

// Windows need nothing of their own: the monitor sees every window's events.
void hook_window_side_buttons(void *window) {}
