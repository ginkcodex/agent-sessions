//go:build darwin && desktop

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>
#import <objc/message.h>

// Zooms a window with AppKit's own -[NSWindow zoom:], bypassing a subclass
// override. Wails' window class overrides zoom: to drive its own resize
// animation from a display link; a window that is not on screen yet gets no
// ticks, so the window reports zoomed but keeps its normal frame until it is
// shown and then visibly grows. AppKit's implementation applies a hidden
// window's standard frame at once and remembers the frame it had as the user
// frame, which a later zoom (Wails' or AppKit's) restores.
//
// Must run on the AppKit main thread. Does nothing when the window is gone or
// already zoomed (zoom: toggles).
void window_zoom_now(void *native) {
    NSWindow *window = (__bridge NSWindow *)native;
    if (window == nil || [window isZoomed]) {
        return;
    }
    Method method = class_getInstanceMethod([NSWindow class], @selector(zoom:));
    if (method == NULL) {
        [window zoom:nil];
        return;
    }
    ((void (*)(id, SEL, id))method_getImplementation(method))(window, @selector(zoom:), nil);
}
