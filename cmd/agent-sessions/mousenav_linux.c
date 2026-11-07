//go:build linux && desktop

#include <gtk/gtk.h>
#include <math.h>
#include <webkit/webkit.h>

#include "_cgo_export.h"

// GTK numbers the mouse's side buttons 8 (back) and 9 (forward).
#define BUTTON_BACK 8
#define BUTTON_FORWARD 9

// A two-finger swipe goes back or forward once it has gone this far
// sideways (in pixels), and more than twice as far as up or down, as on
// macOS (mousenav_darwin.m).
#define SWIPE_DISTANCE 80.0
// Where GTK sends no scroll-end (X11), scrolls further apart than this (ms)
// are a new gesture.
#define SWIPE_GESTURE_GAP 200
// Pixels per wheel unit (X11 touchpads), WebKit's line step.
#define PIXELS_PER_WHEEL_UNIT 40.0

// The touchpad gesture in progress on one web view: its window, how far it
// went, when it last moved, and whether it already navigated. ends is set
// once GTK has sent a scroll-end, which it does when the fingers lift
// (Wayland). newest is the time of the newest scroll or scroll-end seen.
typedef struct {
	void *window;
	double x, y;
	guint32 last, newest;
	gboolean active, sent, ends;
} Swipe;

// AGENT_SESSIONS_TRACE_NAV=1 prints each touchpad gesture's begin, end,
// scrolls and sends to stderr, to see how GTK splits a real swipe.
static gboolean trace;

static guint32 event_time(GtkEventControllerScroll *controller) {
	return gtk_event_controller_get_current_event_time(GTK_EVENT_CONTROLLER(controller));
}

// The controller sees each scroll twice: once as it happens, and again,
// with its own time, when WebKit hands back what the page did not use
// (sideways scrolls, mostly). The copy can come after the swipe's
// scroll-end, while the page is busy drawing what the swipe navigated to;
// counted, it would be a second swipe. An event no newer than one already
// seen is such a copy. Times are milliseconds and wrap, hence the signed
// difference.
static gboolean replayed(Swipe *swipe, guint32 time) {
	return time != 0 && swipe->newest != 0 && (gint32)(time - swipe->newest) <= 0;
}

// Capture and claim side-button sequences before WebKit can turn them into
// page clicks. Navigation happens only on release, not on press.
static void on_side_button_pressed(GtkGestureClick *gesture, int n_press,
                                  double x, double y, gpointer data) {
	guint button = gtk_gesture_single_get_current_button(GTK_GESTURE_SINGLE(gesture));
	if (button == BUTTON_BACK || button == BUTTON_FORWARD) {
		gtk_gesture_set_state(GTK_GESTURE(gesture), GTK_EVENT_SEQUENCE_CLAIMED);
	}
}

static void on_side_button_released(GtkGestureClick *gesture, int n_press,
                                   double x, double y, gpointer window) {
	guint button = gtk_gesture_single_get_current_button(GTK_GESTURE_SINGLE(gesture));
	if (button == BUTTON_BACK || button == BUTTON_FORWARD) {
		gtk_gesture_set_state(GTK_GESTURE(gesture), GTK_EVENT_SEQUENCE_CLAIMED);
		goMouseNavigate(window, button == BUTTON_FORWARD);
	}
}

static void begin_swipe(GtkEventControllerScroll *controller, Swipe *swipe) {
	swipe->x = 0;
	swipe->y = 0;
	swipe->sent = FALSE;
	swipe->active = TRUE;
	swipe->last = event_time(controller);
}

// GTK begins a gesture on the first scroll after an end, a replayed one
// too; that one begins nothing.
static void on_scroll_begin(GtkEventControllerScroll *controller, Swipe *swipe) {
	guint32 time = event_time(controller);
	gboolean replay = replayed(swipe, time);
	if (trace) {
		g_printerr("nav-trace %u scroll-begin (was active %d, sent %d)%s\n", time, swipe->active, swipe->sent,
		           replay ? " replayed, ignored" : "");
	}
	if (!replay) {
		begin_swipe(controller, swipe);
	}
}

// A scroll-end ends the gesture; a replayed one ends nothing new. GTK also
// ends a gesture on a timer when fingers rest on the pad and lift without
// scrolling; that end has no event, so its time is 0.
static void on_scroll_end(GtkEventControllerScroll *controller, Swipe *swipe) {
	guint32 time = event_time(controller);
	if (trace) {
		g_printerr("nav-trace %u scroll-end x %.1f y %.1f sent %d\n", time, swipe->x, swipe->y, swipe->sent);
	}
	swipe->active = FALSE;
	swipe->ends = TRUE;
	if (time != 0 && !replayed(swipe, time)) {
		swipe->newest = time;
	}
}

