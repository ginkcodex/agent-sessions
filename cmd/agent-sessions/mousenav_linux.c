//go:build linux && desktop

#include <gtk/gtk.h>
#include <webkit/webkit.h>

#include "_cgo_export.h"

// GTK numbers the mouse's side buttons 8 (back) and 9 (forward).
#define BUTTON_BACK 8
#define BUTTON_FORWARD 9
#define HOOK_INTERVAL_MS 100
#define HOOK_TRIES 100

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
                                   double x, double y, gpointer data) {
	guint button = gtk_gesture_single_get_current_button(GTK_GESTURE_SINGLE(gesture));
	if (button == BUTTON_BACK || button == BUTTON_FORWARD) {
		gtk_gesture_set_state(GTK_GESTURE(gesture), GTK_EVENT_SEQUENCE_CLAIMED);
		goMouseNavigate(button == BUTTON_FORWARD);
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

static int hook_tries;

// Polls until the window's web view exists, then connects to it once.
static gboolean try_hook_side_buttons(gpointer data) {
	gboolean hooked = FALSE;
	GListModel *windows = gtk_window_get_toplevels();
	for (guint i = 0; i < g_list_model_get_n_items(windows); i++) {
		GtkWidget *window = g_list_model_get_item(windows, i);
		WebKitWebView *view = find_web_view(window);
		if (view != NULL) {
			if (g_object_get_data(G_OBJECT(view), "agent-sessions-side-buttons") == NULL) {
				GtkGesture *gesture = gtk_gesture_click_new();
				gtk_gesture_single_set_button(GTK_GESTURE_SINGLE(gesture), 0);
				gtk_event_controller_set_propagation_phase(GTK_EVENT_CONTROLLER(gesture), GTK_PHASE_CAPTURE);
				g_signal_connect(gesture, "pressed", G_CALLBACK(on_side_button_pressed), NULL);
				g_signal_connect(gesture, "released", G_CALLBACK(on_side_button_released), NULL);
				gtk_widget_add_controller(GTK_WIDGET(view), GTK_EVENT_CONTROLLER(gesture));
				g_object_set_data(G_OBJECT(view), "agent-sessions-side-buttons", GINT_TO_POINTER(1));
			}
			hooked = TRUE;
		}
		g_object_unref(window);
	}
	if (hooked || ++hook_tries >= HOOK_TRIES) {
		return G_SOURCE_REMOVE;
	}
	return G_SOURCE_CONTINUE;
}

void hook_side_buttons(void) {
	g_timeout_add(HOOK_INTERVAL_MS, try_hook_side_buttons, NULL);
}
