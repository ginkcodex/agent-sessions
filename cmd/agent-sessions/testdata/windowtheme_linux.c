#include <gtk/gtk.h>

int set_linux_window_theme(void *native, int appearance);

static gboolean dark_preference(GtkSettings *settings) {
    gboolean dark = FALSE;
    g_object_get(settings, "gtk-application-prefer-dark-theme", &dark, NULL);
    return dark;
}

int main(void) {
    if (!gtk_init_check()) {
        g_printerr("No GTK display available\n");
        return 77;
    }
    GtkWidget *first = gtk_window_new();
    GtkWidget *second = gtk_window_new();
    g_object_ref_sink(first);
    g_object_ref_sink(second);
    GtkSettings *settings = gtk_widget_get_settings(first);
    // Pin only this test process to GTK's bundled theme for color assertions.
    g_object_set(settings, "gtk-theme-name", "Default", NULL);
    gboolean original = dark_preference(settings);
    char *theme = NULL;
    g_object_get(settings, "gtk-theme-name", &theme, NULL);
    g_assert_true(settings == gtk_widget_get_settings(second));
    g_assert_cmpint(set_linux_window_theme(NULL, 2), ==, 0);
    GtkWidget *label = gtk_label_new("Native theme probe");
    gtk_window_set_child(GTK_WINDOW(first), label);
    gtk_widget_realize(first);
    GdkRGBA light_color, dark_color;
    for (int i = 0; i < 3; i++) {
        g_assert_cmpint(set_linux_window_theme(first, 2), ==, 1);
        g_assert_true(dark_preference(settings));
        G_GNUC_BEGIN_IGNORE_DEPRECATIONS
        gtk_style_context_get_color(gtk_widget_get_style_context(label), &dark_color);
        G_GNUC_END_IGNORE_DEPRECATIONS
        g_assert_cmpint(set_linux_window_theme(second, 1), ==, 1);
        g_assert_false(dark_preference(settings));
        G_GNUC_BEGIN_IGNORE_DEPRECATIONS
        gtk_style_context_get_color(gtk_widget_get_style_context(label), &light_color);
        G_GNUC_END_IGNORE_DEPRECATIONS
        g_assert_false(gdk_rgba_equal(&light_color, &dark_color));
        g_assert_cmpint(set_linux_window_theme(first, 0), ==, 1);
        g_assert_cmpint(dark_preference(settings), ==, original);
    }
    g_object_set(settings, "gtk-theme-name", "WindowThemeProbeCustom", NULL);
    set_linux_window_theme(first, 2);
    set_linux_window_theme(first, 1);
    set_linux_window_theme(first, 0);
    char *custom = NULL;
    g_object_get(settings, "gtk-theme-name", &custom, NULL);
    g_assert_cmpstr(custom, ==, "WindowThemeProbeCustom");
    g_free(custom);
    g_object_set(settings, "gtk-theme-name", theme, NULL);
    char *after = NULL;
    g_object_get(settings, "gtk-theme-name", &after, NULL);
    g_assert_cmpstr(theme, ==, after);
    g_assert_true(gtk_window_get_decorated(GTK_WINDOW(first)));
    g_assert_true(gtk_window_get_decorated(GTK_WINDOW(second)));
    g_print("GTK %u.%u.%u theme=%s: explicit modes, native widget colors, system reset, custom theme preservation, shared display scope and native decorations passed\n",
            gtk_get_major_version(), gtk_get_minor_version(), gtk_get_micro_version(), theme);
    g_free(theme);
    g_free(after);
    gtk_window_destroy(GTK_WINDOW(first));
    gtk_window_destroy(GTK_WINDOW(second));
    g_object_unref(first);
    g_object_unref(second);
    return 0;
}
