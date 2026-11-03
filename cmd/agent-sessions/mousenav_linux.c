//go:build linux && desktop

#include <gtk/gtk.h>
#include <webkit/webkit.h>

#include "_cgo_export.h"

// GTK numbers the mouse's side buttons 8 (back) and 9 (forward).
#define BUTTON_BACK 8
#define BUTTON_FORWARD 9

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

// Connects to the window's web view once; the released handler reports the
// window, so the event goes to that window's page.
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
	g_object_set_data(G_OBJECT(view), "agent-sessions-side-buttons", GINT_TO_POINTER(1));
}