// A two-finger touchpad swipe is a scroll in GTK. Only touchpads count, not
// wheels or trackpoints. The scroll always goes on to WebKit; the page drops
// the swipe if it scrolled something (swipeNavigation.ts). As in Chrome, the
// swipe follows the scroll: toward the left is back. One swipe is one step:
// a gesture navigates once, and with scroll-end it lasts until the fingers
// lift, even if they rest on the pad for a while. Replayed scrolls are
// skipped (see replayed).
static gboolean on_scroll(GtkEventControllerScroll *controller, double dx, double dy, Swipe *swipe) {
	GtkEventController *events = GTK_EVENT_CONTROLLER(controller);
	GdkDevice *device = gtk_event_controller_get_current_event_device(events);
	guint32 time = gtk_event_controller_get_current_event_time(events);
	if (trace) {
		g_printerr("nav-trace %u scroll dx %.2f dy %.2f unit %d source %d active %d sent %d\n", time, dx, dy,
		           gtk_event_controller_scroll_get_unit(controller), device ? (int)gdk_device_get_source(device) : -1,
		           swipe->active, swipe->sent);
	}
	if (device == NULL || gdk_device_get_source(device) != GDK_SOURCE_TOUCHPAD ||
	    (gtk_event_controller_get_current_event_state(events) & GDK_CONTROL_MASK)) {
		return FALSE;
	}
	if (replayed(swipe, time)) {
		return FALSE;
	}
	swipe->newest = time;
	if (!swipe->active || (!swipe->ends && time - swipe->last > SWIPE_GESTURE_GAP)) {
		if (trace) {
			g_printerr("nav-trace %u new gesture (active %d, gap %u)\n", time, swipe->active, time - swipe->last);
		}
		begin_swipe(controller, swipe);
	}
	swipe->last = time;
	if (swipe->sent) {
		return FALSE;
	}
	double scale = gtk_event_controller_scroll_get_unit(controller) == GDK_SCROLL_UNIT_WHEEL ? PIXELS_PER_WHEEL_UNIT : 1;
	swipe->x += dx * scale;
	swipe->y += dy * scale;
	if (fabs(swipe->x) >= SWIPE_DISTANCE && fabs(swipe->x) > 2 * fabs(swipe->y)) {
		swipe->sent = TRUE;
		if (trace) {
			g_printerr("nav-trace %u send %s x %.1f y %.1f\n", time, swipe->x > 0 ? "forward" : "back", swipe->x, swipe->y);
		}
		goSwipeNavigate(swipe->window, swipe->x > 0);
	}
	return FALSE;
}

static WebKitWebView *find_web_view(GtkWidget *widget) {
	if (WEBKIT_IS_WEB_VIEW(widget)) {
		return WEBKIT_WEB_VIEW(widget);
	}
	for (GtkWidget *child = gtk_widget_get_first_child(widget); child != NULL;
	     child = gtk_widget_get_next_sibling(child)) {
		WebKitWebView *found = find_web_view(child);
		if (found != NULL) {
			return found;
		}
	}
	return NULL;
}

// The hook is per window: hook_window_side_buttons connects to the web view
// of each window once its page is ready.
void hook_side_buttons(void) {}

// Connects to the window's web view once; the handlers report the window,
// so the event goes to that window's page.
void hook_window_side_buttons(void *window) {
	WebKitWebView *view = find_web_view(GTK_WIDGET(window));
	if (view == NULL || g_object_get_data(G_OBJECT(view), "agent-sessions-side-buttons") != NULL) {
		return;
	}
	GtkGesture *gesture = gtk_gesture_click_new();
	gtk_gesture_single_set_button(GTK_GESTURE_SINGLE(gesture), 0);
	gtk_event_controller_set_propagation_phase(GTK_EVENT_CONTROLLER(gesture), GTK_PHASE_CAPTURE);
	g_signal_connect(gesture, "pressed", G_CALLBACK(on_side_button_pressed), NULL);
	g_signal_connect(gesture, "released", G_CALLBACK(on_side_button_released), window);
	gtk_widget_add_controller(GTK_WIDGET(view), GTK_EVENT_CONTROLLER(gesture));

	const char *trace_env = g_getenv("AGENT_SESSIONS_TRACE_NAV");
	trace = trace_env != NULL && trace_env[0] != '\0' && g_strcmp0(trace_env, "0") != 0;
	Swipe *swipe = g_new0(Swipe, 1);
	swipe->window = window;
	g_object_set_data_full(G_OBJECT(view), "agent-sessions-swipe", swipe, g_free);
	GtkEventController *scroll = gtk_event_controller_scroll_new(GTK_EVENT_CONTROLLER_SCROLL_BOTH_AXES);
	gtk_event_controller_set_propagation_phase(scroll, GTK_PHASE_CAPTURE);
	g_signal_connect(scroll, "scroll-begin", G_CALLBACK(on_scroll_begin), swipe);
	g_signal_connect(scroll, "scroll", G_CALLBACK(on_scroll), swipe);
	g_signal_connect(scroll, "scroll-end", G_CALLBACK(on_scroll_end), swipe);
	gtk_widget_add_controller(GTK_WIDGET(view), scroll);
	g_object_set_data(G_OBJECT(view), "agent-sessions-side-buttons", GINT_TO_POINTER(1));
}
