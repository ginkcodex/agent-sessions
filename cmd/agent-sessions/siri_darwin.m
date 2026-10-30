//go:build darwin && (webkit2_41 || desktop)

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

static BOOL no_affordance(id self, SEL _cmd) {
	return NO;
}

// macOS 27 floats a Siri button by any editable field whose caret rests.
// xterm keeps a focused textarea at the terminal's cursor, so the button
// sat on the prompt after every pause. Neither writingsuggestions="false"
// nor writingToolsBehavior = none hides it on macOS 27; a web view that
// answers NO to allowsWritingToolsAffordance does. Wails creates its own
// WKWebView subclass, WailsWebView, so the answer is set on that class and
// stock WKWebView is left alone.
void hide_siri_affordance(void) {
	Class cls = NSClassFromString(@"WailsWebView");
	SEL sel = NSSelectorFromString(@"allowsWritingToolsAffordance");
	if (cls == Nil || ![cls instancesRespondToSelector:sel]) {
		return;
	}
	class_replaceMethod(cls, sel, (IMP)no_affordance, "B@:");
}
