//go:build darwin && desktop

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// Points per millimetre; the margin matches @page in style.css.
#define PRINT_MARGIN (14.0 * 72.0 / 25.4)

static WKWebView *find_web_view(NSView *view) {
	if ([view isKindOfClass:[WKWebView class]]) {
		return (WKWebView *)view;
	}
	for (NSView *sub in [view subviews]) {
		WKWebView *found = find_web_view(sub);
		if (found != nil) {
			return found;
		}
	}
	return nil;
}

// Wails' WindowPrint always prints landscape with no margins and hides the
// orientation choice. This prints the given window's web view the same way,
// but in portrait with margins, and the print panel offers orientation,
// paper size and scale. Returns 0 once the print is started on the main
// thread, -1 before macOS 11, where WKWebView cannot print, or -2 without a
// window.
int print_web_view(void *nsWindow) {
	if (@available(macOS 11.0, *)) {
		if (nsWindow == NULL) {
			return -2;
		}
		// Kept until the block runs, in case the window closes meanwhile.
		NSWindow *window = [(NSWindow *)nsWindow retain];
		dispatch_async(dispatch_get_main_queue(), ^{
			WKWebView *webView = find_web_view([window contentView]);
			if (webView == nil) {
				NSLog(@"print: no web view to print");
				[window release];
				return;
			}

			NSPrintInfo *info = [[[NSPrintInfo sharedPrintInfo] copy] autorelease];
			info.orientation = NSPaperOrientationPortrait;
			info.horizontalPagination = NSPrintingPaginationModeAutomatic;
			info.verticalPagination = NSPrintingPaginationModeAutomatic;
			info.horizontallyCentered = NO;
			info.verticallyCentered = NO;
			info.leftMargin = PRINT_MARGIN;
			info.rightMargin = PRINT_MARGIN;
			info.topMargin = PRINT_MARGIN;
			info.bottomMargin = PRINT_MARGIN;

			NSPrintOperation *op = [webView printOperationWithPrintInfo:info];
			op.showsPrintPanel = YES;
			op.showsProgressPanel = YES;
			op.printPanel.options |= NSPrintPanelShowsOrientation | NSPrintPanelShowsPaperSize | NSPrintPanelShowsScaling;
			// Without a frame the operation's view prints blank pages.
			op.view.frame = webView.bounds;
			[op runOperationModalForWindow:window delegate:nil didRunSelector:nil contextInfo:nil];
			[window release];
		});
		return 0;
	}
	return -1;
}
