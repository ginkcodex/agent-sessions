//go:build darwin && desktop

#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>

#include "_cgo_export.h"

// Keyboard diagnostics (docs/plan/keyboard-diagnostics.md): while the user
// records, a local monitor and a few observers describe every key event and
// text-input state change this process sees, for the log. They only read:
// the monitor hands every event back unchanged.

static atomic_int keylogOn;
static id keylogMonitor;

void keylog_set_enabled(int on) {
	atomic_store(&keylogOn, on);
}

// codePoints lists s as U+XXXX, so invisible characters (DEL, Backspace,
// U+202F) show in the log.
static NSArray *codePoints(NSString *s) {
	NSMutableArray *out = [NSMutableArray array];
	NSUInteger n = [s length];
	for (NSUInteger i = 0; i < n; i++) {
		UTF32Char c = [s characterAtIndex:i];
		if (CFStringIsSurrogateHighCharacter(c) && i + 1 < n) {
			UniChar low = [s characterAtIndex:i + 1];
			if (CFStringIsSurrogateLowCharacter(low)) {
				c = CFStringGetLongCharacterForSurrogatePair((UniChar)c, low);
				i++;
			}
		}
		[out addObject:[NSString stringWithFormat:@"U+%04X", (unsigned)c]];
	}
	return out;
}

static NSString *sourceID(TISInputSourceRef src) {
	if (src == NULL) {
		return @"";
	}
	NSString *sid = (NSString *)TISGetInputSourceProperty(src, kTISPropertyInputSourceID);
	NSString *out = sid != nil ? [[sid copy] autorelease] : @"";
	CFRelease(src);
	return out;
}

// textInputState is what decides where typed text goes: the key window, its
// first responder, marked text, and the input sources. Main thread only.
static NSMutableDictionary *textInputState(NSWindow *window) {
	NSMutableDictionary *d = [NSMutableDictionary dictionary];
	if (window == nil) {
		window = [NSApp keyWindow];
	}
	d[@"win"] = @([window windowNumber]);
	d[@"keyWin"] = @([[NSApp keyWindow] windowNumber]);
	d[@"active"] = @([NSApp isActive]);
	id responder = [window firstResponder];
	d[@"responder"] = responder != nil ? NSStringFromClass([responder class]) : @"";
	if ([responder conformsToProtocol:@protocol(NSTextInputClient)] && [responder respondsToSelector:@selector(hasMarkedText)]) {
		d[@"marked"] = @([(id<NSTextInputClient>)responder hasMarkedText]);
	}
	NSTextInputContext *ctx = [NSTextInputContext currentInputContext];
	if (ctx != nil) {
		d[@"ctxSource"] = [ctx selectedKeyboardInputSource] ?: @"";
		d[@"ctxClient"] = [ctx client] != nil ? NSStringFromClass([(id)[ctx client] class]) : @"";
	}
	d[@"tis"] = sourceID(TISCopyCurrentKeyboardInputSource());
	d[@"tisLayout"] = sourceID(TISCopyCurrentKeyboardLayoutInputSource());
	d[@"secure"] = @(IsSecureEventInputEnabled() ? YES : NO);
	return d;
}

static void sendRecord(int mark, NSDictionary *record) {
	NSData *json = [NSJSONSerialization dataWithJSONObject:record options:0 error:NULL];
	if (json != nil) {
		goKeylogEvent(mark, (char *)[json bytes], (int)[json length]);
	}
}

static NSString *eventType(NSEvent *event) {
	switch ([event type]) {
	case NSEventTypeKeyDown:
		return @"keyDown";
	case NSEventTypeKeyUp:
		return @"keyUp";
	case NSEventTypeFlagsChanged:
		return @"flagsChanged";
	default:
		return [NSString stringWithFormat:@"type%lu", (unsigned long)[event type]];
	}
}

