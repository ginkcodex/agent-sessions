//go:build darwin && desktop

#import <Cocoa/Cocoa.h>

// Called on the AppKit main thread after the main window's runtime is ready.
// Wails initializes NSWindow with clearColor and opaque=NO even for its normal
// backdrop. Our page is solid: give AppKit an opaque, appearance-adaptive backing
// before declaring that fact. Leave WKWebView, file-drop overlays, titlebar and
// window shadows alone. Whether this improves GPU cost is an experiment (IR-07).
void make_window_opaque(void *native) {
    NSWindow *window = (__bridge NSWindow *)native;
    if (window == nil) {
        return;
    }
    [window setBackgroundColor:[NSColor windowBackgroundColor]];
    [window setOpaque:YES];
}
