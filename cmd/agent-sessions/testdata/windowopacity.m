#import <Cocoa/Cocoa.h>

void make_window_opaque(void *native);

int main(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        NSCAssert([NSThread isMainThread], @"AppKit test must run on main thread");
        make_window_opaque(NULL);
        NSWindow *window = [[NSWindow alloc]
            initWithContentRect:NSMakeRect(0, 0, 200, 100)
            styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskResizable
            backing:NSBackingStoreBuffered defer:YES];
        window.releasedWhenClosed = NO;
        NSView *content = window.contentView;
        NSView *overlay = [[NSView alloc] initWithFrame:content.bounds];
        [content addSubview:overlay];
        NSUInteger mask = window.styleMask;
        BOOL shadow = window.hasShadow;
        for (NSAppearanceName name in @[NSAppearanceNameAqua, NSAppearanceNameDarkAqua]) {
            window.appearance = [NSAppearance appearanceNamed:name];
            window.backgroundColor = NSColor.clearColor;
            window.opaque = NO;
            make_window_opaque((__bridge void *)window);
            NSCAssert(window.opaque, @"native window must be opaque");
            [window.effectiveAppearance performAsCurrentDrawingAppearance:^{
                NSColor *color = [window.backgroundColor colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
                NSColor *expected = [NSColor.windowBackgroundColor colorUsingColorSpace:NSColorSpace.sRGBColorSpace];
                NSCAssert(color != nil && color.alphaComponent == 1, @"backing color must be opaque");
                NSCAssert([color isEqual:expected], @"backing color must follow appearance");
            }];
            make_window_opaque((__bridge void *)window);
            NSCAssert(window.opaque, @"repeat call must be safe");
            NSCAssert(window.contentView == content && overlay.superview == content, @"child views must be unchanged");
            NSCAssert(window.styleMask == mask && window.hasShadow == shadow && window.alphaValue == 1, @"window style must be unchanged");
        }
        NSCAssert(!window.visible, @"test must not show a window");
        [window close];
    }
    return 0;
}
