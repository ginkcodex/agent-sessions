//go:build linux && desktop

#include <gtk/gtk.h>
#include <webkit/webkit.h>

static WebKitWebView *find_print_web_view(GtkWidget *widget) {
	if (WEBKIT_IS_WEB_VIEW(widget)) {
		return WEBKIT_WEB_VIEW(widget);
	}
	for (GtkWidget *child = gtk_widget_get_first_child(widget); child != NULL;
	     child = gtk_widget_get_next_sibling(child)) {
		WebKitWebView *found = find_print_web_view(child);
		if (found != NULL) {
			return found;
		}
	}
	return NULL;
}

static void print_finished(WebKitPrintOperation *operation, gpointer data) {
	// Release the asynchronous reference only after rendering/spooling finishes.
	g_object_unref(operation);
}

static void print_failed(WebKitPrintOperation *operation, GError *error, gpointer data) {
	// Failure is followed by finished; releasing here would release twice.
	g_warning("print: %s", error->message);
}

// Called on GTK's main thread. Explicitly retain the view and parent through
// completion, even if that window closes while printing.
int print_web_view(void *window, int letter, int landscape, double margin_mm) {
	if (window == NULL) {
		return -2;
	}
	WebKitWebView *view = find_print_web_view(GTK_WIDGET(window));
	if (view == NULL) {
		return -1;
	}

	GtkPaperSize *paper = gtk_paper_size_new(letter ? GTK_PAPER_NAME_LETTER : GTK_PAPER_NAME_A4);
	GtkPageOrientation orientation = landscape ? GTK_PAGE_ORIENTATION_LANDSCAPE : GTK_PAGE_ORIENTATION_PORTRAIT;
	GtkPageSetup *page = gtk_page_setup_new();
	gtk_page_setup_set_paper_size(page, paper);
	gtk_page_setup_set_orientation(page, orientation);
	gtk_page_setup_set_top_margin(page, margin_mm, GTK_UNIT_MM);
	gtk_page_setup_set_bottom_margin(page, margin_mm, GTK_UNIT_MM);
	gtk_page_setup_set_left_margin(page, margin_mm, GTK_UNIT_MM);
	gtk_page_setup_set_right_margin(page, margin_mm, GTK_UNIT_MM);

	GtkPrintSettings *settings = gtk_print_settings_new();
	gtk_print_settings_set_paper_size(settings, paper);
	gtk_print_settings_set_orientation(settings, orientation);
	gtk_print_settings_set_scale(settings, 100.0);

	WebKitPrintOperation *operation = webkit_print_operation_new(view);
	webkit_print_operation_set_page_setup(operation, page);
	webkit_print_operation_set_print_settings(operation, settings);
	g_object_set_data_full(G_OBJECT(operation), "agent-sessions-print-parent",
	                       g_object_ref(window), g_object_unref);
	g_object_set_data_full(G_OBJECT(operation), "agent-sessions-print-view",
	                       g_object_ref(view), g_object_unref);
	gtk_paper_size_free(paper);
	g_object_unref(page);
	g_object_unref(settings);

	// Separate local and asynchronous references: finished may arrive inside
	// run_dialog's nested main loop, before it returns. Cancellation emits no
	// finished, so explicitly disconnect and release that reference in that case.
	g_object_ref(operation);
	gulong finished = g_signal_connect(operation, "finished", G_CALLBACK(print_finished), NULL);
	g_signal_connect(operation, "failed", G_CALLBACK(print_failed), NULL);
	WebKitPrintOperationResponse response = webkit_print_operation_run_dialog(operation, GTK_WINDOW(window));
	if (response == WEBKIT_PRINT_OPERATION_RESPONSE_CANCEL) {
		g_signal_handler_disconnect(operation, finished);
		g_object_unref(operation);
	}
	g_object_unref(operation);
	return 0;
}