static void recordEvent(NSEvent *event) {
	NSMutableDictionary *d = textInputState([event window]);
	BOOL secure = [d[@"secure"] boolValue];
	BOOL isKey = [event type] == NSEventTypeKeyDown || [event type] == NSEventTypeKeyUp;
	d[@"type"] = eventType(event);
	d[@"keyCode"] = @([event keyCode]);
	d[@"mods"] = [NSString stringWithFormat:@"0x%lx", (unsigned long)[event modifierFlags]];
	d[@"evTs"] = @([event timestamp]);
	if (isKey) {
		d[@"repeat"] = @([event isARepeat]);
		if (!secure) {
			// characters raises for flagsChanged events: key events only.
			NSString *chars = [event characters] ?: @"";
			NSString *bare = [event charactersIgnoringModifiers] ?: @"";
			d[@"chars"] = chars;
			d[@"cps"] = codePoints(chars);
			d[@"bare"] = codePoints(bare);
		}
	}
	CGEventRef cg = [event CGEvent];
	if (cg != NULL) {
		d[@"pid"] = @(CGEventGetIntegerValueField(cg, kCGEventSourceUnixProcessID));
		d[@"stateID"] = @(CGEventGetIntegerValueField(cg, kCGEventSourceStateID));
		d[@"userData"] = @(CGEventGetIntegerValueField(cg, kCGEventSourceUserData));
		d[@"cgKeyCode"] = @(CGEventGetIntegerValueField(cg, kCGKeyboardEventKeycode));
		d[@"cgFlags"] = [NSString stringWithFormat:@"0x%llx", (unsigned long long)CGEventGetFlags(cg)];
		d[@"cgTs"] = @(CGEventGetTimestamp(cg));
		if (isKey && !secure) {
			UniChar buf[16];
			UniCharCount len = 0;
			CGEventKeyboardGetUnicodeString(cg, 16, &len, buf);
			d[@"cgCps"] = codePoints([NSString stringWithCharacters:buf length:len]);
		}
	}
	sendRecord(0, d);
}

static void recordMark(NSString *name, NSNotification *note) {
	if (!atomic_load(&keylogOn)) {
		return;
	}
	NSWindow *window = [[note object] isKindOfClass:[NSWindow class]] ? [note object] : nil;
	NSMutableDictionary *d = textInputState(window);
	d[@"type"] = name;
	sendRecord(1, d);
}

static void observe(NSNotificationCenter *center, NSString *notification, NSString *name) {
	[[center addObserverForName:notification object:nil queue:[NSOperationQueue mainQueue] usingBlock:^(NSNotification *note) {
		recordMark(name, note);
	}] retain];
}

void hook_keylog(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (keylogMonitor != nil) {
			return;
		}
		NSEventMask mask = NSEventMaskKeyDown | NSEventMaskKeyUp | NSEventMaskFlagsChanged;
		keylogMonitor = [[NSEvent addLocalMonitorForEventsMatchingMask:mask handler:^NSEvent *(NSEvent *event) {
			if (atomic_load(&keylogOn)) {
				@try {
					recordEvent(event);
				} @catch (NSException *e) {
					// A diagnostic must never cost the user a key.
				}
			}
			return event;
		}] retain];

		NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
		observe(center, NSApplicationDidBecomeActiveNotification, @"appActive");
		observe(center, NSApplicationDidResignActiveNotification, @"appInactive");
		observe(center, NSWindowDidBecomeKeyNotification, @"windowKey");
		observe(center, NSWindowDidResignKeyNotification, @"windowNotKey");
		observe(center, NSTextInputContextKeyboardSelectionDidChangeNotification, @"contextSourceChanged");
		NSDistributedNotificationCenter *system = [NSDistributedNotificationCenter defaultCenter];
		observe(system, (NSString *)kTISNotifySelectedKeyboardInputSourceChanged, @"systemSourceChanged");
		observe(system, (NSString *)kTISNotifyEnabledKeyboardInputSourcesChanged, @"systemSourcesEnabled");
	});
}

// keylog_snapshot describes the text-input state now, as JSON for the log's
// header. The caller frees the result.
char *keylog_snapshot(void) {
	__block char *out = NULL;
	void (^take)(void) = ^{
		@autoreleasepool {
			NSMutableDictionary *d = textInputState(nil);
			d[@"os"] = [[NSProcessInfo processInfo] operatingSystemVersionString];
			d[@"windows"] = @([[NSApp windows] count]);
			NSTextInputContext *ctx = [NSTextInputContext currentInputContext];
			if (ctx != nil) {
				d[@"ctxSources"] = [ctx keyboardInputSources] ?: @[];
			}
			NSData *json = [NSJSONSerialization dataWithJSONObject:d options:0 error:NULL];
			if (json != nil) {
				out = malloc([json length] + 1);
				if (out != NULL) {
					memcpy(out, [json bytes], [json length]);
					out[[json length]] = 0;
				}
			}
		}
	};
	if ([NSThread isMainThread]) {
		take();
	} else {
		dispatch_sync(dispatch_get_main_queue(), take);
	}
	return out;
}
