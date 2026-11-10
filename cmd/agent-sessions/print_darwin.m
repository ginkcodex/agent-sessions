//go:build darwin && desktop

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

#define POINTS_PER_MM (72.0 / 25.4)

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

// Called on the main thread, then schedules the panel without blocking the Go
// binding. Retain the calling window and its view until that block runs.
int print_web_view(void *nsWindow, int letter, int landscape, double margin_mm) {
	if (@available(macOS 11.0, *)) {
		if (nsWindow == NULL) {
			return -2;
		}
		NSWindow *window = (NSWindow *)nsWindow;
		WKWebView *webView = find_web_view([window contentView]);
		if (webView == nil) {
			return -3;
		}
		[window retain];
		[webView retain];
		dispatch_async(dispatch_get_main_queue(), ^{
			NSPrintInfo *info = [[[NSPrintInfo alloc] initWithDictionary:@{}] autorelease];
			// Start with unrotated dimensions; setting orientation rotates them.
			info.orientation = NSPaperOrientationPortrait;
			info.paperName = letter ? @"na-letter" : @"iso-a4";
			info.paperSize = letter ? NSMakeSize(612.0, 792.0)
			                        : NSMakeSize(210.0 * POINTS_PER_MM, 297.0 * POINTS_PER_MM);
			info.orientation = landscape ? NSPaperOrientationLandscape : NSPaperOrientationPortrait;
			[[info dictionary] setObject:@1.0 forKey:NSPrintScalingFactor];
			info.horizontalPagination = NSPrintingPaginationModeAutomatic;
			info.verticalPagination = NSPrintingPaginationModeAutomatic;
			info.horizontallyCentered = NO;
			info.verticallyCentered = NO;
			info.leftMargin = margin_mm * POINTS_PER_MM;
			info.rightMargin = margin_mm * POINTS_PER_MM;
			info.topMargin = margin_mm * POINTS_PER_MM;
			info.bottomMargin = margin_mm * POINTS_PER_MM;

			NSPrintOperation *op = [webView printOperationWithPrintInfo:info];
			op.showsPrintPanel = YES;
			op.showsProgressPanel = YES;
			op.printPanel.options |= NSPrintPanelShowsOrientation | NSPrintPanelShowsPaperSize | NSPrintPanelShowsScaling;
			// Without a frame the operation's view prints blank pages.
			op.view.frame = webView.bounds;
			[op runOperationModalForWindow:window delegate:nil didRunSelector:nil contextInfo:nil];
			[webView release];
			[window release];
		});
		return 0;
	}
	return -1;
}
