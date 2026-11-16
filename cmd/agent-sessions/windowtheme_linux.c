//go:build linux && desktop

#include <gtk/gtk.h>

// Called on GTK's main thread. GtkSettings changes only this process/display,
// not desktop configuration; GTK also updates X11 decoration hints itself.
int set_linux_window_theme(void *native, int appearance) {
    if (native == NULL) {
        return 0;
    }
    GtkSettings *settings = gtk_widget_get_settings(GTK_WIDGET(native));
    if (settings == NULL) {
        return 0;
    }
    // shortcut: GTK's theme preference is display-wide; use a per-window style
    // provider if independent native themes become necessary.
    if (appearance == 0) {
        gtk_settings_reset_property(settings, "gtk-application-prefer-dark-theme");
    } else {
        g_object_set(settings, "gtk-application-prefer-dark-theme", appearance == 2, NULL);
    }
    return 1;
}
