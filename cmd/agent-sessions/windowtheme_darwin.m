//go:build darwin && desktop

#import <Cocoa/Cocoa.h>

void set_window_theme(void *native, int appearance) {
    NSWindow *window = (__bridge NSWindow *)native;
    NSAppearance *value = nil;
    if (appearance == 1) {
        value = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
    } else if (appearance == 2) {
        value = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
    }
    // nil restores system inheritance, including future OS appearance changes.
    [window setAppearance:value];
}
